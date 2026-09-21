package aplicacion

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
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

// Similitud propone obras candidatas para un titulo, puntuadas 0-1 y de mas a
// menos parecida.
//
// El UMBRAL no se aplica aqui: lo aplica la cascada. `piso` es solo hasta donde
// merece la pena buscar, y llega por argumento porque es el mismo parametro
// que resuelve la cascada contra la fecha del periodo. Ver D1 y D3 de
// docs/planes/32-difuso/diseno.md.
type Similitud interface {
	Candidatos(ctx context.Context, titulo string, piso decimal.Decimal) ([]identificacion.Candidato, error)
}

// Hasher verifica y genera hashes de contrasena.
//
// Esta como puerto porque bcrypt es un detalle de adaptador: el nucleo decide
// que hay que verificar una credencial, no con que algoritmo.
type Hasher interface {
	Verificar(hash, clave string) bool
	Hash(clave string) (string, error)

	// EsHash dice si una cadena tiene la forma de un hash de ESTE hasher.
	//
	// Existe porque el nucleo tiene una regla que cumplir -- lo que se guarda
	// en `usuarios.password_hash` tiene que ser verificable -- y no puede
	// comprobarla por si mismo sin aprenderse el algoritmo, que es justo lo
	// que este puerto oculta. Asi que pregunta.
	//
	// No es cosmetico: una clave EN CLARO de 20 caracteres o mas pasaba el
	// unico control que habia (la longitud) y el CHECK del esquema, se
	// guardaba tal cual, y a partir de ahi el login fallaba con la clave
	// correcta y con cualquier otra. Sin ninguna via para arreglarlo, porque
	// esta operacion se niega a correr dos veces.
	EsHash(posible string) bool
}

