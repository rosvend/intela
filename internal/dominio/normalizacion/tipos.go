package normalizacion

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Codigos de revision. Son el vocabulario compartido con la cola de revision
// (y, mas adelante, con las anomalias del #37): un listado filtra por codigo,
// no por texto libre.
const (
	CodigoFechaInparseable  = "fecha_inparseable"
	CodigoMonedaDesconocida = "moneda_desconocida"
	CodigoParametroAusente  = "parametro_ausente"
	CodigoMedidaInvalida    = "medida_invalida"
	CodigoModalidadInvalida = "modalidad_invalida"
)

// Modalidades del acto de comunicacion publica (RD 8). Duplicadas aqui y no
// importadas de reparto a proposito: este paquete no toca dinero (ADR 0003)
// y depguard le deniega el motor. Son las mismas cuatro cadenas.
const (
	ModalidadTV    = "tv"
	ModalidadCine  = "cine"
	ModalidadOTT   = "ott"
	ModalidadHotel = "hotel"
)

const (
	unidadMinutos = "minutos"
	unidadHoras   = "horas"
)

// Fila es una fila YA mapeada por el adaptador de formato, todavia en los
// tipos y formatos de la fuente.
//
// Todo lo que parece una fecha, una hora, una duracion o una moneda viaja
// como texto: el adaptador no decide como interpretarlo. Interpretarlo es
// justo lo que hace este paquete, y para eso tiene que ver el valor crudo.
//
// No tiene campo de importe. Un reporte de uso pondera la bolsa, no la
// aporta; [Taquilla] es la metrica de cine (RD 9.2), no dinero a sumar.
type Fila struct {
	ID             string
	Fuente         string
	Modalidad      string
	Titulo         string
	TituloOrig     string
	IDsFuente      string
	TipoObra       string
	Fecha          string
	Hora           string
	Duracion       string
	UnidadDuracion string
	Emisiones      string
	Rating         string
	Taquilla       string
	Moneda         string
	Vistas         string
	MinutosVistos  string
	PB             string
	Autopromo      bool
}

// Parametros son los coeficientes normativos que este paquete necesita,
// resueltos contra la vigencia del periodo (ADR 0004).
//
// Ninguno tiene valor por defecto aqui: un cero o una lista vacia es
// "ausente", y una fila de TV que los necesite va a revision en vez de
// calcularse con un 80% o un 48 inventados por el codigo.
type Parametros struct {
	DuracionArtisticaPct decimal.Decimal
	MinutosHoraTV        decimal.Decimal
	MonedaBase           string
	MonedasReconocidas   []string
	TRM                  decimal.Decimal
}

// Fecha civil. Existe para no importar `time`: el nucleo recibe instantes
// como parametro (ADR 0002), y una fecha de parrilla no es un instante, es
// un dia del calendario.
type Fecha struct {
	Anio int
	Mes  int
	Dia  int
}

// String es la forma canonica: YYYY-MM-DD. La aceptacion de OE-1 pide un
// formato unico con independencia de como viniera la fuente.
func (f Fecha) String() string {
	if f.EsCero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", f.Anio, f.Mes, f.Dia)
}

// EsCero dice si la fecha no se relleno. Una fila OTT puede no traer fecha
// por registro (term_end_date es metadato del export); eso no es un fallo.
func (f Fecha) EsCero() bool {
	return f.Anio == 0 && f.Mes == 0 && f.Dia == 0
}

// Uso es la forma canonica. TV, cine y OTT salen por el mismo tipo: las
// metricas que una modalidad no usa quedan en cero, y no hay un struct por
// fuente. Aguas abajo el motor consume este esquema, no el de Caracol ni el
// de Netflix.
//
// No tiene campo de dinero. Taquilla, despues de normalizar, esta en la
// moneda base; sigue siendo una metrica de ponderacion, no un importe.
type Uso struct {
	ID            string
	Fuente        string
	Modalidad     string
	Titulo        string
	TituloOrig    string
	IDsFuente     string
	TipoObra      string
	Fecha         Fecha
	Hora          string
	DuracionMin   decimal.Decimal
	Emisiones     int64
	Rating        decimal.Decimal
	Taquilla      decimal.Decimal
	Vistas        decimal.Decimal
	MinutosVistos decimal.Decimal
	PB            decimal.Decimal
	Autopromo     bool
}

// Revision es el motivo tipado por el que una fila no es canonica.
//
// Campo se nombra siempre: el log de rechazos existe para pedirle al cliente
// exactamente lo que falta, y un "fila invalida" no sirve.
type Revision struct {
	Codigo  string
	Campo   string
	Detalle string
}

// Motivo es el texto que viaja al log de rechazos. Empieza por el codigo
// para que la cola de revision pueda filtrar sin parsear prosa.
func (r Revision) Motivo() string {
	if r.Detalle == "" {
		return r.Codigo + ": " + r.Campo
	}
	return r.Codigo + ": " + r.Detalle
}
