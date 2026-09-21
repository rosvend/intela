package anomalias

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Detectar corre los seis detectores sobre un periodo armado y devuelve todo
// lo que encontro, ordenado.
//
// El orden final es (Tipo, RefTipo, RefID, RefTitular) y no el de deteccion:
// asi dos pasadas sobre el mismo dato devuelven la MISMA lista aunque un
// detector cambie de sitio, que es lo que el ADR 0005 pide de cualquier cosa
// que alimente una corrida. Ninguna funcion de este fichero recorre un mapa:
// los mapas solo se consultan, y lo que se recorre son las listas de entrada.
//
// Una lista vacia es la respuesta normal de un periodo limpio, no un fallo.
func Detectar(p Periodo) []Hallazgo {
	hallazgos := make([]Hallazgo, 0)
	hallazgos = append(hallazgos, deteccionONI(p.Usos)...)
	hallazgos = append(hallazgos, duplicadosPorHuella(p.Periodo, p.Entregas)...)
	hallazgos = append(hallazgos, duplicadosPorRegistro(p.Usos)...)
	hallazgos = append(hallazgos, titularesSinPorcentaje(p.Obras)...)
	hallazgos = append(hallazgos, retencionPorDeclaracionIncompleta(p.Obras)...)
	hallazgos = append(hallazgos, tipoObraSinMapear(p.Usos)...)

	slices.SortFunc(hallazgos, func(a, b Hallazgo) int {
		if c := strings.Compare(a.Tipo, b.Tipo); c != 0 {
			return c
		}
		if c := strings.Compare(a.RefTipo, b.RefTipo); c != 0 {
			return c
		}
		if c := strings.Compare(a.RefID, b.RefID); c != 0 {
			return c
		}
		return strings.Compare(a.RefTitular, b.RefTitular)
	})
	return hallazgos
}

// ---------------------------------------------------------------------------
// 1. ONI

