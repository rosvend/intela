package aplicacion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Ingesta recibe entregas de reportes de uso y las deja en forma canonica.
//
// Es la columna vertebral de KR-1: toda cifra que el sistema produzca tiene
// que poder rastrearse hasta el byte exacto del que salio. Eso son dos cosas
// distintas y las dos viven aqui:
//
//   - La evidencia CRUDA se congela con su huella SHA-256 (ADR 0006). Una
//     corrida no referencia "el archivo de Caracol", referencia unos bytes.
//   - La forma CANONICA se persiste sin importes. Un reporte de uso PONDERA la
//     bolsa, no la aporta; el esquema lo refuerza no teniendo columna de dinero.
//
// # Que NO decide
//
// Como se lee un .xlsx o un CSV: eso es un adaptador de formato (#25). Aqui
// llegan bytes y filas ya mapeadas al esquema canonico.
//
// Y a que obra corresponde cada fila: la cascada de identificacion es otro
// modulo (ADR 0007). Todo lo que entra por aqui sale con escalon "pendiente".
type Ingesta struct {
	Reportes RepositorioIngesta
	Almacen  AlmacenObjetos
}

// periodoValido refleja el CHECK de reportes.periodo: un ano, opcionalmente
// con mes. Se comprueba aqui y no solo en la base porque el orden de las
// operaciones de GuardarReporte escribe la boveda ANTES que la fila, y de la
// boveda no se puede borrar nada.
var periodoValido = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2})?$`)

// escalones son los valores que admite el CHECK de usos.escalon.
//
// "manual" no esta: una resolucion manual necesita autor e instante -lo exige
// el CHECK manual_tiene_autor- y ninguno de los dos entra por ingesta.
var escalones = map[string]bool{
	"pendiente": true,
	"alias":     true,
	"id_global": true,
	"difuso":    true,
	"oni":       true,
}

// huella devuelve el SHA-256 hexadecimal de unos bytes.
//
// En minusculas y sin separadores porque asi lo exige el CHECK del esquema
// (`sha256 ~ '^[0-9a-f]{64}$'`), que es la forma en la que se cita una
// evidencia en toda la trazabilidad.
func huella(datos []byte) string {
	suma := sha256.Sum256(datos)
	return hex.EncodeToString(suma[:])
}

// claveObjeto deriva la clave del almacen a partir de la huella.
//
// La clave ES el contenido, y eso tiene tres consecuencias buscadas:
//
//  1. Los mismos bytes ocupan un solo objeto, aunque los declaren dos fuentes
//     distintas -que el UNIQUE (sha256, fuente) permite a proposito-.
//  2. Reescribir una clave existente es reescribir contenido identico, asi que
//     la inmutabilidad del ADR 0006 no se puede violar por esta via ni
//     queriendo.
//  3. No entra en la clave nada que venga del formulario de subida. El nombre
//     del fichero llega en multipart.FileHeader.Filename, que la propia
//     documentacion de Go advierte que no es de fiar, y el almacen rechaza
//     todo lo que no sea [A-Za-z0-9._-]: componer la clave con la huella la
//     deja dentro del alfabeto por construccion.
func claveObjeto(sha string) string {
	return "reportes/" + sha
}

// idReporte deriva el identificador de una entrega.
//
// Sale del PAR (fuente, huella) y no de la huella sola: el esquema admite que
// dos fuentes entreguen los mismos bytes, y un id derivado solo del contenido
// haria que la segunda chocara contra la clave primaria en vez de aceptarse.
//
// Que sea derivado y no aleatorio es deliberado, y no contradice el ADR 0006:
// alli el aviso es sobre los ASIENTOS, donde un mismo hecho ocurrido dos veces
// tiene que dejar dos filas. Aqui es al reves -una misma entrega dos veces
// tiene que dejar UNA-, asi que un id derivado del mismo par que el
// UNIQUE (sha256, fuente) hace unico impide que la clave primaria y la
// restriccion de unicidad puedan discrepar.
func idReporte(fuente, sha string) string {
	suma := sha256.Sum256([]byte(fuente + "\x00" + sha))
	return "rep-" + hex.EncodeToString(suma[:])
}

// GuardarReporte congela una entrega: los bytes en la boveda y el acuse en la
// base.
//
// # El orden importa, y este es el motivo
//
// Primero la boveda, despues la fila. El estado que no puede existir es un
// acuse en `reportes` que apunte a una evidencia que no se llego a escribir:
// eso es una cifra que dice de donde salio y no se puede comprobar, que es
// justo lo que el ADR 0006 existe para impedir. Al reves, un objeto sin acuse
// es evidencia inerte que no referencia nadie, y ademas se recupera sola: el
// reintento vuelve a poner los mismos bytes bajo la misma clave -la clave es
// el contenido- y completa la fila que falto.
//
// # La boveda queda FUERA de cualquier transaccion, y no puede ser de otra
// forma
//
// De un fichero escrito no se hace rollback. Meter el Poner dentro de una
// transaccion SQL daria la ilusion de atomicidad y no la propiedad: si el
// COMMIT falla despues, el objeto sigue ahi igualmente. Se asume por escrito
// el unico resto posible -objetos huerfanos- en vez de fingir que no existe.
//
// # El duplicado lo decide la base
//
// La deteccion por huella es el UNIQUE (sha256, fuente), y esa comprobacion
// llega DESPUES de tocar la boveda. No es un problema: el almacen no
// sobrescribe, asi que una resubida de los mismos bytes deja el objeto
// literalmente sin cambios y despues se rechaza con ErrReporteDuplicado.
func (i Ingesta) GuardarReporte(ctx context.Context, fuente, periodo string, datos []byte) (Reporte, error) {
	switch {
	case strings.TrimSpace(fuente) == "":
		return Reporte{}, fmt.Errorf("%w: falta la fuente", ErrReporteInvalido)
	case !periodoValido.MatchString(periodo):
		return Reporte{}, fmt.Errorf(
			"%w: periodo %q, se esperaba AAAA o AAAA-MM", ErrReporteInvalido, periodo)
	case len(datos) == 0:
		return Reporte{}, fmt.Errorf("%w: la entrega no trae bytes", ErrReporteInvalido)
	}

	rep := Reporte{
		Fuente:  fuente,
		Periodo: periodo,
		SHA256:  huella(datos),
		NBytes:  len(datos),
	}
	rep.ID = idReporte(fuente, rep.SHA256)
	rep.ClaveObjeto = claveObjeto(rep.SHA256)

	// ErrObjetoYaExiste no es un fallo: la clave es la huella, asi que lo que
	// ya hay bajo ella son estos mismos bytes. Puede venir de una resubida
	// -que la fila de abajo rechazara-, de otra fuente que entrego lo mismo, o
	// de un intento anterior que murio entre el Poner y el INSERT.
	if err := i.Almacen.Poner(ctx, rep.ClaveObjeto, datos); err != nil &&
		!errors.Is(err, ErrObjetoYaExiste) {
		return Reporte{}, fmt.Errorf("guardar los bytes crudos de %q: %w", fuente, err)
	}

	if err := i.Reportes.GuardarReporte(
		ctx, rep.ID, rep.Fuente, rep.Periodo, rep.SHA256, rep.ClaveObjeto, rep.NBytes,
	); err != nil {
		return Reporte{}, err
	}
	return rep, nil
}

// GuardarUsos persiste las filas de un reporte y devuelve las RECHAZADAS, cada
// una con su motivo.
//
// Un lote con filas malas no es un error: es el caso normal. Los archivos
// reales del cliente traen columnas vacias al 100%, placeholders y tipos
// mixtos, y detener la entrega entera por una fila haria inservible la ingesta.
// Las buenas pasan a la forma canonica, las malas al log de rechazos, y
// NINGUNA se descarta.
//
// El lote viaja al repositorio en UNA sola llamada, valido y rechazado
// mezclados, precisamente para que las dos escrituras sean el mismo hecho: un
// lote guardado a medias deja una entrega cuyo recuento no cuadra con el
// archivo, y nadie sabria cual de las dos mitades falta.
//
// # Por que el Reporte entero y no su id
//
// La fila hereda de la entrega las DOS cosas que la atan a ella: el reporte y
// la fuente. `usos.fuente` es TEXT NOT NULL sin DEFAULT, asi que la cadena
// vacia lo satisface sin ruido, y RepositorioIdentificacion.Alias indexa por
// fuente: una fila con fuente vacia no casaria con ningun alias NUNCA, y el
// sintoma no seria un error sino un catalogo que parece incompleto. Quien llama
// acaba de recibir el Reporte de GuardarReporte, asi que pedirlo entero no le
// cuesta nada.
//
// # Los valores por defecto los pone este metodo
//
// Escalon, ONI y Emisiones no son columna de ningun fichero del cliente, asi
// que un adaptador de formato (#25) que mapee lo que hay en el archivo los deja
// en el valor cero de Go. Sus DEFAULT del esquema no llegan a aplicarse -el
// adaptador de persistencia manda el valor siempre-, asi que los que valen son
// estos.
func (i Ingesta) GuardarUsos(ctx context.Context, rep Reporte, usos []UsoPersistido) ([]UsoPersistido, error) {
	if len(usos) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(rep.ID) == "" {
		return nil, fmt.Errorf("%w: falta el reporte del que salen las filas", ErrReporteInvalido)
	}

	lote := make([]UsoPersistido, len(usos))
	var rechazados []UsoPersistido

	for n, u := range usos {
		u.ReporteID = rep.ID
		u.Fuente = rep.Fuente
		if u.ID == "" {
			// Derivado del reporte y de la posicion en el lote: la fila puede
			// senalar la entrega y la linea exactas de las que salio (ADR
			// 0006). No se reutiliza ningun contador de la fuente -Id_Ntx y
			// companeros se renumeran en cada entrega-, pero el reporte si es
			// estable porque su id sale de la huella.
			u.ID = rep.ID + "-" + strconv.Itoa(n)
		}
		if u.Escalon == "" {
			// Lo que trae una fila recien parseada. Exigirle el vocabulario
			// del esquema a cada adaptador de formato seria filtrar la base
			// hacia afuera.
			u.Escalon = "pendiente"
		}
		if u.ObraID == "" {
			// A la salida de ingesta ninguna fila esta identificada: es lo que
			// dice el doc de Ingesta y lo que asume la cascada (ADR 0007). El
			// DEFAULT TRUE de la columna no llega a aplicarse porque
			// insertarUso manda el valor siempre, asi que el que vale es este.
			u.ONI = true
		}
		if u.Emisiones == 0 {
			// Igual que Escalon y ONI: el DEFAULT 1 de la columna no se aplica
			// porque insertarUso manda el valor siempre. Una parrilla real
			// nunca declara cero emisiones -la granularidad es la emision, no
			// la obra, y RD 9.1.1 las multiplica-, asi que el cero es el valor
			// vacio de Go, no un dato.
			u.Emisiones = 1
		}
		if u.RechazoMotivo == "" {
			// Un motivo que ya viene puesto lo escribio el adaptador de
			// formato, que vio cosas que aqui ya no se ven -coercion de tipo,
			// un placeholder como el `--` de episode_nbr-. Pisarlo con el
			// motivo generico perderia la unica explicacion util del log de
			// rechazos.
			u.RechazoMotivo = validarUso(u)
		}

		lote[n] = u
		if u.RechazoMotivo != "" {
			rechazados = append(rechazados, u)
		}
	}

	if err := i.Reportes.GuardarUsos(ctx, lote); err != nil {
		return nil, fmt.Errorf("guardar las filas del reporte %q: %w", rep.ID, err)
	}
	return rechazados, nil
}

// validarUso devuelve el motivo por el que una fila no es canonica, o "" si lo
// es.
//
// Cada regla refleja una restriccion de la tabla `usos`, y la duplicacion es
// el objetivo, no un descuido: una sola fila que viole un CHECK aborta el
// INSERT del lote ENTERO y se lleva por delante las filas buenas que la
// acompanan. Comprobarlo antes es lo que convierte "la entrega fallo" en "esta
// linea fallo, y por esto".
//
// El motivo NOMBRA EL CAMPO. La skill de ingesta lo pide explicitamente: el
// mensaje tiene que decir que falta o que esta mal formateado, porque es lo
// que permite volver a pedirle al cliente exactamente eso.
//
// Lo que NO se valida aqui: que obra_id exista. Eso es una clave foranea y
// mirarla costaria una consulta por fila; ademas, a la salida de ingesta
// ninguna fila trae obra, que es lo que comprueba la regla de coherencia de
// abajo. Identificar es trabajo de la cascada (ADR 0007).
func validarUso(u UsoPersistido) string {
	if strings.TrimSpace(u.Titulo) == "" {
		return "titulo vacio: sin titulo no hay nada que identificar"
	}
	switch u.Modalidad {
	case reparto.TV, reparto.Cine, reparto.OTT, reparto.Hotel:
	default:
		return fmt.Sprintf("modalidad %q fuera de tv|cine|ott|hotel", u.Modalidad)
	}
	if u.Escalon == "manual" {
		return "escalon manual: una resolucion manual necesita autor e instante, y no entra por ingesta"
	}
	if !escalones[u.Escalon] {
		return fmt.Sprintf("escalon %q desconocido", u.Escalon)
	}

	// El CHECK uso_resuelto_tiene_obra, dicho en Go: una fila identificada
	// apunta a una obra, y una en ONI no apunta a ninguna. Sin esto se puede
	// guardar una fila que dice "identificada" y no senala nada.
	switch {
	case u.ONI && u.ObraID != "":
		return "marcada oni y con obra_id a la vez"
	case !u.ONI && u.ObraID == "":
		return "marcada como identificada y sin obra_id"
	}

	// Los CHECK de no negatividad de las columnas de medida. Una medida
	// negativa no es un uso pequeno: es un dato roto, y ponderaria a la baja.
	negativas := []struct {
		campo string
		valor decimal.Decimal
	}{
		{"duracion_min", u.DuracionMin},
		{"rating", u.Rating},
		{"taquilla", u.Taquilla},
		{"vistas", u.Vistas},
		{"minutos_vistos", u.MinutosVistos},
		{"pb", u.PB},
	}
	for _, n := range negativas {
		if n.valor.IsNegative() {
			return fmt.Sprintf("%s negativa: %s", n.campo, n.valor)
		}
	}
	if u.Emisiones < 0 {
		return fmt.Sprintf("emisiones negativas: %d", u.Emisiones)
	}
	return ""
}
