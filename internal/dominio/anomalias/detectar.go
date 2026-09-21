package anomalias

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
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
			// La frase explica el MECANISMO y no afirma nada sobre este par.
			// La anterior decia "porque no comparten fuente ni periodo" como
			// texto fijo, y el periodo si puede coincidir -- de hecho coincide
			// siempre que las dos patas caen en el mes evaluado --, con lo que
			// el propio detalle se desmentia dos lineas mas arriba, donde
			// `otras` ya imprime el periodo de cada una.
			Detalle: fmt.Sprintf(
				"la entrega %q (fuente %q) trae los mismos bytes que %s; sha256 %s. El UNIQUE (sha256, fuente) de `reportes` no lo impide: solo cierra la puerta a la MISMA fuente reenviando los mismos bytes, y el periodo no entra en esa clave",
				e.ID, e.Fuente, strings.Join(otras, ", "), e.SHA256),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 3. Duplicado por registro logico

// duplicadosPorRegistro levanta alerta sobre las filas de un periodo que
// repiten un registro ya visto en otra fila de la misma fuente.
//
// # Que anade esto a lo que ya hace la ingesta
//
// El adaptador de formato ya rechaza el registro repetido DENTRO de un mismo
// archivo (`ingesta.Mapa.ClaveRegistro`), y esas filas ni siquiera llegan a
// `usos`: van al log de rechazos. Lo que ninguna lectura de un archivo suelto
// puede ver es la repeticion ENTRE archivos del mismo periodo -- dos entregas
// parciales de Caracol que se solapan una semana, por ejemplo --, y eso es
// justamente lo que infla los puntos de una obra y desinfla los de todas las
// demas del canal. Esa es la repeticion que este detector EXISTE para cazar.
//
// # La comparacion NO filtra por entrega, y es deliberado
//
// El parrafo de arriba dice de donde viene el caso interesante, no lo que la
// funcion compara: aqui entran todas las filas del periodo de la misma fuente,
// vengan de la entrega que vengan. Filtrar por `ReporteID` distinto seria
// confiar en que la puerta de la ingesta no se puede saltar nunca, y las dos
// puertas no miran lo mismo -- `claveDe` compara celdas CRUDAS del archivo y
// esta funcion una clave ya normalizada, asi que dos filas que el adaptador
// dejo pasar por una diferencia de formato pueden ser la misma aqui. Un
// duplicado dentro de una misma entrega sigue contando el mismo hecho dos
// veces, asi que se avisa igual.
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
// Callar no es gratis y no se deja mudo: [SinClaveDeRegistro] las cuenta y la
// cifra sube al resumen de la pasada. Ver su doc.
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