// GeneradorTokens produce el identificador opaco de una sesion.
//
// Es un puerto por la misma razon que Reloj: el nucleo decide que hace falta
// un token, no de donde sale la aleatoriedad. Con crypto/rand suelto dentro
// del caso de uso, IniciarSesion no se puede probar -cada corrida daria un
// token distinto- y la fuente de entropia, que es una decision criptografica,
// quedaria fijada en el nucleo. Es la misma linea que separa bcrypt detras de
// Hasher.
//
// Devuelve error aunque el adaptador real no vaya a producirlo -crypto/rand
// llena el buffer o mata el proceso-: es lo que permite que un doble simule el
// fallo y se compruebe que no se entrega una sesion sin token.
type GeneradorTokens interface {
	Generar() (string, error)
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

// PadronTitulares es la lectura del padron: quien puede figurar como titular
// de una Declaracion de Obra.
//
// Es la PRIMERA lectura de `titulares` en produccion. Hasta aqui su unico
// consumidor era el trigger `exigir_persona_natural`, que la consulta al
// pagar, y lo unico que la escribia era el seed: el padron no tenia ni tipo en
// el nucleo, ni puerto, ni adaptador, y los tres entran con el editor de
// reparto de la #30.
//
// # Devuelve el padron entero, personas juridicas incluidas
//
// Y no solo los elegibles para cobrar (`R-01`). Filtrar aqui la haria
// invisible: quien edita un reparto tiene que poder ver que el titular del
// padron que el editor no le ofrece como parte existe, y que no se lo ofrece
// por una regla -`R-01`, `RD 4.5`- y no porque falte el dato. Un padron
// recortado en silencio convierte la regla en una rareza inexplicable.
//
// # Por que el metodo no se llama Buscar
//
// Porque el mismo *Store ya tiene un `Buscar` -el del catalogo de obras, con
// otra firma-, y dos metodos con el mismo nombre no caben en un tipo. Es la
// misma razon por la que [BitacoraAuditoria.AsientoPorID] no se llama PorID.
type PadronTitulares interface {
	BuscarTitulares(ctx context.Context, f FiltroTitulares) ([]afiliacion.Titular, error)
}

// RepositorioProvisionInicial crea la primera cuenta de una instalacion vacia.
//
// Puerto aparte y no un metodo mas de RepositorioAfiliacion: eso es lectura de
// usuarios en cada peticion autenticada, y esto se invoca UNA vez en la vida de
// una instalacion. Juntarlos obligaria a todo doble de la afiliacion a
// implementar una escritura que no usa.
//
// El contrato incluye la unicidad: la implementacion inserta solo si la tabla
// esta vacia, EN LA MISMA SENTENCIA, y devuelve ErrYaHayUsuarios si no lo
// estaba. Comprobarlo con un recuento previo deja una ventana entre el SELECT y
// el INSERT por la que cabe una segunda cuenta de administrador.
type RepositorioProvisionInicial interface {
	CrearPrimerAdministrador(ctx context.Context, u Usuario, hash string) error
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
	ListarObras(ctx context.Context, p Paginacion) ([]Obra, error)
	ObraPorID(ctx context.Context, id string) (Obra, error)
	Declaraciones(ctx context.Context) (map[string]repertorio.Declaracion, error)
	DeclaracionDeObra(ctx context.Context, obraID string) (repertorio.Declaracion, error)
}

// CatalogoObras es la escritura y la busqueda del catalogo maestro.
//
// Esta separado de [RepositorioRepertorio] y no fusionado con el, aunque las
// dos toquen la tabla `obras`, porque son dos lecturas distintas del mismo
// dato y el ADR 0003 pide un puerto por responsabilidad:
//
//   - [RepositorioRepertorio] sirve al motor de reparto. Devuelve [Obra], que
//     es una PROYECCION: identidad mas EstadoDecl, el estado de la declaracion
//     derivado de `declaraciones`. No sabe de coautores.
//   - CatalogoObras es el ABM del catalogo. Habla en [repertorio.Obra], que es
//     la ENTIDAD, con su identificador inmutable y su invariante. No sabe de
//     `declaraciones`, y no debe saber: el catalogo no reparte.
//
// Fundirlas daria un tipo que a la vez lleva el invariante de construccion y
// un campo derivado de otra tabla, y cada lectura tendria que decidir cual de
// las dos mitades vale.
//
// Registrar devuelve [ErrObraDuplicada] si el identificador ya existe. Lo
// decide la clave primaria, no un SELECT previo: entre la consulta y el INSERT
// cabe otra peticion.
//
// Registrar y Actualizar son ATOMICOS por contrato -la obra y sus coautores
// entran o no entran-. Una obra a medias, sin coautores, viola la invariante
// de [repertorio.Obra] en cuanto alguien la lea de vuelta.
//
// Bloquear toma el cerrojo de fila de una obra sin leerla, o devuelve
// [ErrNoEncontrado]. Solo tiene sentido DENTRO de una [UnidadDeTrabajo]:
// [Catalogo.ActualizarMetadatosObra] la llama justo antes de PorID para que
// esa lectura no pueda adelantarse al commit de otro PATCH concurrente sobre
// la MISMA obra -- ver el comentario de ese metodo para la carrera exacta que
// evita.
type CatalogoObras interface {
	Registrar(ctx context.Context, o repertorio.Obra) error
	Actualizar(ctx context.Context, o repertorio.Obra) error
	PorID(ctx context.Context, id string) (repertorio.Obra, error)
	Buscar(ctx context.Context, f FiltroObras) ([]repertorio.Obra, error)
	Bloquear(ctx context.Context, id string) error
}

// GestionDeclaraciones es la escritura y el historial de la Declaracion de
// Obra: el ABM de la #23 y lo que consume el editor de splits de la #30.
//
// Separado de [RepositorioRepertorio] por la misma razon que [CatalogoObras]
// esta separado de el (ver su comentario arriba): son dos lecturas del mismo
// dato para dos consumidores distintos. RepositorioRepertorio sirve al motor
// de reparto y al estado del catalogo con la declaracion VIGENTE, sin
// versiones visibles. GestionDeclaraciones habla en versiones explicitas
// porque el criterio de la #23 pide ver el historial y resolver la vigente en
// un instante pasado.
//
// Guardar cierra la version abierta de la obra -si la hay- y abre una nueva
// con las partes que llegan, en una sola operacion atomica por contrato: es
// lo mismo declarar por primera vez que editar, la unica diferencia es si
// habia una version que cerrar. Devuelve [ErrNoEncontrado] si la obra no
// existe en el catalogo.
//
// El asiento de auditoria (ADR 0006) entra en el MISMO contrato atomico:
// actorID es quien firma el hecho, y la implementacion lo asienta en la misma
// transaccion que la version. No es un puerto ni una llamada aparte -eso deja
// una version guardada sin asiento si la segunda llamada falla- sino la unica
// forma de que "version + asiento" sea una sola cosa o ninguna.
type GestionDeclaraciones interface {
	// Guardar devuelve la version nueva y el vigente_desde que de verdad quedo
	// escrito: no siempre es el ahora que llego, porque la implementacion
	// puede ajustarlo -por ejemplo para que no coincida con el vigente_desde
	// de la version que cierra-. Devolver el valor real y no el que se envio
	// es lo que evita que el llamador informe una ventana de vigencia que la
	// base nunca tuvo.
	Guardar(ctx context.Context, d repertorio.Declaracion, ahora time.Time, actorID string) (version int, vigenteDesde time.Time, err error)
	// Historial sirve una pagina de versiones, tomada desde la MAS RECIENTE
	// hacia atras y devuelta en orden ascendente. Ver [Store.Historial].
	Historial(ctx context.Context, obraID string, pag Paginacion) ([]VersionDeclaracion, error)
	VigenteEn(ctx context.Context, obraID string, momento time.Time) (VersionDeclaracion, error)

	// VigentesDeObras devuelve la version ABIERTA de cada una de las obras
	// pedidas, indexada por obra, en UNA consulta para toda la lista.
	//
	// Existe aparte de [VigenteEn] porque responde otra pregunta. VigenteEn
	// resuelve que regia en un INSTANTE -por eso recibe el momento y por eso
	// su ausencia es [ErrNoEncontrado]-. Esto resuelve que rige AHORA MISMO
	// para las obras de una pagina del catalogo, y con N obras preguntar una
	// por una seria N+1 viajes contra la base: es la misma cuenta que
	// [RepositorioRepertorio.ListarObras] ya evita para las partes.
	//
	// # Una obra sin declaracion NO aparece en el mapa
	//
	// Y la ausencia es el dato, no un hueco que rellenar con la Declaracion
	// cero: `Estado()` da "incompleta" tanto para una obra que nunca se
	// declaro como para una declarada que no suma 100 (`R-04`, `RD 13.1.3`),
	// asi que devolver una entrada de ceros para la primera haria que el
	// llamador no pudiera distinguir las dos -y una pantalla que las pinta
	// igual afirma una declaracion que nadie hizo-. Quien necesite el estado
	// de una obra ausente del mapa lo compone sabiendo que la version es nil.
	VigentesDeObras(ctx context.Context, obraIDs []string) (map[string]VersionDeclaracion, error)
}

// Paginacion es el recorte comun de [CatalogoObras.Buscar] y
// [RepositorioRepertorio.ListarObras]. Misma forma a proposito: la nota 4 de
// #86 pide que GET /obras y ListarObras paginen juntos, no cada uno a su aire.
//
// Limite cero significa "usar el por defecto" ([LimiteObrasPorDefecto]): es el
// tope que cierra el catalogo real de REDES SGC. [LimiteSinTope] es el
// centinela explicito para pedir el catalogo entero -"todo" se dice, no se
// obtiene dejando el campo a cero-. Desplazamiento cero es el inicio.
//
// Los valores ilegales -limite negativo distinto de LimiteSinTope, por encima
// del maximo, o desplazamiento negativo- los rechaza el adaptador HTTP con
// 400; el repositorio solo aplica el defecto.
type Paginacion struct {
	Limite         int
	Desplazamiento int
}

const (
	// LimiteObrasPorDefecto es el tope cuando quien llama no pide otro.
	LimiteObrasPorDefecto = 100
	// LimiteObrasMaximo es el techo que acepta GET /obras. Por encima es 400,
	// no un silencio que lo recorte: quien pide 10_000 tiene que saber que no.
	//
	// Y el mismo techo que acepta GET /titulares (#30). El nombre se queda
	// como esta a proposito: los dos son listados del mismo sistema, 500 es
	// correcto para los dos, y renombrarlo a algo generico tocaria todo lo que
	// ya lo usa para no cambiar ni un valor.
	LimiteObrasMaximo = 500
	// LimiteSinTope pide el catalogo entero. Solo tiene sentido en lecturas
	// internas (p. ej. el motor de reparto via ListarObras); GET /obras lo
	// rechaza como limite negativo.
	LimiteSinTope = -1
)

// ConDefecto pone LimiteObrasPorDefecto cuando Limite llega en cero. No
// toca [LimiteSinTope]: ese centinela ya es una eleccion explicita. No
// recorta ni rechaza: eso es del adaptador HTTP.
func (p Paginacion) ConDefecto() Paginacion {
	if p.Limite == 0 {
		p.Limite = LimiteObrasPorDefecto
	}
	return p
}

// FiltroObras recorta una busqueda en el catalogo. Un campo en su valor cero
// NO filtra, y los que vienen se combinan con Y.
//
// Titulo es parcial porque es el unico campo por el que se busca sin saber el
// dato exacto -es el escalon 3 de la cascada, y la muestra muestra por que:
// el titulo localizado y el original difieren en 16 de 59 filas de Caracol-.
// Los otros tres son exactos: un genero, un anio y un IPI se conocen enteros
// o no se conocen.
//
// La paginacion viaja embebida: Buscar y ListarObras comparten la misma forma
// (issue #90, seguimiento de #86).
type FiltroObras struct {
	Titulo string
	Genero string
	IPI    string
	Anio   int
	Paginacion
}

// FiltroTitulares recorta una busqueda en el padron. Un campo en su valor cero
// NO filtra, y los que vienen se combinan con Y, igual que [FiltroObras].
//
// Nombre es parcial porque por el padron se busca sin saber el nombre exacto
// -"Ana Escritora" o "Ana Escritora de Perez"-, y sin distinguir mayusculas.
// El IPI es exacto: un IPI se conoce entero o no se conoce.
//
// PersonaNatural es un PUNTERO, y no es un capricho. Con un bool a secas, "no
// filtrar" y "solo personas juridicas" serian el mismo valor cero -`false`- y
// no habria forma de preguntar por las productoras, que es lo que ofrece
// `GET /titulares`: quien esta en el padron y NO puede recibir reparto.
//
// IDs pide esas filas del padron y ninguna otra, y una lista vacia NO filtra,
// igual que los tres de arriba. Existe porque `R-01` tiene que decidir sobre
// los titulares que NOMBRA una declaracion, y esos son unos pocos: sin este
// filtro, comprobar la regla en cada guardado obliga a leer el padron entero
// -que no tiene tope- para responder por un punado de ids.
type FiltroTitulares struct {
	Nombre         string
	IPI            string
	PersonaNatural *bool
	IDs            []string
	Paginacion
}

// RepositorioIdentificacion cubre alias, identificadores globales y el
// resultado del matching.
//
// ObraPorIDGlobal recibe los tres identificadores y devuelve ErrNoEncontrado
// si los tres llegan vacios: llamarla sin datos no puede pasar por "no hay
// match".
//
// GuardarMatch escribe r solo si la fila sigue en escalonPrevio, el escalon
// con que se leyo; si no existe o ya cambio, devuelve ErrNoEncontrado sin
// escribir nada.
// GuardarCandidatos REEMPLAZA la bandeja de un uso: son el resultado de una
// corrida contra el catalogo tal como estaba (D9).
type RepositorioIdentificacion interface {
	Alias(ctx context.Context, fuente, tipo, valor string) (obraID string, err error)
	GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error
	ObraPorIDGlobal(ctx context.Context, ida, eidr, imdb string) (obraID string, err error)
	GuardarMatch(ctx context.Context, usoID, escalonPrevio string, r identificacion.Resultado) error
	GuardarCandidatos(ctx context.Context, usoID string, cs []identificacion.Candidato) error
}

// RepositorioUsosDeReparto entrega los usos que ponderan la bolsa de un canal.
//
// El filtro es el par (periodo, canal) y no el periodo suelto: el valor punto
// de `RD 9.1.1` se calcula por canal, y una corrida es una bolsa (ADR 0019).
// Mezclar dos pagadores en una consulta produciria un valor punto que el
// reglamento no reconoce.
//
// anioClasificacion llega resuelto desde el nucleo y no se deduce aqui: la
// regla de que es el ano INMEDIATAMENTE ANTERIOR al periodo es `RD 9.5.4`, y
// dejarla en el adaptador la volveria improbable sin una base de datos.
//
// Un canal sin filas devuelve la lista vacia, no ErrNoEncontrado. UsosDeCanal
// solo devuelve filas con obra identificada (`obra_id IS NOT NULL`): una fila
// pendiente, ONI o excluida (R-27) nunca llega al motor con un `ObraID`
// vacio. [ResumenUsosDeCanal] cuenta cuantas se quedaron fuera y por que --
// pero SOLO cuenta: no reserva su importe. Ver la advertencia en
// [ResumenUsosDeCanal] antes de pasar el resultado de UsosDeCanal a
// [reparto.Reparto].
type RepositorioUsosDeReparto interface {
	UsosDeCanal(
		ctx context.Context, periodo, canalID string, anioClasificacion int,
	) ([]UsoDeReparto, ResumenUsosDeCanal, error)

	// UsosSinCanal cuenta los usos de un periodo -- de cualquier pagador -- que
	// llegaron con `canal_id` vacio. Mientras ningun adaptador de ingesta
	// (`internal/infraestructura/ingesta`) mapee la columna real de canal, esto
	// es lo unico que distingue "el canal no emitio" (cero filas SUYAS) de "la
	// fuente no dijo de que canal eran" (filas ajenas a todos los canales).
	UsosSinCanal(ctx context.Context, periodo string) (int, error)
}

// RepositorioIngesta cubre los reportes recibidos y sus filas.
type RepositorioIngesta interface {
	GuardarReporte(ctx context.Context, id, fuente, periodo, sha, claveObjeto string, nbytes int) error
	GuardarUsos(ctx context.Context, usos []UsoPersistido) error

	// GuardarEntrega escribe el acuse de una entrega Y sus filas como UN SOLO
	// hecho: o entran los dos o no entra ninguno.
	//
	// # Por que no basta con llamar a los dos metodos de arriba
	//
	// Porque el acuse QUEMA la huella. El duplicado lo decide el
	// UNIQUE (sha256, fuente), asi que una fila de `reportes` escrita y un lote
	// que falla despues dejan una entrega registrada con CERO filas y la huella
	// gastada: el cliente vuelve a mandar el mismo archivo -- que es exactamente
	// lo que hace cuando le dicen que su carga fallo -- y se lleva un
	// [ErrReporteDuplicado] para siempre. La entrega no se recupera sin cirugia
	// en la base, y de la boveda no se borra (ADR 0006).
	//
	// Es el mismo agujero que [Ingesta.IngerirReporte] evita parseando antes de
	// tocar nada, por la otra puerta: alli el archivo esta roto, aqui el archivo
	// esta bien y lo que falla es la escritura.
	//
	// # Por que es un metodo del puerto y no una transaccion del caso de uso
	//
	// Por lo mismo que la atomicidad del lote en GuardarUsos: el nucleo no puede
	// abrir una transaccion sin aprenderse el driver, que es justo lo que este
	// puerto oculta -- y depguard deniega `pgx` en esta capa --. Lo que el caso de
	// uso SI decide es el limite, y lo declara eligiendo esta llamada en vez de
	// las otras dos. Es la misma forma que [CatalogoObras.Registrar], que mete la
	// obra y sus coautores juntas, y que [RepositorioResultados.GuardarResultado].
	//
	// La boveda se queda FUERA, y no puede ser de otra manera: de un fichero
	// escrito no se hace rollback. El resto que eso deja -- un objeto sin acuse --
	// es inerte y se recupera solo, porque la clave del objeto es su contenido
	// (ver [Ingesta.GuardarReporte]).
	//
	// Devuelve [ErrReporteDuplicado] si esa fuente ya entrego esos mismos bytes.
	GuardarEntrega(ctx context.Context, rep Reporte, usos []UsoPersistido) error

	UsosSinResolver(ctx context.Context) ([]UsoPersistido, error)
	UsosDePeriodo(ctx context.Context, periodo string) ([]UsoPersistido, error)
	UsoPorID(ctx context.Context, id string) (UsoPersistido, error)
	ListarRechazos(ctx context.Context) ([]UsoPersistido, error)

	// RechazosDeReporte devuelve una PAGINA del log de rechazos de una entrega,
	// en el orden de fila del archivo.
	//
	// La cota la elige quien llama y la aplica la base. No es una comodidad: un
	// archivo con la cabecera equivocada rechaza TODAS sus filas, y el log entero
	// se leia, se traducia a JSON y se pintaba completo -un `<tr>` por fila-, con
	// lo que eso hace a la pestana y a la memoria del proceso. Paginar no es
	// truncar en silencio: el recuento total sigue viajando en `Carga.rechazados`
	// (ver [Ingesta.Cargas]), que es de donde quien lee saca el "N de M" sin
	// afirmar una cifra que nadie conto.
	//
	// Devuelve [ErrNoEncontrado] si la entrega no existe, y una lista vacia -no
	// nil- si existe y no tuvo rechazos, o si la pagina pedida cae mas alla del
	// final. Una lista vacia no puede significar "no existe": seria la misma
	// ambiguedad que [Ingesta.Cargas] evita validando el periodo.
	RechazosDeReporte(ctx context.Context, reporteID string, pag Paginacion) ([]UsoPersistido, error)

	// ListarCargas devuelve las entregas recibidas, de la mas reciente a la
	// mas antigua. Un periodo vacio NO filtra.
	//
	// Devuelve tambien los dos recuentos porque separados no significan nada:
	// una carga de la que solo se sabe que llego no dice si entro entera, y
	// "entro entera" es justamente lo que hay que poder mirar para saber si
	// falta pedirle algo al cliente.
	ListarCargas(ctx context.Context, periodo string) ([]CargaReporte, error)
}

// Formatos en los que puede llegar una entrega. Son la mitad de la clave con
// la que se elige el adaptador que sabe leerla.
//
// Viven en el nucleo y no en el adaptador aunque nombren formatos de archivo:
// lo que el nucleo necesita saber es que una misma fuente puede entregar lo
// mismo de varias maneras, no como se parsea ninguna de ellas. Que detras del
// XLSX haya excelize y detras del CSV encoding/csv no se sabe desde aqui, y
// depguard lo deja por escrito denegando los dos paquetes en esta capa.
const (
	FormatoXLSX = "xlsx"
	FormatoCSV  = "csv"
	FormatoJSON = "json"
)

// ClaveLector identifica al adaptador de formato de una entrega.
//
// Es el PAR (fuente, formato) y no la fuente sola porque son dos ejes
// independientes y los dos varian de verdad: la parrilla de Caracol y el
// reporte de Netflix traen columnas distintas -- no comparten ni un nombre de
// columna, esta medido en `docs/dominio/fuentes-datos.md` --, y una misma
// fuente puede entregar su mismo contenido en .xlsx hoy y en CSV manana sin
// que su mapa de columnas cambie una linea.
//
// Con la fuente sola como clave, dar de alta el CSV de Caracol obligaria a
// inventarse una fuente "caracol-csv", y a partir de ahi la fuente dejaria de
// significar quien entrego -- que es lo que indexa `alias_obra` y lo que ata
// una fila a su usuario del `RD 8` --, para significar quien entrego y como.
type ClaveLector struct {
	Fuente  string
	Formato string
}

// LectorReporte convierte los bytes de una entrega en filas del esquema
// canonico.
//
// Es el puerto de los adaptadores de formato. Lo satisface un adaptador por
// PAR (fuente, formato); ver [ClaveLector].
//
// # Por que devuelve las rechazadas mezcladas con las buenas
//
// Una fila que no se puede normalizar NO se descarta: viaja en el mismo
// resultado con su [UsoPersistido.RechazoMotivo] puesto, y es
// [Ingesta.GuardarUsos] quien la encamina al log de rechazos. Devolver dos
// slices dejaria al adaptador decidir que es un rechazo y que es una perdida,
// y la unica forma de perder una fila en silencio es que alguien pueda no
// devolverla.
//
// El motivo lo escribe el adaptador porque ve cosas que aguas arriba ya no se
// ven: que celda no se pudo convertir, que placeholder traia -- el `--` de
// `episode_nbr` --, en que fila del archivo estaba. GuardarUsos respeta el
// motivo que ya viene puesto justamente para no perderlo.
//
// # Y por que el error es otra cosa
//
// El error es el fallo ESTRUCTURAL: el archivo no se puede abrir, la hoja no
// esta, falta una columna requerida. Ahi no hay filas buenas que salvar, y el
// contrato es que no se persiste NADA -- ni la boveda --. Se devuelve envuelto
// en [ErrReporteInvalido] y nombrando el campo, que es lo que permite volver a
// pedirle al cliente exactamente eso.
type LectorReporte interface {
	Leer(datos []byte) ([]UsoPersistido, error)
}

// RepositorioONI es la cola manual. Separado de identificacion porque son dos
// modulos distintos del ADR 0003.
type RepositorioONI interface {
	Listar(ctx context.Context) ([]UsoPersistido, error)
}

// RepositorioRecaudo expone las bolsas. Recaudo es el unico modulo que conoce
// Usuario, Convenio y Tarifa; aguas abajo solo circula la bolsa (ADR 0003).
//
// BolsasDePeriodo existe aparte de ListarBolsas -y no como un filtro opcional-
// por lo mismo que UsosDePeriodo en [RepositorioIngesta]: es la lectura que
// pide el motor de reparto, va por el indice `bolsas_periodo`, y un listado
// entero de todos los periodos no es lo que nadie quiere cuando pregunta por
// uno.
type RepositorioRecaudo interface {
	ListarBolsas(ctx context.Context) ([]BolsaPersistida, error)
	BolsasDePeriodo(ctx context.Context, periodo string) ([]BolsaPersistida, error)
	BolsaPorID(ctx context.Context, id string) (BolsaPersistida, error)
	ListarUsuarios(ctx context.Context) ([]recaudo.Usuario, error)
}

// GestionRecaudo registra lo que se cobro. Es el lado de escritura de
// [RepositorioRecaudo], separado por la misma razon que [GestionDeclaraciones]
// lo esta de [RepositorioRepertorio]: quien solo lee no tiene por que poder
// escribir dinero.
//
// El asiento de auditoria (ADR 0006) entra en el MISMO contrato atomico que la
// escritura, y por eso `ahora` y `actorID` son parametros de estos metodos y no
// una segunda llamada a [BitacoraAuditoria] que el caso de uso orqueste. Una
// bolsa escrita sin asiento es dinero que entro sin que nadie pueda decir de
// donde salio, que es la pregunta 1 del ADR 0006.
//
// `ahora` viene del puerto [Reloj]; el adaptador no llama a time.Now().
type GestionRecaudo interface {
	RegistrarUsuario(ctx context.Context, u recaudo.Usuario, ahora time.Time, actorID string) error
	RegistrarBolsa(ctx context.Context, b BolsaPersistida, ahora time.Time, actorID string) error
}

// ParametroEnFecha resuelve UN parametro normativo vigente en una fecha.
//
// Mas estrecho que [ParametrosNormativos] a proposito: identificar no mueve
// dinero (ADR 0003) y no necesita el snapshot congelado de #118. Una clave sin
// vigencia es un error que la nombra, nunca un cero (ADR 0004).
type ParametroEnFecha interface {
	ParametroVigente(ctx context.Context, clave string, fecha time.Time) (decimal.Decimal, error)
}

// ParametrosNormativos resuelve los parametros con vigencia y organo
// aprobador que exige el ADR 0004.
//
// SnapshotEnFecha se resuelve UNA VEZ, al abrir el proceso, contra la fecha
// del periodo -no contra el ahora del reloj- y queda congelado. Recalcular una
// corrida lee el snapshot del proceso con SnapshotPorID, nunca vuelve a
// resolver: si volviera, cambiar un parametro cambiaria en silencio el
// resultado de una corrida ya hecha.
//
// "Queda congelado" es una ESCRITURA, y por eso este puerto ya no es de solo
// lectura desde la #118: resolver sin persistir el corte dejaria SnapshotPorID
// sin nada que leer, y la unica forma de reproducir la corrida seria volver a
// resolver la fecha -- que es exactamente lo que el parrafo anterior prohibe.
// Lo que se congela es el corte, nunca la tabla de vigencias.
//
// # El id
//
// Esta direccionado por contenido: sale de los pares (clave, valor) que el
// snapshot consume, ordenados. Tres consecuencias que forman parte del
// contrato y no del adaptador que lo cumple:
//
//   - resolver dos veces la misma fecha sobre los mismos valores da el MISMO
//     id, asi que abrir el proceso es idempotente;
//   - dos conjuntos de valores distintos no pueden compartir id;
//   - un parametro que el snapshot no consume no cambia el id, porque no
//     cambia el snapshot.
//
// # Los ausentes
//
// Una clausula sin valor vigente en la fecha NO resuelve a cero ni a un valor
// por defecto (ADR 0004): sale [ErrorParametroAusente], que la nombra. Un
// snapshot a medias es una cifra falsa con aspecto de cifra buena.
type ParametrosNormativos interface {
	SnapshotEnFecha(ctx context.Context, fechaPeriodo time.Time) (id string, s reparto.Snapshot, err error)
	SnapshotPorID(ctx context.Context, id string) (reparto.Snapshot, error)
	Vigentes(ctx context.Context, ahora time.Time) ([]FilaParametro, error)
}

// FilaParametro es un parametro normativo con su procedencia. Sin vigencia y
// organo aprobador no es un parametro, es una constante disfrazada.
//
// Valor es texto y no un decimal porque esta fila se LISTA, no se calcula con
// ella: es la pantalla de administracion del ADR 0004. El adaptador lo entrega
// en la misma forma canonica que entra en el id del snapshot, para que lo que
// se ve en la lista y lo que se congelo sean comparables caracter a caracter.
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
// GuardarResultado es transaccional por contrato: un resultado a medias es
// una cifra que alguien puede leer y pagar.
//
// Nombres largos y no Guardar/PorProceso a secas: el mismo *Store satisface
// [GestionDeclaraciones] (que ya tiene su propio Guardar) y este puerto, y
// tambien [RepositorioReservas] y [RepositorioReclamacionesReserva] mas
// abajo -- mismo patron que AsientoPorID en [BitacoraAuditoria].
type RepositorioResultados interface {
	GuardarResultado(ctx context.Context, procesoID string, r reparto.Resultado) error
	ResultadoPorProceso(ctx context.Context, procesoID string) (reparto.Resultado, error)
}

// RepositorioLiquidacion sirve lo que le corresponde a un titular.
type RepositorioLiquidacion interface {
	DeTitular(ctx context.Context, titularID string) ([]reparto.LineaTitular, error)
}

// RepositorioReservas guarda y lee las reservas de errores tecnicos (RD 14), una por corrida.
// LiberarSaldoReserva bloquea reserva y rendimiento, entrega el saldo a fn, y persiste todo en una transaccion.
type RepositorioReservas interface {
	CrearReserva(ctx context.Context, r reparto.PoolReserva) error
	ReservaPorProceso(ctx context.Context, procesoID string) (reparto.PoolReserva, error)
	LiberarSaldoReserva(ctx context.Context, procesoID, vigenciaRendimiento string, rendimientoAUsar decimal.Decimal,
		fn func(saldoActual decimal.Decimal) (nuevoSaldo decimal.Decimal, lineas []reparto.LineaTitular, err error)) error
}

// RepositorioRendimientos guarda y lee los pools de rendimientos financieros (RD 10), uno por (circuito, vigencia).
// AcrecerRendimiento suma en una sola sentencia; ActualizarMontoRendimiento bloquea, entrega el monto a fn, y persiste monto+lineas en una transaccion.
type RepositorioRendimientos interface {
	AcrecerRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia string, incremento decimal.Decimal) error
	PorCircuitoYVigencia(ctx context.Context, circuito reparto.Circuito, vigencia string) (reparto.PoolRendimiento, error)
	ActualizarMontoRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia, procesoID string,
		fn func(montoActual decimal.Decimal) (nuevoMonto decimal.Decimal, lineas []reparto.LineaTitular, err error)) error
}

