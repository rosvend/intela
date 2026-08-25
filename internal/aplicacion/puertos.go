package aplicacion

import (
	"context"
	"time"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// ---------------------------------------------------------------------------
// Puertos de servicio
// ---------------------------------------------------------------------------

// Reloj es la unica forma de saber que hora es dentro del nucleo.
//
// El ADR 0005 exige que una corrida se reproduzca bit a bit anos despues, y
// el ADR 0002 que las prescripciones de 3 y 10 anos y las ventanas de 15 dias
// se puedan probar sin esperar una decada. Un time.Now() suelto rompe las dos
// cosas en silencio: no falla, devuelve otro numero.
//
// Vive aqui y no en internal/dominio porque depguard deniega el paquete time
// dentro del dominio (regla dominio-sin-reloj-ni-azar), y declarar
// Ahora() time.Time exige importarlo. El dominio recibe el instante como
// parametro; no lo obtiene.
type Reloj interface {
	Ahora() time.Time
}

// AlmacenObjetos guarda los reportes crudos tal como llegaron.
//
// El ADR 0006 pide copia inmutable con retencion: los reportes crudos son la
// evidencia de la que cuelga todo lo demas. Un almacen que permita sobrescribir
// una clave ya escrita no satisface este puerto.
type AlmacenObjetos interface {
	Poner(ctx context.Context, clave string, datos []byte) error
	Obtener(ctx context.Context, clave string) ([]byte, error)
}

type Notificador interface {
	Notificar(ctx context.Context, dest, asunto, cuerpo string) (acuse string, err error)
}

// Similitud propone obras candidatas para un titulo. El umbral no se aplica
// aqui: lo aplica la cascada, con el valor que venga del snapshot normativo.
type Similitud interface {
	Candidatos(ctx context.Context, titulo string) ([]identificacion.Candidato, error)
}

// Hasher verifica y genera hashes de contrasena.
//
// Esta como puerto porque bcrypt es un detalle de adaptador: el nucleo decide
// que hay que verificar una credencial, no con que algoritmo.
type Hasher interface {
	Verificar(hash, clave string) bool
	Hash(clave string) (string, error)
}

// ---------------------------------------------------------------------------
// Puertos de persistencia, uno por modulo
// ---------------------------------------------------------------------------

// RepositorioAfiliacion cubre el padron y las sesiones.
//
// UsuarioPorEmail devuelve el hash aparte para que sea el Hasher quien lo
// verifique: el resto del nucleo nunca ve una credencial.
type RepositorioAfiliacion interface {
	UsuarioPorEmail(ctx context.Context, email string) (u Usuario, hash string, err error)
	UsuarioPorID(ctx context.Context, id string) (Usuario, error)
}

// Sesiones tiene TTL por contrato: una sesion sin expiracion es una
// credencial permanente que nadie puede revocar.
type Sesiones interface {
	Crear(ctx context.Context, token, usuarioID string, expira time.Time) error
	PorToken(ctx context.Context, token string, ahora time.Time) (Usuario, error)
	Revocar(ctx context.Context, token string) error
}

// RepositorioRepertorio cubre el catalogo maestro y las declaraciones.
type RepositorioRepertorio interface {
	ListarObras(ctx context.Context) ([]Obra, error)
	ObraPorID(ctx context.Context, id string) (Obra, error)
	Declaraciones(ctx context.Context) (map[string]repertorio.Declaracion, error)
	DeclaracionDeObra(ctx context.Context, obraID string) (repertorio.Declaracion, error)
}

// RepositorioIdentificacion cubre alias, identificadores globales y el
// resultado del matching.
//
// ObraPorIDGlobal recibe los tres identificadores y devuelve ErrNoEncontrado
// si los tres llegan vacios: llamarla sin datos no puede pasar por "no hay
// match".
type RepositorioIdentificacion interface {
	Alias(ctx context.Context, fuente, tipo, valor string) (obraID string, err error)
	GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error
	ObraPorIDGlobal(ctx context.Context, ida, eidr, imdb string) (obraID string, err error)
	GuardarMatch(ctx context.Context, usoID string, r identificacion.Resultado) error
}

// RepositorioIngesta cubre los reportes recibidos y sus filas.
type RepositorioIngesta interface {
	GuardarReporte(ctx context.Context, id, fuente, periodo, sha, claveObjeto string, nbytes int) error
	GuardarUsos(ctx context.Context, usos []UsoPersistido) error
	UsosSinResolver(ctx context.Context) ([]UsoPersistido, error)
	UsosDePeriodo(ctx context.Context, periodo string) ([]UsoPersistido, error)
	UsoPorID(ctx context.Context, id string) (UsoPersistido, error)
}

// RepositorioONI es la cola manual. Separado de identificacion porque son dos
// modulos distintos del ADR 0003.
type RepositorioONI interface {
	Listar(ctx context.Context) ([]UsoPersistido, error)
}

// RepositorioRecaudo expone las bolsas. Recaudo es el unico modulo que conoce
// Usuario, Convenio y Tarifa; aguas abajo solo circula la bolsa (ADR 0003).
type RepositorioRecaudo interface {
	ListarBolsas(ctx context.Context) ([]BolsaPersistida, error)
	BolsaPorID(ctx context.Context, id string) (BolsaPersistida, error)
}

// ParametrosNormativos resuelve los parametros con vigencia y organo
// aprobador que exige el ADR 0004.
//
// SnapshotEnFecha se resuelve UNA VEZ, al abrir el proceso, contra la fecha
// del periodo -no contra el ahora del reloj- y queda congelado. Recalcular una
// corrida lee el snapshot del proceso con SnapshotPorID, nunca vuelve a
// resolver: si volviera, cambiar un parametro cambiaria en silencio el
// resultado de una corrida ya hecha.
type ParametrosNormativos interface {
	SnapshotEnFecha(ctx context.Context, fechaPeriodo time.Time) (id string, s reparto.Snapshot, err error)
	SnapshotPorID(ctx context.Context, id string) (reparto.Snapshot, error)
	Vigentes(ctx context.Context, ahora time.Time) ([]FilaParametro, error)
}

// FilaParametro es un parametro normativo con su procedencia. Sin vigencia y
// organo aprobador no es un parametro, es una constante disfrazada.
type FilaParametro struct {
	Clave           string
	Valor           string
	VigenteDesde    time.Time
	VigenteHasta    *time.Time
	OrganoAprobador string
	Reglamento      string
}

// RepositorioProcesos cubre el flujo de aprobaciones del RD 13.5.
type RepositorioProcesos interface {
	Guardar(ctx context.Context, p ProcesoVista) error
	PorID(ctx context.Context, id string) (ProcesoVista, error)
	Listar(ctx context.Context) ([]ProcesoVista, error)
	GuardarFirma(ctx context.Context, procesoID string, f reparto.Firma) error
}

// ProcesoVista es el proceso tal como se persiste.
//
// No embebe un agregado de dominio: el ADR 0008 pide dos maquinas de estado
// distintas, una por circuito, y hasta que ese PR fije los tipos esta vista
// guarda los campos planos.
type ProcesoVista struct {
	ID            string
	Circuito      reparto.Circuito
	Etapa         reparto.Etapa
	Periodo       string
	BolsaID       string
	SnapshotID    string
	Revision      int
	Firmas        []reparto.Firma
	RechazoMotivo string
}

// RepositorioResultados guarda y lee las corridas.
//
// Guardar es transaccional por contrato: un resultado a medias es una cifra
// que alguien puede leer y pagar.
type RepositorioResultados interface {
	Guardar(ctx context.Context, procesoID string, r reparto.Resultado) error
	PorProceso(ctx context.Context, procesoID string) (reparto.Resultado, error)
}

// RepositorioLiquidacion sirve lo que le corresponde a un titular.
type RepositorioLiquidacion interface {
	DeTitular(ctx context.Context, titularID string) ([]reparto.LineaTitular, error)
}

// BitacoraAuditoria es el libro append-only del ADR 0006.
//
// No hay Actualizar ni Borrar, y no los va a haber. Asentar devuelve error y
// ese error NO se descarta: el ADR declara el asiento "parte de la definicion
// de hecho de cada caso de uso", asi que un caso de uso cuyo asiento fallo no
// esta hecho.
//
// La regla "ningun modulo escribe en la trazabilidad de otro" (ADR 0003) se
// sostiene porque este puerto se inyecta por separado, no porque estuviera
// suelto en un contrato que todos comparten.
type BitacoraAuditoria interface {
	Asentar(ctx context.Context, a Asiento) error
	De(ctx context.Context, refTipo, refID string) ([]Asiento, error)
	PorID(ctx context.Context, id string) (Asiento, error)
}

// ColaTrabajos desacopla la ingesta del matching y del reparto por lotes.
type ColaTrabajos interface {
	Encolar(ctx context.Context, tipo string, payload []byte) error
	Tomar(ctx context.Context) (Trabajo, error)
	Cerrar(ctx context.Context, id int64, errMsg string) error
}

// Trabajo es una unidad de trabajo tomada de la cola. Tomar devuelve
// ErrSinTrabajo cuando no hay ninguno: no hacen falta un ok y un error a la
// vez para decir lo mismo.
type Trabajo struct {
	ID      int64
	Tipo    string
	Payload []byte
}

// Calendario dispara las corridas segun RD 10 y RD 12.
type Calendario interface {
	Pendientes(ctx context.Context, hoy time.Time) ([]string, error)
	MarcarDisparado(ctx context.Context, periodo string) error
}

type RepositorioAlertas interface {
	Listar(ctx context.Context) ([]Alerta, error)
}

type RepositorioAnticipos interface {
	Listar(ctx context.Context) ([]Anticipo, error)
	Guardar(ctx context.Context, a Anticipo) error
}