// deteccionONI levanta una alerta por cada fila que la cascada miro y no
// reconocio.
//
// # El filtro es el escalon, NUNCA la bandera `oni`
//
// `usos.oni` es BOOLEAN NOT NULL DEFAULT TRUE (migracion 00001): una fila
// recien ingerida llega con escalon='pendiente' Y oni=TRUE a la vez. Filtrar
// por la bandera contaria como "obra no identificada" todo lo que todavia no
// se ha INTENTADO identificar, que es otra cosa -- y en una entrega recien
// subida son todas las filas. El adaptador ya lo advierte en
// [postgres.Store.UsosSinResolver], por la misma razon.
//
// # `excluido` no es ONI
//
// La migracion 00007 anadio el escalon `excluido` para `R-27` / `RD 9.5`:
// una fila de un canal fuera del repertorio de REDES SGC se queda sin obra y
// con oni=false a proposito, porque estar fuera del repertorio no es no haber
// sido identificada. Levantarle alerta de ONI mandaria a la cola manual
// trabajo que no existe.
//
// Tampoco `manual`: esa fila ya la resolvio una persona.
func deteccionONI(usos []Uso) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.Escalon != identificacion.EscalonONI {
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoONI,
			RefTipo: RefUso,
			RefID:   u.ID,
			Detalle: fmt.Sprintf(
				"la cascada no reconocio %q (fuente %q, entrega %q): queda en la cola manual y su parte se retiene",
				u.Titulo, u.Fuente, u.ReporteID),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 2. Duplicado por huella de archivo

// duplicadosPorHuella levanta alerta sobre las entregas DEL PERIODO cuyos
// bytes ya llegaron en otra entrega.
//
// # Lo que esta funcion NO detecta, porque seria codigo muerto
//
// `reportes` tiene UNIQUE (sha256, fuente): la MISMA fuente entregando
// exactamente los mismos bytes ya falla al insertar, y el adaptador traduce
// esa violacion a [aplicacion.ErrReporteDuplicado]. Esa puerta ya esta
// cerrada y comprobarla aqui no encontraria nunca nada.
//
// Lo que el UNIQUE deja pasar, y es lo que se busca aqui, es la colision
// entre entregas que NO comparten fuente: el mismo archivo reenviado bajo
// otro nombre de fuente, o un reporte reexportado que dos proveedores
// entregan por separado. Como el periodo tampoco esta en la clave, la otra
// pata de la colision puede estar en otro mes, y por eso `entregas` son todas
// las conocidas y no solo las del periodo evaluado.
//
// Se levanta alerta sobre CADA entrega del periodo que colisione, no solo
// sobre la segunda: las dos ponderan el periodo, y quien resuelve tiene que
// decidir cual se queda. La otra pata se nombra en el detalle para que esa
// decision no exija abrir la base.
func duplicadosPorHuella(periodo string, entregas []Entrega) []Hallazgo {
	porHuella := make(map[string][]Entrega, len(entregas))
	for _, e := range entregas {
		if e.SHA256 == "" {
			continue
		}
		porHuella[e.SHA256] = append(porHuella[e.SHA256], e)
	}

	out := make([]Hallazgo, 0)
	for _, e := range entregas {
		if e.Periodo != periodo {
			continue
		}
		colisiones := porHuella[e.SHA256]
		if len(colisiones) < 2 {
			continue
		}
		otras := make([]string, 0, len(colisiones)-1)
		for _, c := range colisiones {
			if c.ID == e.ID {
				continue
			}
			otras = append(otras, fmt.Sprintf("%s (fuente %q, periodo %q)", c.ID, c.Fuente, c.Periodo))
		}
		out = append(out, Hallazgo{
			Tipo:    TipoDuplicadoArchivo,
			RefTipo: RefReporte,
			RefID:   e.ID,
			Detalle: fmt.Sprintf(
				"la entrega %q (fuente %q) trae los mismos bytes que %s; sha256 %s. El UNIQUE (sha256, fuente) no lo impide porque no comparten fuente ni periodo",
				e.ID, e.Fuente, strings.Join(otras, ", "), e.SHA256),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 3. Duplicado por registro logico

// duplicadosPorRegistro levanta alerta sobre las filas de un periodo que
// repiten un registro ya visto en OTRO archivo de la misma fuente.
//
// # Que anade esto a lo que ya hace la ingesta
//
// El adaptador de formato ya rechaza el registro repetido DENTRO de un mismo
// archivo (`ingesta.Mapa.ClaveRegistro`), y esas filas ni siquiera llegan a
// `usos`: van al log de rechazos. Lo que ninguna lectura de un archivo suelto
// puede ver es la repeticion ENTRE archivos del mismo periodo -- dos entregas
// parciales de Caracol que se solapan una semana, por ejemplo --, y eso es
// justamente lo que infla los puntos de una obra y desinfla los de todas las
// demas del canal.
//
// # La clave llega derivada, y una clave vacia no compara
//
// [Uso.ClaveRegistro] la deriva la capa de aplicacion con el vocabulario del
// ADR 0018. Si llega vacia -- fuente sin clave declarada, o fila a la que le
// faltan los identificadores -- la fila NO se compara con nada. Agruparlas
// todas bajo la cadena vacia marcaria como duplicadas entre si todas las
// filas de las que no se sabe nada, que es el falso positivo mas caro
// posible: bloquearia el periodo entero.
//
// # La fuente entra en la clave
//
// Los ids de fuente NO cruzan entre fuentes (ADR 0018,
// docs/dominio/identificadores.md): que el `id=7` de cine coincida con el
// `id_ficha=7` de Caracol no significa nada. Sin la fuente en la clave, esa
// coincidencia seria un duplicado inventado.
//
// La alerta va sobre la fila REPETIDA, no sobre la primera: la primera es un
// registro legitimo y borrarla tambien perderia el hecho.
func duplicadosPorRegistro(usos []Uso) []Hallazgo {
	type primera struct{ usoID, reporteID string }

	vistas := make(map[string]primera, len(usos))
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.ClaveRegistro == "" {
			continue
		}
		clave := u.Fuente + "\x00" + u.ClaveRegistro
		antes, repetida := vistas[clave]
		if !repetida {
			vistas[clave] = primera{usoID: u.ID, reporteID: u.ReporteID}
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoDuplicadoRegistro,
			RefTipo: RefUso,
			RefID:   u.ID,
			Detalle: fmt.Sprintf(
				"el registro %s de la fuente %q ya venia en el uso %q (entrega %q); esta fila llego en la entrega %q y contaria el mismo hecho dos veces",
				u.ClaveRegistro, u.Fuente, antes.usoID, antes.reporteID, u.ReporteID),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 4. Titulares sin porcentaje

// titularesSinPorcentaje nombra a las PERSONAS a las que les falta parte
// declarada en una obra que SI tiene declaracion abierta.
//
// # El reparto de responsabilidades con [retencionPorDeclaracionIncompleta]
//
// Los dos detectores miran el mismo hecho desde dos alturas distintas y esto
// es deliberado, no un solapamiento que se haya colado:
//
//   - [retencionPorDeclaracionIncompleta] habla de la OBRA y del DINERO: lo
//     declarado no suma 100, asi que se retiene el total (`R-04`). Una alerta
//     por obra, referencia a la obra. Es lo que distribucion informa a
//     contabilidad.
//   - Esto habla del TITULAR y de la GESTION: quien tiene que firmar para
//     desbloquearla. Una alerta por (obra, IPI), referencia a la obra mas el
//     IPI. Es a quien distribucion tiene que llamar.
//
// Una obra a la que le falta un coautor levanta las dos, y no son el mismo
// aviso: la primera dice cuanto dinero esta parado, la segunda dice a quien
// hay que perseguir. Colapsarlas en una sola perderia el nombre, que es lo
// unico accionable de las dos.
//
// # Por que una obra SIN declaracion no entra aqui
//
// Porque no hay nada que decir de cada coautor por separado: nadie declaro
// nada, y emitir una alerta por cada uno inundaria el tablero con N avisos
// que dicen lo mismo. Ese caso lo cubre entero
// [retencionPorDeclaracionIncompleta] con UNA alerta. Una version ABIERTA Y
// VACIA si entra -- el esquema lo permite, `VigentesDeObras` usa LEFT JOIN a
// proposito para no confundirla con la ausencia -- porque ahi si hay una
// version concreta a la que le faltan partes concretas.
//
// # Los dos casos que levanta
//
//  1. Un IPI de `obra_coautores` que no aparece como parte. El catalogo dice
//     que esa persona escribio la obra y la declaracion no le asigna nada.
//  2. Una parte con el IPI vacio. `declaraciones.ipi` es TEXT NOT NULL sin
//     CHECK de no-vacio (migracion 00001), asi que el esquema lo admite; el
//     porcentaje esta declarado pero no dice A QUIEN se le paga, que aguas
//     abajo es `resultados_titular.ipi` en blanco.
//
// Lo que NO busca es un porcentaje nulo o cero: el CHECK
// (porcentaje > 0 AND porcentaje <= 100) y [repertorio.NuevaDeclaracion] ya
// lo impiden en los dos extremos. La anomalia real es la fila que FALTA.
func titularesSinPorcentaje(obras []Obra) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, o := range obras {
		if !o.Declarada {
			continue
		}

		declarados := make(map[string]bool, len(o.Declaracion.Partes))
		for _, p := range o.Declaracion.Partes {
			declarados[p.IPI] = true
		}

		// Caso 1: coautor del catalogo sin parte. Se recorre la lista de
		// coautores -- que llega ordenada --, no el mapa.
		for _, ipi := range o.CoautoresIPI {
			if ipi == "" || declarados[ipi] {
				continue
			}
			out = append(out, Hallazgo{
				Tipo:       TipoTitularSinPorcentaje,
				RefTipo:    RefObra,
				RefID:      o.ID,
				RefTitular: ipi,
				Detalle: fmt.Sprintf(
					"el coautor con IPI %s figura en el catalogo de la obra %q y no tiene parte en la declaracion vigente: su porcentaje no esta declarado y sin el no se le puede pagar (R-03)",
					ipi, o.ID),
			})
		}

		// Caso 2: parte sin IPI.
		for _, p := range o.Declaracion.Partes {
			if p.IPI != "" {
				continue
			}
			out = append(out, Hallazgo{
				Tipo:       TipoTitularSinPorcentaje,
				RefTipo:    RefObra,
				RefID:      o.ID,
				RefTitular: p.TitularID,
				Detalle: fmt.Sprintf(
					"la parte del titular %q en la obra %q declara %s%% y no trae IPI: el porcentaje esta, pero no dice a quien se le paga",
					p.TitularID, o.ID, p.Porcentaje.String()),
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 5. Retencion por declaracion incompleta (R-04 / RD 13.1.3)

// retencionPorDeclaracionIncompleta levanta alerta por cada obra del periodo
// cuya declaracion vigente no esta completa.
//
// # No es un error, es un estado
//
// `R-04` / `RD 13.1.3`: si los porcentajes declarados no suman exactamente
// 100%, no se reparte NADA de esa obra y se retiene el total. Nunca se
// prorratea y nunca se paga lo declarado dejando el resto. Que esto sea una
// anomalia de primera clase del tablero -- y no un error del pipeline -- es
// justamente lo que pide OE-5: es la cola que distribucion tiene que
// perseguir con los autores para desbloquear un pago que existe.
//
// Retiene ESA OBRA. No bloquea el periodo: ver [EsCritica].
//
// # El criterio es del dominio de repertorio, no de aqui
//
// [repertorio.Declaracion.Completa] ya decide los tres casos -- la suma no da
// 100, una parte no trae IPI, una parte no es positiva -- y es el mismo
// criterio que usa el catalogo para pintar el estado de una obra. Volver a
// sumar aqui abriria un segundo criterio de `R-04`, y el dia que divergieran
// la pantalla diria una cosa y el reparto haria otra.
//
// # La obra sin ninguna declaracion tambien retiene
//
// Y sale por aqui: la [repertorio.Declaracion] cero da Completa() == false.
// El detalle distingue los dos casos porque el arreglo es distinto -- ahi hay
// que declarar desde cero, aqui hay que completar --, pero el efecto sobre el
// dinero es el mismo y por eso es la misma alerta.
func retencionPorDeclaracionIncompleta(obras []Obra) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, o := range obras {
		if o.Declarada && o.Declaracion.Completa() {
			continue
		}

		detalle := fmt.Sprintf(
			"la obra %q no tiene ninguna Declaracion de Obra: se retiene el total de lo que le corresponda en este periodo (R-04, RD 13.1.3)",
			o.ID)
		if o.Declarada {
			suma := sumaDeclarada(o)
			detalle = fmt.Sprintf(
				"la declaracion vigente de la obra %q suma %s%% en %d parte(s) y no llega a 100: se retiene el TOTAL de esa obra, no se reparte la parte declarada (R-04, RD 13.1.3)",
				o.ID, suma, len(o.Declaracion.Partes))
		}

		out = append(out, Hallazgo{
			Tipo:    TipoReservaDeclaracionIncompleta,
			RefTipo: RefObra,
			RefID:   o.ID,
			Detalle: detalle,
		})
	}
	return out
}

// sumaDeclarada es solo para el texto del detalle. No decide nada: quien
// decide si la declaracion esta completa es [repertorio.Declaracion.Completa].
func sumaDeclarada(o Obra) string {
	suma := decimal.Zero
	for _, p := range o.Declaracion.Partes {
		suma = suma.Add(p.Porcentaje)
	}
	return suma.String()
}

// ---------------------------------------------------------------------------
// 6. tipo_obra sin mapear (RD 9.1.1)

// tipoObraSinMapear levanta alerta por cada fila identificada que no trae la
// categoria de `RD 9.1.1`.
//
// La tabla de ponderacion del reglamento esta indexada por cuatro categorias
// -- cinematografica 5.0, unitario 2.8, serie/telenovela 1.3, sketches 0.8 --
// y `usos.tipo_obra` admite la cadena vacia (CHECK de la migracion 00011):
// vacio significa "el mapa de columnas de esa fuente no la trae", que hoy es
// el caso de la parrilla de Caracol entera, porque la correspondencia entre
// su `TIPO`/`SubGenero` y las cuatro categorias es la pregunta P-05 y el
// cliente no la ha contestado (`ingesta.MapaCaracol`). Inventarla al ingerir
// seria peor: no habria forma de distinguir lo mapeado de lo supuesto.
//
// El efecto aguas abajo no es que la obra puntue cero: `reparto` ABORTA la
// corrida de TV y la de suscripcion con ErrRepartoInvalido en cuanto ve el
// tipo vacio (reparto/estrategia.go, `case ""`). Esta alerta es el preaviso
// de esa parada, en el momento en que todavia se puede pedir el dato.
//
// # Solo las filas con obra identificada
//
// Una fila pendiente, ONI o excluida no llega al motor -- `UsosDeCanal` filtra
// por `obra_id IS NOT NULL` -- asi que su tipo_obra no pondera nada, y su
// problema es otro y ya tiene su propia alerta. Sin este filtro, cada fila
// ONI de Caracol levantaria dos alertas por el mismo hecho.
func tipoObraSinMapear(usos []Uso) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.ObraID == "" || u.TipoObra != "" {
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoTipoObraSinMapear,
			RefTipo: RefUso,
			RefID:   u.ID,
			Detalle: fmt.Sprintf(
				"el uso %q de la obra %q (fuente %q) no trae tipo_obra: RD 9.1.1 pondera por cuatro categorias y sin una de ellas la corrida de TV se aborta",
				u.ID, u.ObraID, u.Fuente),
		})
	}
	return out
}