// RepositorioReclamacionesReserva guarda y lee los reclamos contra la
// reserva (RD 14.5).
type RepositorioReclamacionesReserva interface {
	GuardarReclamacion(ctx context.Context, r reparto.ReclamacionReserva) error
	ReclamacionPorID(ctx context.Context, id string) (reparto.ReclamacionReserva, error)
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
// AsientoPorID y no PorID: el mismo *Store satisface tambien [CatalogoObras],
// que ya tiene un PorID con otra firma -misma razon por la que
// [RepositorioRepertorio] tiene ObraPorID y no PorID-.
type BitacoraAuditoria interface {
	Asentar(ctx context.Context, a Asiento) error
	De(ctx context.Context, refTipo, refID string) ([]Asiento, error)
	AsientoPorID(ctx context.Context, id string) (Asiento, error)
}

// UnidadDeTrabajo es el limite de transaccion cuando un caso de uso escribe
// por DOS puertos y las dos escrituras son un solo hecho.
//
// # Por que hace falta un puerto para esto
//
// Cuando el asiento y la fila que explica salen del MISMO puerto, el limite lo
// declara el contrato de ese metodo y no hace falta nada mas: es lo que hacen
// [GestionDeclaraciones.Guardar] y [GestionRecaudo.RegistrarBolsa], que
// reciben `ahora` y `actorID` y asientan por dentro.
//
// El catalogo no puede resolverlo asi. El ADR 0003 pide que la trazabilidad
// entre por [BitacoraAuditoria] y no por el contrato del modulo -- "ningun
// modulo escribe en la trazabilidad de otro" solo es exigible si Asentar no
// esta en el contrato que todos comparten --, asi que [Catalogo] sostiene los
// dos puertos y es EL quien tiene que decir que la obra y su asiento son una
// sola cosa. Sin esto solo quedan dos llamadas seguidas, y un alta confirmada
// cuyo asiento fallo despues es justo la escritura huerfana que el ADR 0006
// prohibe.
//
// # Por que fn recibe un context
//
// Porque es lo unico que puede transportar la transaccion sin que el nucleo
// aprenda el driver: depguard deniega `pgx` en esta capa, asi que una firma
// con la transaccion como parametro tipado no se puede ni escribir aqui. El
// contrato es que los puertos invocados DENTRO de fn tienen que recibir ese
// ctx -- el que fn recibe, no el de fuera --; un puerto llamado con el ctx
// exterior escribe fuera de la unidad y se confirma aparte.
//
// Se confirma si fn devuelve nil y se revierte con cualquier error, que sube
// sin envolver para que quien llama distinga sus centinelas. Una
// implementacion puede ser reentrante (una unidad dentro de otra es la misma
// unidad) pero nadie debe depender de que lo sea.
type UnidadDeTrabajo interface {
	EnUnidad(ctx context.Context, fn func(ctx context.Context) error) error
}

// ColaTrabajos desacopla la ingesta del matching y del reparto por lotes.
//
// Los tres metodos son los que pide el issue #35; el detalle de por que la
// cola es una tabla propia y no River esta en el ADR 0015.
//
// El contrato tiene una obligacion que no se ve en las firmas: Tomar reclama
// en exclusiva. Dos workers que llamen a la vez tienen que recibir trabajos
// distintos o ErrSinTrabajo, nunca el mismo. El adaptador de PostgreSQL lo
// resuelve con SELECT ... FOR UPDATE SKIP LOCKED; cualquier otro tiene que
// dar la misma garantia, porque el nucleo no la comprueba.
type ColaTrabajos interface {
	// Encolar es IDEMPOTENTE por clave natural. Devuelve false, sin error,
	// cuando el trabajo ya estaba encolado: reintentar el encolado no es un
	// fallo, y duplicarlo pagaria un periodo dos veces.
	Encolar(ctx context.Context, clave ClaveTrabajo, payload []byte) (encolado bool, err error)

	// Tomar reclama el trabajo pendiente mas antiguo cuya espera de reintento
	// ya vencio, lo marca en curso y suma uno a Intentos. Devuelve
	// ErrSinTrabajo cuando no hay ninguno: no hacen falta un ok y un error a
	// la vez para decir lo mismo.
	//
	// `ahora` entra por parametro y no de now() por lo mismo que en Sesiones:
	// una espera de reintento que solo se puede probar esperando no se prueba.
	Tomar(ctx context.Context, ahora time.Time) (Trabajo, error)

	// Cerrar termina un trabajo EN CURSO. Cerrar uno que no lo esta devuelve
	// ErrNoEncontrado: un cierre por duplicado es un defecto del worker, no
	// algo que convenga tragarse.
	Cerrar(ctx context.Context, id int64, c Cierre) error
}

// Trabajo es una unidad de trabajo tomada de la cola.
//
// Intentos es el numero de veces que se ha tomado ESTE trabajo, ya contando la
// actual. Clave.Corrida es otra cosa: cual corrida logica del periodo es. Ver
// [ClaveTrabajo].
type Trabajo struct {
	ID       int64
	Clave    ClaveTrabajo
	Payload  []byte
	Intentos int
}

// Calendario dispara las corridas segun RD 10 y RD 12.
//
// Es dato que administra el Consejo Directivo, no configuracion de operacion
// (ADR 0004): por eso las fechas se leen de aqui y no de un cron del sistema
// operativo.
type Calendario interface {
	// Pendientes devuelve los periodos cuya fecha de apertura ya llego y que
	// todavia no se han disparado.
	Pendientes(ctx context.Context, hoy time.Time) ([]string, error)

	// MarcarDisparado deja constancia de que el periodo ya se encolo.
	// Devuelve ErrNoEncontrado si el periodo no esta en el calendario.
	MarcarDisparado(ctx context.Context, periodo string) error
}

// RepositorioAlertas es la bandeja de anomalias de un periodo (#37).
//
// # Guardar tiene que ser IDEMPOTENTE, y no es un detalle del adaptador
//
// [Anomalias.Evaluar] se puede correr las veces que haga falta -- al cerrar la
// ingesta, otra vez despues de arreglar una declaracion, otra vez antes de la
// compuerta de #34 -- y las tres pasadas ven las mismas anomalias. Sin clave
// natural, la tercera pasada triplica el tablero y el contador de la compuerta
// deja de significar nada.
//
// La clave es (periodo, tipo, ref_tipo, ref_id, ref_titular), que es la
// identidad del HALLAZGO: la misma anomalia sobre el mismo registro del mismo
// periodo es una sola alerta, se detecte una vez o veinte. El detalle NO entra
// en la clave a proposito -- es prosa, y reescribir una frase duplicaria la
// fila --.
//
// Devuelve cuantas filas nuevas entraron, no cuantas se le pasaron: es la
// unica forma de que quien llama pueda decir "esta pasada encontro tres
// anomalias que antes no estaban".
//
// # Una alerta ya resuelta NO se reabre
//
// Volver a detectar algo que una persona marco como resuelto deja la fila como
// esta. Es deliberado: la resolucion de #39 actua sobre el REGISTRO OFENSOR
// -asignar la obra, descartar la fila-, asi que si la anomalia sigue ahi es
// porque el registro sigue igual, y reabrirla borraria la decision de quien la
// cerro sin que nadie lo pidiera. Queda escrito como limitacion conocida en el
// ADR 0020.
// # Los metodos llevan "Alerta(s)" en el nombre y no es redundancia
//
// `Listar`, `Guardar` y `Resolver` a secas serian mas cortos y no caben: el
// mismo *Store satisface este puerto y [GestionDeclaraciones], que ya tiene un
// `Guardar` con otra firma, y dos metodos con el mismo nombre no caben en un
// tipo. Es lo mismo que le paso a `Store.Cerrar` cuando llego
// [ColaTrabajos.Cerrar] (ver [postgres.Store.CerrarPool]). El issue ademas
// nombra `ResolverAlerta` por su nombre.
type RepositorioAlertas interface {
	// ListarAlertas devuelve las que cuadran con el filtro, de la mas reciente
	// a la mas antigua y desempatando por id. Sin coincidencias devuelve la
	// lista vacia, no ErrNoEncontrado.
	//
	// ESTA PAGINADO. `FiltroAlertas` lleva [Paginacion] y el cero significa
	// [LimiteObrasPorDefecto], no "todo": una evaluacion real puede dejar del
	// orden de 10.000 alertas -- tres de los seis detectores emiten una por
	// fila de uso y KR-1 habla de lotes de 10.000 registros -- y devolverlas
	// en un array de ~4 MB a un panel que sondea cada 15 segundos no es
	// servible. Quien necesite TODAS tiene que pedirlo con [LimiteSinTope],
	// explicitamente.
	//
	// Por eso una cuenta NO se hace sobre este metodo. Ver
	// ContarAlertasSinResolver.
	ListarAlertas(ctx context.Context, f FiltroAlertas) ([]Alerta, error)

	// GuardarAlertas escribe las que todavia no estaban. El lote entra entero
	// o no entra ninguna, por lo mismo que [RepositorioIngesta.GuardarUsos]:
	// una evaluacion guardada a medias deja un tablero que no corresponde a
	// ninguna pasada.
	GuardarAlertas(ctx context.Context, alertas []Alerta) (nuevas int, err error)

	// ResolverAlerta marca una alerta y devuelve como quedo. Devuelve
	// ErrNoEncontrado si no existe y ErrAlertaYaResuelta si ya lo estaba --
	// que no es lo mismo: lo primero es un id equivocado, lo segundo es una
	// carrera entre dos personas mirando el mismo tablero.
	ResolverAlerta(ctx context.Context, id, actorID, nota string, cuando time.Time) (Alerta, error)

	// ContarAlertasSinResolver cuenta las abiertas de un periodo entre los
	// tipos que se le pidan. Una lista de tipos vacia cuenta TODOS.
	//
	// Los tipos llegan como parametro y no se deciden en el SQL: cuales
	// bloquean es [anomalias.EsCritica], en el dominio, y un adaptador que
	// llevara su propia lista seria un segundo criterio que nadie mira.
	//
	// # Cuenta en la base, y NO se puede reimplementar sobre ListarAlertas
	//
	// Es un COUNT(*) y no un `len()` de la lista a proposito, porque esta
	// cuenta es la que lee la compuerta de #34: `ListarAlertas` pagina, asi
	// que contar sus filas daria como mucho [LimiteObrasPorDefecto] y un
	// periodo con 3.000 criticas abiertas se leeria como 100 -- o como 0 si
	// alguien pide la segunda pagina de un periodo limpio. Una compuerta que
	// cuenta de menos ABRE EL PASO al reparto, que es el unico sentido en el
	// que puede fallar sin que nadie se entere. Es el mismo modo de fallo que
	// el `cardinality(NULL)` que ya obligo a un COALESCE en el adaptador.
	//
	// [TestLaCompuertaCuentaMasAlertasQueUnaPagina] lo defiende sembrando mas
	// criticas que el tamano de pagina.
	ContarAlertasSinResolver(ctx context.Context, periodo string, tipos []string) (int, error)
}

type RepositorioAnticipos interface {
	Listar(ctx context.Context) ([]Anticipo, error)
	Guardar(ctx context.Context, a Anticipo) error
}
