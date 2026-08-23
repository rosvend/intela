package aplicacion

import (
	"context"
	"time"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/shopspring/decimal"
)

type Usuario struct {
	ID        string
	Email     string
	Nombre    string
	Rol       string
	TitularID string
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

type UsoPersistido struct {
	ID            string
	ReporteID     string
	Fuente        string
	Titulo        string
	IDsFuente     string
	ObraID        string
	Escalon       string
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
	Circuito  string
	Bruto     decimal.Decimal
}

type ProcesoVista struct {
	reparto.Proceso
	Periodo string
	BolsaID string
}

type Asiento struct {
	ID      string
	Hecho   string
	RefTipo string
	RefID   string
	Payload string
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

type Reloj interface {
	Ahora() time.Time
}

type AlmacenObjetos interface {
	Poner(ctx context.Context, clave string, datos []byte) error
}

type Notificador interface {
	Notificar(ctx context.Context, dest, asunto, cuerpo string) (acuse string, err error)
}

type Similitud interface {
	Candidatos(ctx context.Context, titulo string) ([]identificacion.Candidato, error)
}

type Repositorios interface {
	UsuarioPorEmail(ctx context.Context, email string) (Usuario, string, error)
	UsuarioPorID(ctx context.Context, id string) (Usuario, error)
	GuardarSesion(ctx context.Context, token, usuarioID string) error
	UsuarioPorToken(ctx context.Context, token string) (Usuario, error)

	ListarObras(ctx context.Context) ([]Obra, error)
	Declaraciones(ctx context.Context) (map[string]repertorio.Declaracion, error)
	Alias(ctx context.Context, fuente, tipo, valor string) (string, error)
	GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error
	ObraPorIDGlobal(ctx context.Context, ida, eidr, imdb string) (string, error)

	GuardarReporte(ctx context.Context, id, fuente, periodo, sha, claveObjeto string, nbytes int) error
	GuardarUsos(ctx context.Context, usos []UsoPersistido) error
	UsosSinResolver(ctx context.Context) ([]UsoPersistido, error)
	UsosDePeriodo(ctx context.Context, periodo string) ([]UsoPersistido, error)
	ActualizarUsoMatch(ctx context.Context, usoID, obraID, escalon string, oni bool) error
	ListarONI(ctx context.Context) ([]UsoPersistido, error)

	ListarBolsas(ctx context.Context) ([]BolsaPersistida, error)
	BolsaPorID(ctx context.Context, id string) (BolsaPersistida, error)

	SnapshotVigente(ctx context.Context, cuando time.Time) (reparto.Snapshot, error)
	FilasParametros(ctx context.Context) ([]map[string]string, error)

	GuardarProceso(ctx context.Context, p ProcesoVista) error
	ProcesoPorID(ctx context.Context, id string) (ProcesoVista, error)
	ListarProcesos(ctx context.Context) ([]ProcesoVista, error)
	GuardarFirma(ctx context.Context, procesoID, rol, actorID string, rev int) error

	GuardarResultado(ctx context.Context, procesoID string, r reparto.Resultado) error
	ResultadoDe(ctx context.Context, procesoID string) (reparto.Resultado, error)
	LiquidacionesDeTitular(ctx context.Context, titularID string) (reparto.Resultado, string, error)

	Asentar(ctx context.Context, a Asiento) error
	Asientos(ctx context.Context, refTipo, refID string) ([]Asiento, error)
	AsientoPorID(ctx context.Context, id string) (Asiento, error)

	Encolar(ctx context.Context, tipo, payload string) error
	TomarTrabajo(ctx context.Context) (id int64, tipo, payload string, ok bool, err error)
	CerrarTrabajo(ctx context.Context, id int64, errMsg string) error

	CalendarioPendiente(ctx context.Context, hoy string) ([]string, error)
	MarcarCalendarioDisparado(ctx context.Context, periodo string) error

	Alertas(ctx context.Context) ([]Alerta, error)
	Anticipos(ctx context.Context) ([]Anticipo, error)
	GuardarAnticipo(ctx context.Context, a Anticipo) error
	Reclamaciones(ctx context.Context) ([]map[string]string, error)

	SembrarSiVacio(ctx context.Context, hashAdmin, hashTitular, hashAuditor string) error
}
