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

// ObraDelCatalogo es una entrada del catalogo maestro tal como la sirve el
// catalogo: sus metadatos y, ademas, lo que el sistema sabe hoy de su
// Declaracion de Obra.
//
// # Por que es una proyeccion y no la entidad
//
// [repertorio.Obra] es identidad y metadatos: no tiene donde poner un estado
// que sale de `declaraciones`, y no debe tenerlo -el porcentaje de reparto
// nace en la Declaracion de Obra y en ningun otro sitio (`R-02`, `R-03`)-.
// Los tres campos que van aqui son LEIDOS de la declaracion vigente, no
// declarables desde el catalogo.
//
// # VersionVigente es el ORIGEN del estado, no un tercer estado
//
// `EstadoDecl` solo distingue "completa" de "incompleta", y con eso una obra
// que nunca se declaro y una declarada que no suma 100 son indistinguibles:
// las dos son `incompleta`, y las dos se retienen (`R-04`). Una pantalla que
// pintara "incompleta" sobre una obra sin ninguna declaracion afirmaria una
// declaracion que nadie hizo. `VersionVigente` en nil dice lo que de verdad
// pasa -no hay ninguna version- y por eso el estado no necesita un tercer
// valor: `invalida` no es un estado del modelo sino lo que el backend
// RECHAZA al escribir (una suma por encima de 100), asi que no puede estar
// persistido ni aparecer en un listado.
type ObraDelCatalogo struct {
	// La entidad entera -identidad y metadatos- se embebe para que una obra
	// proyectada siga respondiendo ID(), Metadatos() y Coautores(): el
	// catalogo y su estado son la misma obra, no dos cosas que haya que
	// volver a cruzar por id.
	repertorio.Obra

	// EstadoDecl es "completa" o "incompleta", y lo calcula el dominio
	// ([repertorio.Declaracion.Estado]): no hay un tercer valor. Una suma por
	// debajo de 100 es `incompleta`, un estado VALIDO del negocio -se retiene
	// el total de esa obra, nunca se reparte a medias-, no un error.
	EstadoDecl string

	// SumaPorcentajes es la suma de los porcentajes de las partes de la
	// version vigente; cero si la obra no tiene ninguna.
	//
	// No es "cuanto le toca a nadie" ni un reparto: es cuanto esta declarado.
	// Y no se deduce del estado ni el estado de ella: una parte sin IPI deja
	// la declaracion `incompleta` con la suma en 100, asi que quien muestre
	// las dos cosas tiene que mostrar las dos -ver el comentario de arriba
	// sobre no afirmar mas de lo que el sistema sabe-.
	SumaPorcentajes decimal.Decimal

	// VersionVigente es la version ABIERTA de la declaracion de la obra, y
	// nil quiere decir que la obra no tiene ninguna declaracion -distincion
	// que `EstadoDecl` por si solo no puede hacer: ver el comentario del
	// tipo-.
	VersionVigente *int
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

// EntregaRecibida es el acuse de un reporte con lo JUSTO para cotejar sus
// bytes contra los de otra entrega: quien la entrego, de que periodo y que
// huella trae.
//
// Existe aparte de [CargaReporte] por coste, no por gusto. La deteccion de
// anomalias pide TODAS las entregas conocidas en cada pasada -- el
// UNIQUE (sha256, fuente) de `reportes` no lleva el periodo, asi que la otra
// pata de una colision puede estar en otro mes -- y de cada una solo mira estos
// cuatro campos. `ListarCargas` ademas calcula dos COUNT correlacionados por
// fila (usos y usos_rechazados) que esta lectura DESCARTA: medido sobre 5.001
// reportes y 50.000 usos son 130,8 ms contra 4,8 ms sin ellos, en cada pasada y
// creciendo para siempre.
type EntregaRecibida struct {
	ID      string
	Fuente  string
	Periodo string
	SHA256  string
}

// UsoPersistido es una fila de reporte tal como quedo guardada, con el
// resultado de la identificacion.
//
// Igual que reparto.Uso, no tiene campo de dinero, y por la misma razon.
type UsoPersistido struct {
	ID             string
	ReporteID      string
	Fuente         string
	Titulo         string
	TituloOrig     string
	IDsFuente      string
	ObraID         string
	Escalon        string
	Evidencia      string
	ONI            bool
	Modalidad      reparto.Modalidad
	TipoObra       string
	CanalID        string
	Fecha          string
	Hora           string
	Moneda         string
	UnidadDuracion string
	// DuracionTexto y EmisionesTexto conservan el crudo del adaptador cuando
	// el campo viaja como texto hacia [normalizacion.Fila]. Si estan vacios,
	// aFila re-serializa DuracionMin / Emisiones.
	DuracionTexto  string
	EmisionesTexto string
	DuracionMin    decimal.Decimal
	Emisiones      int64
	Rating         decimal.Decimal
	Taquilla       decimal.Decimal
	Espectadores   decimal.Decimal
	Exhibiciones   int64
	Vistas         decimal.Decimal
	MinutosVistos  decimal.Decimal
	PB             decimal.Decimal
	Autopromo      bool

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

	// RechazoTipo y RechazoCodigo son el discriminante tipado de la cola de
	// revision (B3). Sin columnas propias, cortar el motivo por ": " inventaba
	// codigos a partir de prosa del adaptador y afirmaba origen normalizacion
	// sobre filas que nunca pasaron por ese detector.
	RechazoTipo   string
	RechazoCodigo string
}

// UsoDeReparto es una fila canonica lista para el motor: el uso mas la
// clasificacion anual del canal que lo emitio (`RD 9.5.4`).
//
// El grupo no es columna de `usos` porque no es un hecho del reporte: se
// resuelve por ano contra `canales_clasificacion`, y una reejecucion de un
// periodo pasado tiene que leer la fila de aquel ano (ADR 0005). Llega vacio
// cuando el catalogo no clasifico el canal, que solo es legal fuera de
// suscripcion.
type UsoDeReparto struct {
	Uso           UsoPersistido
	GrupoEfectivo string
}

// ResumenUsosDeCanal cuenta, dentro de un (periodo, canal), los usos que NO
// llegan al motor porque no tienen obra identificada (`usos.obra_id IS NULL`).
// Ningun tratamiento se puede confundir con otro, porque el reglamento los
// trata distinto:
//
//   - Pendientes: la cascada de identificacion (ADR 0007) todavia no corrio
//     sobre la fila. No es lo mismo que "no se reconocio nada" -- es "no se
//     ha intentado".
//   - ONI: la cascada corrio y no reconocio ninguna obra.
//   - Excluidos: el canal esta fuera del catalogo de REDES SGC (R-27), asi
//     que la fila nunca tuvo obra que identificar.
//
// # Esto SOLO cuenta. No reserva nada
//
// `RD 13.8` / R-18 / R-19 mandan que la parte de una obra no identificada
// quede en reserva, no que se pierda ni que se reparta entre las demas obras.
// Este tipo no implementa eso: es un conteo, para que el hueco sea visible.
// Quien tome los `[]reparto.Uso] que devuelve UsosDeCanal y los pase
// directamente a [reparto.Reparto] -- que es lo unico que existe hoy, porque
// ProcesoDeReparto (#33/#34) todavia no orquesta una corrida -- reparte el
// 100% de la bolsa entre las obras IDENTIFICADAS: la parte que le habria
// correspondido a una fila ONI desaparece DENTRO de esas obras, no en
// reserva. Reservarla de verdad -- y decidir como se libera cuando la obra se
// identifica (R-19: 3 anos) -- es trabajo de #33/#34, registrado con
// implementacion pendiente bajo R-18 en docs/dominio/reglas-negocio.md.
type ResumenUsosDeCanal struct {
	Pendientes int
	ONI        int
	Excluidos  int
}

// ItemRevision es una fila de la cola de revision: lo que no se pudo
// normalizar, y los rechazos del adaptador de formato.
//
// Tipo discrimina el origen ("normalizacion" | "adaptador" | "anomalia") para
// que un solo listado sirva sin mezclar los vocabularios. Codigo es el motivo
// tipado; Motivo es el texto que nombra el campo.
//
// # Las anomalias del #37 NO salen por aqui
//
// El comentario anterior decia que llegarian, y no llegaron: una anomalia
// necesita estado de resolucion -- actor e instante -- y este tipo no lo
// tiene; y `/admin/cola-revision` es solo de `administrador` mientras el
// tablero de anomalias lo miran ademas `distribucion` y `auditor`. Viven en
// [Alerta], con tabla y recurso propios (ADR 0020). El valor "anomalia" se
// queda en el vocabulario porque la columna `usos_rechazados.tipo` lo admite
// desde la migracion 00010.
type ItemRevision struct {
	ID        string
	Tipo      string
	Codigo    string
	Motivo    string
	Fuente    string
	Titulo    string
	ReporteID string
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

// Alerta es una anomalia de un periodo tal como queda persistida (#37).
//
// Es la forma de ESTADO del [anomalias.Hallazgo], que es la forma de HECHO: el
// hallazgo lo produce el dominio sin identidad ni instante, y esto es lo que
// la fila anade -- quien es, cuando se detecto y si alguien ya la resolvio.
//
// # No es un asiento, y por eso si se actualiza
//
// La bitacora del ADR 0006 es append-only y `asientos` lo impone por trigger;
// esta tabla no. La diferencia no es de rigor: un asiento registra que algo
// PASO, y una alerta registra que algo SIGUE pasando. Resolver una alerta es
// cambiar ese estado, y el rastro de quien lo cambio y cuando va donde tiene
// que ir -- a un asiento, escrito en la misma unidad de trabajo que el UPDATE
// (ver [Anomalias.Resolver]).
type Alerta struct {
	ID string

	// Periodo al que pertenece la evaluacion que la levanto, en la forma
	// `AAAA` o `AAAA-MM`. Va en la fila y no se deduce del registro ofensor
	// porque una misma obra puede quedar retenida en dos periodos seguidos, y
	// cada periodo tiene su propia alerta que perseguir.
	Periodo string

	// Tipo es uno de los seis de [anomalias.Tipos].
	Tipo string

	// RefTipo, RefID y RefTitular apuntan al registro EXACTO que la disparo.
	// Mismo par polimorfico que `asientos` (migracion 00001); RefTitular es la
	// segunda coordenada y solo la usa `titular_sin_porcentaje`.
	RefTipo    string
	RefID      string
	RefTitular string

	Detalle string

	// Critica es DERIVADO ([anomalias.EsCritica]) y no una columna.
	//
	// Persistirlo congelaria la clasificacion en el momento de detectar: el
	// dia que el Consejo decida que un tipo pasa a bloquear, habria que migrar
	// las filas ya escritas o convivir con dos criterios. Derivarlo al leer
	// hace que la regla viva en un solo sitio, que es el dominio.
	Critica bool

	Detectada time.Time

	// Resuelta, ResueltaPor, ResueltaEn y Nota son la decision humana (#39).
	// ResueltaEn es puntero: nil es "sin resolver", y un cero de time.Time
	// seria un instante del ano 1 indistinguible de un dato mal escrito.
	Resuelta    bool
	ResueltaPor string
	ResueltaEn  *time.Time
	Nota        string
}

// FiltroAlertas recorta el listado de alertas. Un campo en su valor cero NO
// filtra, y los que vienen se combinan con Y, igual que [FiltroObras].
//
// Resueltas es PUNTERO por lo mismo que `FiltroTitulares.PersonaNatural`: con
// un bool a secas, "no filtrar" y "solo las resueltas" serian el mismo valor
// cero y no habria forma de pedir el historial de lo ya cerrado.
type FiltroAlertas struct {
	Periodo   string
	Tipo      string
	Resueltas *bool

	// Paginacion, con la MISMA forma que `/obras`, `/titulares` y el historial
	// de declaraciones, y por la misma razon de la nota 4 de #86: los listados
	// de este sistema paginan juntos, no cada uno a su aire.
	//
	// Aqui hace falta ademas por volumen. `oni`, `duplicado_registro` y
	// `tipo_obra_sin_mapear` emiten UNA alerta por fila de uso, y la meta KR-1
	// habla de lotes de 10.000 registros: una evaluacion real puede dejar del
	// orden de 10.000 alertas en un periodo. Sin paginar, `GET /alertas`
	// devolvia ese array entero -- del orden de 4 MB -- y el panel de #104
	// sondea cada 15 segundos.
	Paginacion
}

type Anticipo struct {
	ID        string
	TitularID string
	Monto     decimal.Decimal
	Estado    string
}
