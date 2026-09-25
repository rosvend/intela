package anomalias

import (
	"slices"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Tipos de anomalia. Son el vocabulario que viaja por HTTP, no un enum
// interno: los cinco primeros los fijo el tablero de la #104
// (`web/src/reparto/tipos.ts`, `TipoDeAlerta`) antes de que existiera este
// paquete, y cambiar una grafia deja el tablero pintando la etiqueta cruda.
//
// TipoReservaDeclaracionIncompleta se llama asi por el contrato y NO por lo
// que mide. "Reserva" en este sistema es ademas el 5% por errores tecnicos de
// `R-07` / `RD 14.5.4`, que es otra cosa y vive en reparto. Lo que este tipo
// nombra es la RETENCION del total de una obra por `R-04` / `RD 13.1.3`: por
// eso el detector de Go se llama [retencionPorDeclaracionIncompleta] y la
// palabra "reserva" se queda unicamente en el string que viaja por la red.
const (
	TipoONI                          = "oni"
	TipoDuplicadoArchivo             = "duplicado_archivo"
	TipoDuplicadoRegistro            = "duplicado_registro"
	TipoTitularSinPorcentaje         = "titular_sin_porcentaje"
	TipoReservaDeclaracionIncompleta = "reserva_declaracion_incompleta"
	TipoTipoObraSinMapear            = "tipo_obra_sin_mapear"
)

// Tipos devuelve los seis, en el orden en que corren los detectores. El orden
// es parte del contrato de [Detectar]: dos pasadas producen la misma lista.
func Tipos() []string {
	return []string{
		TipoONI,
		TipoDuplicadoArchivo,
		TipoDuplicadoRegistro,
		TipoTitularSinPorcentaje,
		TipoReservaDeclaracionIncompleta,
		TipoTipoObraSinMapear,
	}
}

// EsTipo dice si el string es uno de los seis. La lista es cerrada por lo
// mismo que la de ids_fuente (ADR 0018): lo que la base guarda con otra
// grafia no lo encuentra ningun filtro, y nada falla de forma visible.
func EsTipo(tipo string) bool { return slices.Contains(Tipos(), tipo) }

// Tipos de referencia al registro ofensor. Es el par (ref_tipo, ref_id) que
// ya usa `asientos` desde la migracion 00001: el mismo patron polimorfico,
// porque la referencia apunta a tablas distintas segun el detector.
const (
	RefUso     = "uso"
	RefReporte = "reporte"
	RefObra    = "obra"
)

// Espacios de nombres de [Hallazgo.RefTitular].
//
// `titular_sin_porcentaje` referencia dos cosas distintas segun el caso: un
// IPI de `obra_coautores` cuando falta la parte de un coautor, y un
// `titulares.id` cuando la parte existe y no trae IPI. Los dos son TEXT y
// comparten columna, asi que sin prefijo la clave natural de `alertas` los
// confunde en cuanto coinciden -- y con ON CONFLICT DO NOTHING uno de los dos
// hallazgos se pierde sin que nada falle. Ver [Hallazgo.RefTitular].
const (
	PrefijoIPI     = "ipi:"
	PrefijoTitular = "titular:"
)

// Hallazgo es una anomalia detectada, sin identidad de fila y sin instante.
//
// No es la alerta persistida: no trae id ni `cuando` ni estado de resolucion.
// Esos tres los pone la capa que escribe, porque los tres salen de fuera del
// dominio -- el id lo genera la base, el instante entra por el puerto Reloj
// (ADR 0002) y la resolucion es una decision humana posterior (#39).
type Hallazgo struct {
	// Tipo es uno de los seis de arriba.
	Tipo string

	// RefTipo y RefID son el registro EXACTO que disparo la anomalia, que es
	// criterio de aceptacion del issue: sin el, una alerta dice que algo pasa
	// y no donde.
	RefTipo string
	RefID   string

	// RefTitular es la segunda coordenada del registro ofensor, CON su espacio
	// de nombres por delante: [PrefijoIPI] o [PrefijoTitular]. Solo lo rellena
	// [TipoTitularSinPorcentaje].
	//
	// Existe porque el registro que falta es una fila de `declaraciones`, cuya
	// identidad es (obra, titular): con la obra sola, dos coautores ausentes de
	// la MISMA obra serian la misma alerta y uno de los dos nombres se
	// perderia.
	//
	// # Por que lleva prefijo y no el identificador pelado
	//
	// Los dos casos del detector referencian espacios de nombres DISTINTOS: el
	// caso 1 nombra un IPI (`obra_coautores.ipi`) y el caso 2 un
	// `titulares.id`. Sin discriminar, los dos caian en la misma columna, y la
	// clave natural de `alertas` -- UNIQUE (periodo, tipo, ref_tipo, ref_id,
	// ref_titular) con ON CONFLICT DO NOTHING -- colapsaba los dos hallazgos en
	// cuanto un `titulares.id` coincidia con un IPI. Uno de los dos
	// desaparecia SIN RUIDO, y cual sobrevivia no lo fijaba nada: el orden de
	// [Detectar] empata en las cuatro claves y `slices.SortFunc` no es estable.
	//
	// Con el prefijo, los dos espacios no se pueden pisar y la referencia dice
	// ademas QUE es lo que nombra, que es lo que la bandeja de #39 necesita
	// para llevar a quien resuelve al registro correcto.
	RefTitular string

	// Detalle es la frase que lee una persona de distribucion. Lleva las
	// cifras concretas -- que suma da, que otra entrega colisiona, en que fila
	// venia el registro repetido -- porque quien persigue la alerta tiene que
	// poder actuar sin abrir la base.
	Detalle string
}

// EsCritica dice si el tipo BLOQUEA la distribucion del periodo.
//
// El issue lo pide explicito ("criticals block, informational ones do not") y
// la linea no es "que tan grave suena", sino si dejar la anomalia sin
// resolver hace que las cifras del periodo salgan MAL:
//
//   - duplicado_archivo y duplicado_registro: SI. Una emision contada dos
//     veces infla los puntos de su obra, y el valor punto de `RD 9.1.1` es un
//     cociente: la obra duplicada se lleva de mas y TODAS las demas del mismo
//     canal se llevan de menos. No hay forma de verlo en el resultado.
//
//   - tipo_obra_sin_mapear: SI. `RD 9.1.1` pondera por las cuatro categorias,
//     y una fila sin clasificar no tiene ponderacion que aplicar. El motor de
//     TV -- y el de suscripcion y hotel, que pasan por el mismo `puntosTV` --
//     ABORTAN la corrida entera con ErrRepartoInvalido (reparto/estrategia.go,
//     rama `case ""`), asi que esto es ademas el preaviso de un fallo duro.
//
//     Que sea critico SIEMPRE se sostiene en que [tipoObraSinMapear] solo
//     dispara sobre las modalidades cuyo motor lee el campo: sin ese filtro,
//     esta rama marcaria como bloqueante una fila de OTT a la que el tipo
//     vacio no le afecta en nada.
//
//   - oni: NO. Una obra no identificada es una ETAPA del diseno (ADR 0007),
//     no un fallo: su parte queda en reserva y se libera si se resuelve antes
//     de la prescripcion de tres anos (`R-19`, `RD 13.8`). Bloquear el
//     periodo por ONI seria no repartir nunca.
//
//   - reserva_declaracion_incompleta: NO. `R-04` / `RD 13.1.3` es un ESTADO
//     VALIDO del modelo: se retiene el TOTAL de esa obra y el resto del
//     periodo se reparte con normalidad.
//
//   - titular_sin_porcentaje: NO. Es la CAUSA de la anterior, con nombre y
//     apellidos; el efecto sobre el dinero ya lo cubre la retencion.
func EsCritica(tipo string) bool {
	switch tipo {
	case TipoDuplicadoArchivo, TipoDuplicadoRegistro, TipoTipoObraSinMapear:
		return true
	default:
		return false
	}
}

// Modalidades cuyo motor consulta `tipo_obra`. El nombre va en minusculas
// porque es el valor de `usos.modalidad` y el del CHECK de la migracion 00011.
//
// Son estas tres y nada mas, y se puede comprobar leyendo el motor:
// `reparto.ponderacionTipo` -- la UNICA funcion que mira `TipoObra` -- solo la
// llama `puntosTV`, y a `puntosTV` se llega desde `case TV:` (motor.go) y desde
// `repartirSuscripcionOrden`, que sirve a `case Suscripcion, Hotel:`.
// `puntosCineTeatro`, `puntosTransporte` y `puntosOTT` no leen `TipoObra` en
// ninguna linea.
const (
	ModalidadTV          = "tv"
	ModalidadSuscripcion = "suscripcion"
	ModalidadHotel       = "hotel"
)

// ModalidadPonderaPorTipoObra dice si la corrida de esa modalidad LEE
// `tipo_obra`.
//
// Existe para que [tipoObraSinMapear] no avise de un campo que la corrida de
// esa fila no va a mirar. Sin esta distincion, un periodo de Netflix -- cuyo
// mapa no trae la columna, asi que TODAS sus filas llegan con el tipo vacio --
// levantaba una alerta CRITICA por fila y repartia perfectamente: la compuerta
// de #34 lee [aplicacion.Anomalias.CriticasAbiertas], que contaria esas N, y no
// abriria nunca.
//
// Una modalidad desconocida cuenta como que SI pondera, y es deliberado: el
// vocabulario lo cierra `reparto.ParseModalidad` y el CHECK de `usos.modalidad`
// (00011), asi que llegar aqui con otra cosa ya es un fallo en otro sitio.
// Ante la duda se avisa, que es el lado ruidoso -- callar dejaria pasar en
// silencio justamente la fila de la que no se sabe nada.
func ModalidadPonderaPorTipoObra(modalidad string) bool {
	pondera, conocida := ponderaPorTipoObra[modalidad]
	return pondera || !conocida
}

// ModalidadesClasificadas devuelve, ordenadas, las modalidades de [ponderaPorTipoObra]; la prueba de aplicacion las cruza con reparto.
func ModalidadesClasificadas() []string {
	out := make([]string, 0, len(ponderaPorTipoObra))
	for m := range ponderaPorTipoObra {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// ponderaPorTipoObra clasifica cada modalidad de `reparto` (copiada en strings: este paquete no puede importarlo).
var ponderaPorTipoObra = map[string]bool{
	ModalidadTV:          true,
	ModalidadSuscripcion: true,
	ModalidadHotel:       true,
	"cine":               false,
	"teatro":             false,
	"transporte":         false,
	"ott":                false,
}

// Uso es una fila de reporte tal como la mira este paquete.
//
// Es una PROYECCION y no `aplicacion.UsoPersistido`: de las treinta y tantas
// columnas de `usos` aqui hacen falta siete, y recibir el tipo entero ataria
// el dominio a la forma de la tabla. El dinero no aparece porque no existe:
// un reporte de uso pondera la bolsa, no la aporta.
type Uso struct {
	ID        string
	ReporteID string
	Fuente    string
	Titulo    string

	// Escalon es el de la cascada (identificacion.Escalon*). La anomalia de
	// ONI se decide por AQUI y nunca por la bandera `usos.oni`: ver
	// [deteccionONI].
	Escalon string

	// Modalidad es la de `usos.modalidad` (`reparto.Modalidad`), como string
	// porque este paquete NO puede importar `reparto` -- la regla
	// `modulos-anomalias` de `.golangci.yml` se lo deniega (ADR 0003) -- y
	// porque aqui solo se compara, nunca se pondera.
	//
	// Hace falta porque no todas las modalidades leen los mismos campos, y sin
	// ella un detector no puede distinguir la fila que rompe una corrida de la
	// que no la roza: ver [ModalidadPonderaPorTipoObra] y [tipoObraSinMapear].
	Modalidad string

	// ObraID vacio significa que la fila no tiene obra identificada.
	ObraID string

	// TipoObra es la categoria de `RD 9.1.1`. Vacia = sin mapear.
	TipoObra string

	// ClaveRegistro es la clave logica del registro, YA derivada por la capa
	// de aplicacion a partir del contrato de ids_fuente (ADR 0018) y de los
	// campos canonicos. Vacia significa "de esta fuente no se sabe que
	// identifica un registro", y entonces la fila no se compara con ninguna:
	// ver [duplicadosPorRegistro].
	//
	// Llega derivada y no se deriva aqui porque el vocabulario de claves de
	// ids_fuente vive en `internal/aplicacion/idsfuente.go`, que es el unico
	// sitio donde ese contrato se decide, y el dominio no puede importar
	// aplicacion (ADR 0002). Duplicar la lista aqui es exactamente la deriva
	// que el ADR 0018 existe para impedir.
	ClaveRegistro string
}

// Entrega es el acuse de un reporte recibido: lo justo para decidir si sus
// bytes ya habian llegado antes.
type Entrega struct {
	ID      string
	Fuente  string
	Periodo string
	SHA256  string
}

// Obra es una obra del catalogo con lo que hace falta para juzgar su
// Declaracion de Obra.
type Obra struct {
	ID string

	// CoautoresIPI son los IPI de `obra_coautores`, ordenados. Figurar ahi NO
	// da derecho a cobrar (`R-02`, `R-03`): es la lista de quienes ESCRIBIERON
	// la obra, y sirve aqui para saber a quien le falta declarar.
	CoautoresIPI []string

	// Declarada dice si la obra tiene una version ABIERTA de declaracion.
	// Falso y `Declaracion` en su cero no son lo mismo que una version
	// abierta sin partes: la primera es "nadie declaro nunca", la segunda es
	// "se abrio una version y quedo vacia". Las dos retienen (`R-04`), pero
	// solo la segunda tiene a quien reclamarle dentro de una version.
	Declarada bool

	// Declaracion es la version vigente. Se recibe entera -- y no solo la
	// suma -- porque quien decide si esta completa es el dominio de
	// repertorio, no este paquete: [repertorio.Declaracion.Completa] ya cubre
	// los tres casos (suma distinta de 100, IPI vacio, parte no positiva) y
	// reimplementarlos aqui abriria un segundo criterio de `R-04`.
	Declaracion repertorio.Declaracion
}

// Periodo es todo lo que los seis detectores necesitan de un periodo.
//
// Lo arma la capa de aplicacion; aqui no se consulta nada. Las tres listas
// llegan ORDENADAS por su clave natural (usos y obras por id, entregas por
// recepcion e id): el resultado de [Detectar] se ordena igual antes de
// devolverse, pero un recorrido estable de la entrada es lo que hace que los
// mensajes -- "ya venia en el uso X" -- nombren siempre al mismo.
type Periodo struct {
	// Periodo es el que se evalua, en la forma `AAAA` o `AAAA-MM`.
	Periodo string

	// Usos son las filas canonicas de los reportes DE ESTE periodo.
	Usos []Uso

	// Entregas son TODAS las entregas conocidas, no solo las de este periodo.
	//
	// No es un descuido: el UNIQUE (sha256, fuente) de `reportes` no incluye
	// el periodo, asi que la colision que hay que cazar puede tener una pata
	// en otro mes. Solo se levanta alerta sobre las entregas DE ESTE periodo;
	// las de fuera estan para poder nombrarlas en el detalle.
	Entregas []Entrega

	// Obras son las obras que este periodo pondera -- las que aparecen como
	// `ObraID` de algun uso --, no el catalogo entero. Evaluar el catalogo
	// completo levantaria alertas sobre obras que nadie emitio en el periodo,
	// y la pregunta que responde este paso es "que impide repartir ESTE
	// periodo".
	Obras []Obra
}
