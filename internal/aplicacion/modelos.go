package aplicacion

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Rol de un actor. La autorizacion de cada caso de uso se decide contra esto,
// no contra la mera existencia de una sesion.
type Rol string

const (
	RolAdministrador Rol = "administrador"
	RolDistribucion  Rol = "distribucion"
	RolContabilidad  Rol = "contabilidad"
	RolAuditor       Rol = "auditor"
	RolTitular       Rol = "titular"
)

// Usuario autenticado. No lleva el hash de la contrasena: eso no sale del
// adaptador que lo verifica.
type Usuario struct {
	ID        string
	Email     string
	Nombre    string
	Rol       Rol
	TitularID string
}

// Sesion es lo que devuelve un login correcto.
//
// Token es el valor EN CLARO, y es la unica vez que existe: lo que se
// persiste es un resumen suyo, y de ahi no se puede volver. Si quien llama lo
// pierde, no hay forma de recuperarlo -hay que iniciar sesion otra vez-, que
// es justo la propiedad que se busca.
//
// Sin etiquetas json, como el resto de los modelos del nucleo: la forma que
// viaja por la red la decide el adaptador HTTP, no esto.
type Sesion struct {
	Token   string
	Expira  time.Time
	Usuario Usuario
}

type Obra struct {
	ID         string
	Titulo     string
	IDA        string
	EIDR       string
	IMDB       string
	Tipo       string
	EstadoDecl string
}

// UsoPersistido es una fila de reporte tal como quedo guardada, con el
// resultado de la identificacion.
//
// Igual que reparto.Uso, no tiene campo de dinero, y por la misma razon.
type UsoPersistido struct {
	ID            string
	ReporteID     string
	Fuente        string
	Titulo        string
	IDsFuente     string
	ObraID        string
	Escalon       string
	Evidencia     string
	ONI           bool
	Modalidad     reparto.Modalidad
	TipoObra      string
	DuracionMin   decimal.Decimal
	Emisiones     int64
	Rating        decimal.Decimal
	Taquilla      decimal.Decimal
	Vistas        decimal.Decimal
	MinutosVistos decimal.Decimal
	PB            decimal.Decimal
}

type BolsaPersistida struct {
	ID        string
	UsuarioID string
	Periodo   string
	Circuito  reparto.Circuito
	Bruto     decimal.Decimal
}

// Asiento de la bitacora. Append-only (ADR 0006): no se actualiza, no se
// borra, y un mismo hecho ocurrido dos veces deja dos asientos.
//
// El ID lo asigna el adaptador y es opaco. Derivarlo del hecho -por ejemplo
// "as-calc-"+procesoID- convierte un INSERT idempotente en perdida silenciosa
// de asientos: recalcular no dejaria rastro del segundo calculo.
type Asiento struct {
	ID      string
	Hecho   string
	RefTipo string
	RefID   string
	ActorID string
	Payload []byte
	Cuando  time.Time
}

type Alerta struct {
	ID      string
	Tipo    string
	Detalle string
}

type Anticipo struct {
	ID        string
	TitularID string
	Monto     decimal.Decimal
	Estado    string
}

// Formatos de exportacion que entiende [Exportador]. Cualquier otro es
// ErrFormatoInvalido.
const (
	FormatoPDF  = "pdf"
	FormatoXLSX = "xlsx"
)

// FilaLiquidacion es lo que el repositorio lee de una corrida: el neto del
// titular en una obra y los totales del proceso, todavia sin prorratear.
//
// El prorrateo vive en dominio/liquidacion, no aqui ni en el SQL: si una
// cifra se puede calcular mal, se calcula una sola vez (postgres/doc.go).
type FilaLiquidacion struct {
	Periodo        string
	ObraID         string
	Titulo         string
	Neto           decimal.Decimal
	ProcesoBruto   decimal.Decimal
	ProcesoAdmin   decimal.Decimal
	ProcesoSocial  decimal.Decimal
	ProcesoReserva decimal.Decimal
	ProcesoNeto    decimal.Decimal
}

// LineaLiquidacion es una obra en el reporte del titular, con bruto,
// cada deduccion y neto ya prorrateados.
type LineaLiquidacion struct {
	Periodo string
	ObraID  string
	Titulo  string
	Bruto   decimal.Decimal
	Admin   decimal.Decimal
	Social  decimal.Decimal
	Reserva decimal.Decimal
	Neto    decimal.Decimal
}

// TotalesLiquidacion suma las lineas. Es lo que el panel y el archivo
// exportado tienen que coincidir.
type TotalesLiquidacion struct {
	Bruto   decimal.Decimal
	Admin   decimal.Decimal
	Social  decimal.Decimal
	Reserva decimal.Decimal
	Neto    decimal.Decimal
}

// Liquidacion es el reporte de un titular, filtrado por periodo si viene.
type Liquidacion struct {
	TitularID string
	Periodo   string
	Lineas    []LineaLiquidacion
	Totales   TotalesLiquidacion
}

// Archivo es un PDF o un Excel ya renderizado. Lleva las cifras embebidas:
// abrirlo sin red muestra la liquidacion completa (OE-6).
type Archivo struct {
	Nombre    string
	TipoMIME  string
	Contenido []byte
}
