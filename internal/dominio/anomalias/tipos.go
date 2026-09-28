package anomalias

import (
	"slices"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Tipos de anomalia: vocabulario de red fijado por #104. "Reserva" en el nombre es del contrato; mide la retencion de R-04 (D7).
const (
	TipoONI                          = "oni"
	TipoDuplicadoArchivo             = "duplicado_archivo"
	TipoDuplicadoRegistro            = "duplicado_registro"
	TipoTitularSinPorcentaje         = "titular_sin_porcentaje"
	TipoReservaDeclaracionIncompleta = "reserva_declaracion_incompleta"
	TipoTipoObraSinMapear            = "tipo_obra_sin_mapear"
)

// Tipos devuelve los seis, en el orden en que corren los detectores.
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

// EsTipo dice si el string es uno de los seis (lista cerrada).
func EsTipo(tipo string) bool { return slices.Contains(Tipos(), tipo) }

// Tipos de referencia al registro ofensor: el par (ref_tipo, ref_id) de `asientos`.
const (
	RefUso     = "uso"
	RefReporte = "reporte"
	RefObra    = "obra"
)

// Espacios de nombres de [Hallazgo.RefTitular]: IPI y titulares.id comparten columna (D4).
const (
	PrefijoIPI     = "ipi:"
	PrefijoTitular = "titular:"
)

// Hallazgo es una anomalia detectada, sin id, instante ni resolucion (los pone la capa que escribe).
type Hallazgo struct {
	// Tipo es uno de los seis de arriba.
	Tipo string

	// RefTipo y RefID son el registro exacto que la disparo.
	RefTipo string
	RefID   string

	// RefTitular: segunda coordenada con prefijo; solo la usa titular_sin_porcentaje (D4).
	RefTitular string

	// Detalle: frase para distribucion, con las cifras concretas.
	Detalle string
}

// EsCritica dice si el tipo bloquea la distribucion del periodo (D7).
func EsCritica(tipo string) bool {
	switch tipo {
	case TipoDuplicadoArchivo, TipoDuplicadoRegistro, TipoTipoObraSinMapear:
		return true
	default:
		return false
	}
}

// Modalidades cuyo motor consulta `tipo_obra` (via ponderacionTipo/puntosTV).
const (
	ModalidadTV          = "tv"
	ModalidadSuscripcion = "suscripcion"
	ModalidadHotel       = "hotel"
)

// ModalidadPonderaPorTipoObra dice si la corrida de esa modalidad lee `tipo_obra`; desconocida cuenta como si (D6).
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

// Uso es la proyeccion de una fila de `usos` que miran los detectores.
type Uso struct {
	ID        string
	ReporteID string
	Fuente    string
	Titulo    string

	// Escalon es el de la cascada (identificacion.Escalon*).
	Escalon string

	// Modalidad de `usos.modalidad` como string: este paquete no puede importar reparto (ADR 0003).
	Modalidad string

	// ObraID vacio significa que la fila no tiene obra identificada.
	ObraID string

	// TipoObra es la categoria de `RD 9.1.1`. Vacia = sin mapear.
	TipoObra string

	// ClaveRegistro llega derivada por aplicacion (ADR 0018); vacia = no se compara (D3).
	ClaveRegistro string
}

// Entrega es el acuse de un reporte: lo justo para cotejar su huella.
type Entrega struct {
	ID      string
	Fuente  string
	Periodo string
	SHA256  string
}

// Obra es una obra del periodo con lo necesario para juzgar su declaracion.
type Obra struct {
	ID string

	// CoautoresIPI: IPI de obra_coautores, ordenados. No dan derecho a cobrar (R-02, R-03).
	CoautoresIPI []string

	// Declarada: hay version abierta. Falso no es lo mismo que una version abierta sin partes.
	Declarada bool

	// Declaracion vigente entera: quien decide si esta completa es repertorio.
	Declaracion repertorio.Declaracion
}

// Periodo es lo que los detectores necesitan, armado por aplicacion y con listas ordenadas (D0).
type Periodo struct {
	// Periodo es el que se evalua, en la forma `AAAA` o `AAAA-MM`.
	Periodo string

	// Usos son las filas canonicas de los reportes DE ESTE periodo.
	Usos []Uso

	// Entregas son TODAS las conocidas: la colision puede tener una pata en otro mes (D2).
	Entregas []Entrega

	// Obras son las que el periodo pondera, no el catalogo entero.
	Obras []Obra
}
