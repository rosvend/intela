package aplicacion

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
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

// VersionDeclaracion es una Declaracion de Obra con su ventana de vigencia.
//
// La vigencia vive aqui y no en [repertorio.Declaracion] porque depguard
// deniega el paquete `time` entero dentro de internal/dominio (regla
// dominio-sin-reloj-ni-azar de .golangci.yml), sin excepcion para "solo el
// tipo del parametro". Es el mismo motivo por el que [FilaParametro] -que
// tiene el mismo problema, un valor con vigencia- vive en esta capa y no en el
// dominio. [repertorio.Declaracion] se queda pura: solo las partes y el
// calculo de si suman 100.
//
// VigenteHasta en nil quiere decir que esta es la version abierta, la vigente
// ahora mismo -la unica que puede tener otro EditarSplits por encima-.
type VersionDeclaracion struct {
	Version      int
	VigenteDesde time.Time
	VigenteHasta *time.Time
	Declaracion  repertorio.Declaracion
}

// Reporte es el acuse de una entrega recibida.
//
// Lleva lo justo para volver a la evidencia exacta que pondero una corrida:
// de que fuente vino, de que periodo, que bytes fueron -SHA256- y donde estan
// -ClaveObjeto-. Es la pregunta 2 del ADR 0006, "la version exacta del
// archivo", y no "el archivo de Caracol".
//
// No tiene campo de dinero por la misma razon que UsoPersistido: un reporte de
// uso no aporta importes.
type Reporte struct {
	ID          string
	Fuente      string
	Periodo     string
	SHA256      string
	ClaveObjeto string
	NBytes      int
}

// Recepcion es el acuse de una entrega INGERIDA entera: congelada, parseada y
// persistida.
//
// Los rechazos viajan enteros y no como recuento. Quien sube un archivo tiene
// que poder leer, sin abrir la base, que linea le falta y por que: es criterio
// de aceptacion de OE-1, y un numero a secas obliga a otra consulta para saber
// que pedirle al cliente.
//
// Aceptados es un recuento y no una lista a proposito: las filas buenas ya
// estan en `usos` y quien las quiera las lee de ahi. Devolverlas aqui seria
// materializar una parrilla entera en memoria para que casi siempre nadie la
// mire.
type Recepcion struct {
	Reporte    Reporte
	Aceptados  int
	Rechazados []UsoPersistido
}

// CargaReporte es una entrega tal como aparece en el listado de cargas hechas.
//
// Es una PROYECCION de lectura, no el [Reporte] con campos de mas: los dos
// recuentos salen de contar `usos` y `usos_rechazados`, o sea de otras dos
// tablas, y meterlos en Reporte obligaria a que toda escritura de una entrega
// -- que ocurre ANTES de que exista ninguna fila -- cargara dos ceros sin
// significado.
//
// Recibido es el `creado` de la fila. Sale del reloj de la base y no del
// [Reloj] del nucleo porque es la marca de un hecho de almacenamiento, no de
// negocio: nada del reparto se calcula con el.
type CargaReporte struct {
	Reporte
	Recibido   time.Time
	Aceptados  int
	Rechazados int
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

	// RechazoMotivo: por que esta fila no se pudo normalizar.
	//
	// Vacio en una fila canonica. Con contenido, la fila NO es un uso: es una
	// entrada del log de rechazos, y ni pondera ni aparece en las lecturas
	// canonicas. Guardarla con su motivo en vez de descartarla es criterio de
	// aceptacion de OE-1 y de KR-1, y es lo que permite volver a pedirle al
	// cliente exactamente lo que falta.
	//
	// Es la misma forma que reparto.LineaObra.Retenida/Motivo y que
	// ProcesoVista.RechazoMotivo: en este sistema, lo que se aparta se aparta
	// CON su razon. Donde acaba cada una de las dos clases de fila lo decide
	// el adaptador (ADR 0016).
	RechazoMotivo string
}

// BolsaPersistida es una [recaudo.Bolsa] con lo que la fila anade: su
// identificador y su procedencia.
//
// El ID vive aqui y no en el dominio por lo mismo que la vigencia de
// [VersionDeclaracion]: al motor de reparto la bolsa le llega como dato de
// entrada de una funcion pura y no necesita saber en que fila estaba.
//
// Convenio, Tarifa y Factura son PROCEDENCIA, no insumos de calculo. Bajo P-08
// Intela recibe el importe ya cobrado y no liquida tarifas, asi que estos tres
// campos no se usan para computar nada: responden la pregunta 1 del ADR 0006
// -de donde salio este dinero- y son lo que un auditor sigue hasta la cuenta
// de cobro. Los tres son opcionales: `T-11` dice que la tarifa publicada es el
// valor por defecto CUANDO NO HAY convenio, asi que exigir un convenio seria
// afirmar algo que el reglamento no afirma.
type BolsaPersistida struct {
	ID        string
	UsuarioID string
	Periodo   string
	Circuito  recaudo.Circuito
	Bruto     decimal.Decimal

	Convenio string
	Tarifa   string
	Factura  string
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

// SolicitudAfiliacion es lo que el asistente de alta manda al caso de uso.
//
// Los documentos van en bytes, no en claves: quien llama no conoce el
// almacen. El caso de uso los guarda y deja las claves en el Afiliado.
type SolicitudAfiliacion struct {
	Nombre             string
	Email              string
	DocumentoIdentidad string
	IPI                string
	Subtipo            string
	PerteneceOtraSGC   bool
	Clave              string
	RUT                []byte
	CertBancaria       []byte
	Renuncia           []byte
}

// AfiliacionVista es la solicitud (o el afiliado ya admitido) tal como
// sale del nucleo hacia el adaptador. Sin etiquetas json: la forma de
// red la decide HTTP.
type AfiliacionVista struct {
	ID                 string
	Nombre             string
	Email              string
	DocumentoIdentidad string
	IPI                string
	Subtipo            string
	Estado             string
	ElegibleAnticipo   bool
	TieneRUT           bool
	TieneCertBancaria  bool
	TieneRenuncia      bool
	TitularID          string
}
