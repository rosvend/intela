package aplicacion

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/oni"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// HechoListadoONIPublicado es el hecho que deja PublicarListadoONI en la
// bitacora. El modulo ONI publica sus propios asientos (ADR 0003, ADR 0006).
const HechoListadoONIPublicado = "oni.listado_publicado"

// RefTipoPublicacionONI es la referencia de ese asiento. El id es el de la
// fila en oni_publicaciones, no un id derivado del periodo: publicar dos
// veces el mismo periodo no debe ser idempotente en silencio.
const RefTipoPublicacionONI = "oni_publicacion"

// ExplicacionListadoONI es el texto de RD 13.8.4.4: una explicacion clara
// del proceso de Distribucion en el que se incluyen los derechos de las ONI.
// Vive aqui, no en el frontend, para que el contrato HTTP y la pagina
// publica no divergjan.
const ExplicacionListadoONI = "REDES SGC publica este listado de obras no identificadas (ONI) para que los titulares documenten su autoria y soliciten la remuneracion en el siguiente proceso de Distribucion (RD 13.8). Se publican titulos e informacion identificatoria, sin montos: la informacion economica se mantiene en reserva (RD 13.8.2-13.8.3). La prescripcion de estos recaudos es de tres anos contados desde esta publicacion (RD 13.8.7, R-19)."

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

// PublicacionONI es la instantanea de un listado publicado.
//
// FechaProceso es el ancla de R-19. Obras es la proyeccion publica: titulos
// e identificadores, nunca importes.
type PublicacionONI struct {
	ID                   string
	Periodo              string
	FechaProceso         time.Time
	DireccionFisica      string
	DireccionElectronica string
	Obras                []oni.ProyeccionPublica
}