// SinClaveDeRegistro cuenta las filas que [duplicadosPorRegistro] NO pudo
// cotejar con ninguna otra.
//
// No comparar esas filas es lo correcto -- agruparlas bajo la cadena vacia
// marcaria como duplicadas entre si todas las filas de las que no se sabe nada
// y bloquearia el periodo entero --, pero dejarlo sin decir convierte el
// acierto en un punto ciego MUDO: el tablero muestra cero duplicados y nadie
// sabe sobre cuantas filas no se miro.
//
// Y no es hipotetico. `Hora` es `Requerida: false` en `MapaCaracol` y a la vez
// componente de su `ClaveRegistro`, asi que una fila sin hora se queda sin
// clave. Ahi hay ademas una asimetria real: dentro de un archivo, `claveDe`
// compara las celdas CRUDAS y SI caza dos filas con `Hora` vacia; entre
// archivos la clave sale vacia y no se caza ninguna. El mismo par de filas, en
// dos archivos, pasa -- y `duplicado_registro` es de los tipos criticos.
//
// La cifra sube a [aplicacion.ResumenEvaluacion] para que el hueco se pueda
// mirar. Un numero mayor que cero no es un fallo: es cuanta de la entrega
// quedo fuera del cotejo.
func SinClaveDeRegistro(usos []Uso) int {
	n := 0
	for _, u := range usos {
		if u.ClaveRegistro == "" {
			n++
		}
	}
	return n
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
				RefTitular: PrefijoIPI + ipi,
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
				RefTitular: PrefijoTitular + p.TitularID,
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
			"la obra %q no tiene ninguna Declaracion de Obra%s: se retiene el total de lo que le corresponda en este periodo (R-04, RD 13.1.3)",
			o.ID, aQuienReclamar(o))
		if o.Declarada {
			detalle = fmt.Sprintf(
				"la declaracion vigente de la obra %q no esta completa: %s. Se retiene el TOTAL de esa obra, no se reparte la parte declarada (R-04, RD 13.1.3)",
				o.ID, motivoIncompleta(o.Declaracion))
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

// motivoIncompleta narra POR QUE [repertorio.Declaracion.Completa] dijo que no.
//
// Es solo para el texto del detalle y no decide nada -- quien decide sigue
// siendo `Completa()` --, pero tiene que narrar el motivo REAL: `Completa()`
// falla por tres cosas distintas (una parte sin IPI, una parte no positiva, y
// la suma que no da 100 exactos) y el detalle anterior contaba siempre la
// tercera. Una obra cuyas partes suman 100 clavado pero a la que le falta un
// IPI salia como "suma X% y no llega a 100" con X igual a 100: distribucion
// perseguia un porcentaje que estaba bien mientras el dato que falta es un
// identificador de persona.
//
// El orden de las comprobaciones es el mismo que el de `Completa()`, para que
// el motivo que se narra sea el que de verdad corto.
func motivoIncompleta(d repertorio.Declaracion) string {
	if len(d.Partes) == 0 {
		return "la version vigente esta abierta y no tiene ninguna parte declarada"
	}

	sinIPI := make([]string, 0)
	noPositivas := make([]string, 0)
	suma := decimal.Zero
	for _, p := range d.Partes {
		if p.IPI == "" {
			sinIPI = append(sinIPI, p.TitularID)
		}
		if p.Porcentaje.LessThanOrEqual(decimal.Zero) {
			noPositivas = append(noPositivas, p.TitularID)
		}
		suma = suma.Add(p.Porcentaje)
	}

	motivos := make([]string, 0, 3)
	if len(sinIPI) > 0 {
		motivos = append(motivos, fmt.Sprintf(
			"%d de %d parte(s) no traen IPI (titular(es) %s), asi que no dicen a quien se le paga",
			len(sinIPI), len(d.Partes), strings.Join(sinIPI, ", ")))
	}
	if len(noPositivas) > 0 {
		motivos = append(motivos, fmt.Sprintf(
			"%d parte(s) no son positivas (titular(es) %s)",
			len(noPositivas), strings.Join(noPositivas, ", ")))
	}
	if !suma.Equal(decimal.NewFromInt(100)) {
		motivos = append(motivos, fmt.Sprintf(
			"lo declarado suma %s%% en %d parte(s) y R-04 exige 100 exactos",
			suma.String(), len(d.Partes)))
	}
	if len(motivos) == 0 {
		// Inalcanzable mientras este detector solo llame aqui con una
		// declaracion que Completa() rechazo. Si alguien invierte esa
		// condicion, mas vale que el texto lo diga a que invente un motivo.
		return "no se pudo determinar el motivo"
	}
	return strings.Join(motivos, "; ")
}

// aQuienReclamar nombra a los coautores del catalogo cuando la obra no tiene
// NINGUNA declaracion.
//
// [titularesSinPorcentaje] calla en ese caso -- no hay version abierta a la que
// le falten partes concretas --, asi que sin esto la obra SIN declaracion, que
// es el caso PEOR, era la unica que se quedaba sin un solo nombre: la misma
// obra con una version abierta y vacia emite N alertas con nombre y apellido.
// `R-04` retiene el total en los dos casos y para desbloquearlo hay que llamar
// a alguien; el nombre va aqui, en la alerta que si se emite.
//
// Devuelve la cadena vacia -- y no una frase -- cuando no hay coautores
// registrados: `NuevaObra` exige al menos uno, pero una fila escrita por SQL
// directo podria no tenerlos, y "hay que hablar con: nadie" no ayuda.
func aQuienReclamar(o Obra) string {
	ipis := make([]string, 0, len(o.CoautoresIPI))
	for _, ipi := range o.CoautoresIPI {
		if ipi == "" {
			continue
		}
		ipis = append(ipis, ipi)
	}
	if len(ipis) == 0 {
		return ""
	}
	return fmt.Sprintf(" y en el catalogo figura(n) %d coautor(es) a quien(es) reclamarla (IPI %s)",
		len(ipis), strings.Join(ipis, ", "))
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
// corrida con ErrRepartoInvalido en cuanto ve el tipo vacio
// (reparto/estrategia.go, `case ""`, introducido por #120 el 2026-09-18 junto
// con la propia `ponderacionTipo` -- antes de esa fecha no habia ponderacion
// por tipo en absoluto). Esta alerta es el preaviso de esa parada, en el
// momento en que todavia se puede pedir el dato.
//
// # Que el preaviso llegue ANTES que la parada es el punto
//
// Hoy esa parada no se esta produciendo, y no porque el dato este: porque la
// fila no llega al motor. `MapaCaracol` tampoco mapea `canal_id` -- CampoCanalID
// existe en `ingesta/mapa.go` y no lo usa ningun Mapa --, asi que `UsosDeCanal`
// devuelve cero filas para esas entregas. El hueco de `tipo_obra` esta LATENTE,
// y salta el dia que `canal_id` se puebla.
//
// Por eso este detector AVISA y no se intenta arreglar el dato por detras.
// Rellenar `tipo_obra` desde `obras.tipo` al identificar -- que es lo que parece
// el arreglo de fondo -- se midio contra Postgres real y empeora el conjunto:
// `MapaCaracol` tampoco mapea `rating`, que queda en 0, y `puntosTV` multiplica
// por el, asi que la corrida deja de abortar y pasa a repartir CERO con error
// nil (`noDistribuido` = la bolsa entera). Un fallo ruidoso convertido en uno
// silencioso. Los tres huecos -- `tipo_obra`, `canal_id`, `rating` -- se cierran
// juntos y en su propia issue.
//
// # Solo las filas con obra identificada
//
// Una fila pendiente, ONI o excluida no llega al motor -- `UsosDeCanal` filtra
// por `obra_id IS NOT NULL` -- asi que su tipo_obra no pondera nada, y su
// problema es otro y ya tiene su propia alerta. Sin este filtro, cada fila
// ONI de Caracol levantaria dos alertas por el mismo hecho.
//
// # Solo las modalidades cuyo motor LEE el campo
//
// `tipo_obra` solo lo consulta `reparto.ponderacionTipo`, y a esa funcion solo
// se llega por TV, suscripcion y hotel (ver [ModalidadPonderaPorTipoObra]). Una
// fila de OTT, cine, teatro o transporte con el tipo vacio no rompe nada: su
// estrategia no lo mira.
//
// Sin este filtro la alerta era ademas ACTIVAMENTE danina, no solo ruido.
// `MapaNetflix` no declara `CampoTipoObra`, asi que TODA fila de Netflix llega
// con el tipo vacio: un periodo de OTT levantaba N alertas marcadas como
// criticas por [EsCritica] y repartia perfectamente. Como la compuerta de #34
// consume `CriticasAbiertas`, ese periodo no se habria podido cerrar NUNCA por
// un campo que su corrida no lee.
//
// Es ademas lo que mantiene honesto a [EsCritica]: si el detector solo dispara
// sobre filas cuya corrida se aborta, entonces toda alerta de este tipo es de
// verdad bloqueante, y la criticidad sigue siendo funcion del tipo sin
// necesidad de una segunda condicion en el SQL.
func tipoObraSinMapear(usos []Uso) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.ObraID == "" || u.TipoObra != "" {
			continue
		}
		if !ModalidadPonderaPorTipoObra(u.Modalidad) {
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoTipoObraSinMapear,
			RefTipo: RefUso,
			RefID:   u.ID,
			// "en cuanto esta fila entre en una corrida" y no "la corrida se
			// aborta": las dos cosas no son la misma, y hoy la segunda seria
			// falsa. `MapaCaracol` tampoco mapea `canal_id`, asi que
			// `UsosDeCanal` no devuelve estas filas y el motor ni siquiera
			// arranca sobre ellas. La alerta es un PREAVISO -- que es para lo
			// que sirve, mientras todavia se puede pedir el dato -- y el texto
			// tiene que decir eso y no afirmar una parada que no esta pasando.
			Detalle: fmt.Sprintf(
				"el uso %q de la obra %q (fuente %q, modalidad %q) no trae tipo_obra: RD 9.1.1 pondera por cuatro categorias y el motor de %s aborta la corrida entera (ErrRepartoInvalido) en cuanto esta fila entre en una",
				u.ID, u.ObraID, u.Fuente, u.Modalidad, u.Modalidad),
		})
	}
	return out
}
