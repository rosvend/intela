package aplicacion

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errores que los adaptadores devuelven y los casos de uso distinguen.
//
// Existen porque "no encontrado" y "fallo la base de datos" no son lo mismo y
// tratarlos igual tiene consecuencias caras: en la cascada de identificacion,
// tragarse un error transitorio de red como si fuera "no hay alias"
// reclasifica un uso como ONI en silencio, y eso luego se desenreda a mano.
var (
	// ErrNoEncontrado: la consulta fue bien y no hay fila.
	ErrNoEncontrado = errors.New("no encontrado")

	// ErrSinTrabajo: la cola esta vacia. No es un fallo.
	ErrSinTrabajo = errors.New("sin trabajo pendiente")

	// ErrNoAutorizado: el actor esta autenticado pero su rol no basta.
	ErrNoAutorizado = errors.New("no autorizado")

	// ErrCredenciales: usuario o clave incorrectos. Deliberadamente sin
	// distinguir cual de los dos.
	ErrCredenciales = errors.New("credenciales invalidas")

	// ErrParametroAusente: falta un parametro normativo para el calculo.
	// No se inventa un valor por defecto: se falla (ADR 0004).
	//
	// Quien resuelve un snapshot entero lo devuelve dentro de
	// [ErrorParametroAusente], que ademas NOMBRA las clausulas que faltan.
	ErrParametroAusente = errors.New("parametro normativo ausente")

	// ErrConflicto: la fila ya existe. En afiliaciones, el indice parcial
	// cubre correo e IPI no vacio de una solicitud activa (pendiente o
	// admitida). En titulares, el IPI no vacio es unico. Distinto de
	// ErrNoEncontrado: aqui la consulta encontro de mas, no de menos.
	ErrConflicto = errors.New("ya existe una solicitud o afiliacion con esos datos")

	// ErrClaveInvalida: la clave del alta no cumple el minimo. Se distingue
	// de ErrDocumentoInvalido porque quien la recibe tiene que saber que
	// campo rehacer, y de ErrCredenciales porque aqui todavia no hay
	// sesion que rechazar.
	ErrClaveInvalida = errors.New("la clave tiene que tener entre 8 y 72 caracteres")

	// ErrDocumentoInvalido: el adjunto no es un PDF o una imagen, o viene
	// vacio. El dominio no mira bytes; esto lo decide el caso de uso antes
	// de mandarlos al almacen.
	ErrDocumentoInvalido = errors.New("el documento tiene que ser un pdf o una imagen y no puede estar vacio")

	// ErrSnapshotCorrupto: bajo ese id hay filas congeladas que no forman el
	// snapshot que el id anuncia.
	//
	// El id de un snapshot esta direccionado por contenido: es el sha256 de
	// sus pares (clave, valor) ordenados. Eso lo convierte en una suma de
	// verificacion, y entonces "las filas no hashean a su id" es un caso
	// posible y hay que poder decirlo. Lo mismo cuando al conjunto congelado
	// le falta una clausula: nunca fue un snapshot valido.
	//
	// Es la hermana de ErrEvidenciaCorrupta y existe por lo mismo: servir esas
	// filas como si fueran el snapshot devolveria una corrida "reproducida"
	// con cifras que no son las que se pagaron, y eso no se puede distinguir
	// mirando el resultado. No es un fallo de infraestructura ni un "no
	// encontrado".
	ErrSnapshotCorrupto = errors.New("snapshot de parametros corrupto")

	// ErrTasaAmbigua: dos claves `cambio.*` normalizan al mismo codigo ISO.
	//
	// `cambio.USD` y `cambio.usd` son filas DISTINTAS para el esquema --
	// `parametros.clave` no tiene collation especial -- pero [reparto.Snapshot]
	// solo tiene una entrada por moneda (`Tasas[iso]`). Sin este centinela, la
	// que ordena despues por bytes pisa a la otra en el mapa sin que nada lo
	// diga, y un factor de conversion que alguien cargo de verdad desaparece.
	// Es la misma disciplina de "una sola respuesta por clave" que ya exige la
	// EXCLUDE de vigencias, aplicada al codigo ISO derivado en vez de a la
	// clave literal.
	ErrTasaAmbigua = errors.New("tasa de cambio ambigua")

	// ErrActorAusente: un hecho que va FIRMADO llego sin quien lo firme.
	//
	// El ADR 0006 exige saber quien hizo cada hecho de los que nacen de una
	// accion de una persona. La base no lo impide -- `asientos.actor_id` es
	// nullable y el adaptador convierte el actor vacio en NULL --, y ese hueco es
	// deliberado: el ADR solo pide el actor "en ese ultimo caso", el de la
	// decision manual, asi que un hecho que el sistema produzca solo (un
	// calculo, una identificacion automatica) podra asentarse sin firma el dia
	// que exista. Lo que NO puede pasar es que un caso de uso que recibe un
	// actor de la sesion lo pierda por el camino y deje el asiento sin firmar:
	// eso lo cierra [exigirActor], en esta capa, antes de escribir nada.
	ErrActorAusente = errors.New("actorID vacio")

	// ErrUsuarioInvalido: los datos de una cuenta nueva no cumplen el esquema.
	//
	// Se envuelve siempre con el campo concreto que falla, por la misma razon
	// que ErrReporteInvalido: quien provisiona una instalacion lo hace desde
	// una terminal y no tiene un formulario que le marque el campo.
	ErrUsuarioInvalido = errors.New("usuario invalido")

	// ErrYaHayUsuarios: la instalacion ya estaba provisionada.
	//
	// No es un fallo de escritura y no es un dato invalido: la operacion es de
	// una sola vez y ya se hizo. Distinguirlo es lo que permite invocarla sin
	// miedo -- una segunda invocacion no hace nada -- y lo que evita que un
	// reintento cree una segunda cuenta de administrador que nadie pidio.
	ErrYaHayUsuarios = errors.New("ya hay usuarios: la instalacion ya estaba provisionada")

	// ErrObraDuplicada: ya hay una obra con ese identificador en el catalogo.
	//
	// No es "no se pudo escribir" y no es "los datos son invalidos": el alta
	// estaba bien formada y el catalogo ya la tiene. Distinguirlo es lo que
	// deja responder 409 en vez de 500, y lo que hace comprobable el criterio
	// "un segundo alta con el mismo identificador se rechaza".
	ErrObraDuplicada = errors.New("ya existe una obra con ese identificador")

	// ErrReporteDuplicado: esa fuente ya entrego exactamente esos bytes.
	//
	// Es la deteccion de duplicado POR HUELLA, no por nombre de archivo: el
	// reglamento se fija en la identidad del contenido, y los contadores de
	// fila del estilo Id_Ntx se renumeran en cada entrega. La decide el
	// UNIQUE (sha256, fuente) del esquema, que es la unica fuente de verdad;
	// el adaptador traduce esa violacion a este centinela.
	//
	// No es un fallo de infraestructura: es una respuesta del negocio, y el
	// adaptador HTTP la convierte en 409 y no en 500.
	ErrReporteDuplicado = errors.New("reporte duplicado")

	// ErrReporteInvalido: la entrega no cumple la estructura minima.
	//
	// Se envuelve siempre con el campo concreto que falla. La skill de ingesta
	// es explicita: el mensaje tiene que decir QUE falta o esta mal formateado,
	// porque es lo que permite volver a pedirle al cliente exactamente eso.
	ErrReporteInvalido = errors.New("reporte invalido")

	// ErrObjetoYaExiste: esa clave del almacen ya tiene contenido.
	//
	// Un AlmacenObjetos no sobrescribe (ADR 0006), asi que necesita una forma
	// de decir "ya estaba" que no se confunda con un fallo de escritura. Es la
	// simetrica de ErrNoEncontrado en Obtener, y existe por la misma razon:
	// que el nucleo no tenga que reconocer los errores del sistema de ficheros
	// ni los de S3.
	ErrObjetoYaExiste = errors.New("objeto ya existe")

	// ErrEvidenciaCorrupta: bajo la clave hay bytes que no son los que dice la
	// huella.
	//
	// No es un fallo de escritura ni un duplicado, y por eso no puede compartir
	// centinela con ninguno de los dos: es que la boveda YA tenia contenido
	// bajo esa clave y ese contenido no hashea a lo que el acuse iba a
	// certificar. Un objeto desgarrado por un fallo anterior, una copia
	// restaurada a medias o un almacen que no escriba atomicamente llegan asi.
	//
	// Aceptarlo dejaria una fila en `reportes` certificando un SHA-256 que el
	// objeto real no tiene: una cifra que dice de donde salio y no se puede
	// comprobar, que es exactamente lo que el ADR 0006 existe para impedir. El
	// handler de #29 tampoco lo puede mandar a un 500 generico.
	ErrEvidenciaCorrupta = errors.New("evidencia corrupta")

	// ErrTitularInexistente: la declaracion nombra un titular_id que no esta en
	// el padron.
	//
	// Distinto de ErrNoEncontrado: ese centinela es "el recurso de la URL no
	// esta" (la obra del path); este es "un dato DENTRO del cuerpo de la
	// peticion senala una entidad que no existe" -el mismo caso que ya anticipa
	// el comentario de esClaveForanea en postgres/errores.go sobre la FK de
	// `declaraciones` hacia `titulares`-. Confundirlos convierte un typo de
	// titular_id en el JSON del cliente en un 404 que dice "la obra no esta en
	// el catalogo", que no es lo que paso.
	ErrTitularInexistente = errors.New("ese titular no existe")

	// ErrTitularNoEsPersonaNatural: la declaracion nombra un titular que SI
	// esta en el padron y que no puede recibir reparto, porque no es persona
	// natural (`R-01`, `RD 4.5`).
	//
	// No es ErrTitularInexistente, y la diferencia es la razon de ser de los
	// dos: alli el titular_id no resuelve a nadie y el defecto esta en el dato
	// que llego; aqui el dato es correcto -una productora tiene su fila en el
	// padron, con su nombre y su clase- y lo que la rechaza es la REGLA. Los
	// dos salen como 400 porque el campo viene en el cuerpo de la peticion,
	// pero dicen cosas distintas y por eso llevan mensajes distintos: decirle
	// "no existe" a quien mando el id de una sociedad que si existe lo manda a
	// buscar un error que no cometio.
	//
	// Existe porque hasta ahora la unica barrera de R-01 en el camino de la
	// declaracion era el trigger `resultados_titular_persona_natural`
	// (migracion 00001), que dispara en `resultados_titular`, es decir al
	// PAGAR: la declaracion con una sociedad dentro se guardaba con 200 y el
	// reparto la rechazaba mucho mas tarde, con una excepcion cruda de
	// Postgres y con el dinero ya en juego. Este centinela es lo que permite
	// decirlo en la puerta de entrada, antes de abrir la version.
	//
	// El trigger no se toca: sigue siendo la ultima linea, y la unica que
	// cubre lo que entre por SQL crudo -lo dice el comentario de
	// `afiliacion.Titular.PuedeRecibirReparto`-. Esto es la mitad del nucleo.
	ErrTitularNoEsPersonaNatural = errors.New("ese titular no es persona natural")

	// ErrIPIQueNoCuadra: la parte declara un IPI que no es el del titular en el
	// padron.
	//
	// El IPI es el identificador de la sociedad de gestion en el sistema CISAC
	// (`RD 3`), y es lo que aguas abajo dice A QUIEN se le paga: la columna
	// `resultados_titular.ipi` guarda el valor que llego en la declaracion, no
	// el del padron. Dos numeros distintos para el mismo titular significan que
	// el reparto puede pagarle a una persona con el identificador de otra, y eso
	// no lo caza ninguna otra comprobacion: `declaraciones.ipi` es TEXT NOT NULL
	// sin FK ni CHECK (migracion 00001), asi que el esquema acepta cualquier
	// cadena.
	//
	// No es ErrTitularInexistente ni ErrTitularNoEsPersonaNatural, y la
	// diferencia importa: ahi lo que falla es la ENTIDAD -no resuelve a nadie, o
	// resuelve a quien la regla no admite-, y aqui la entidad esta bien y lo que
	// discrepa es un DATO de la parte. Quien edita tiene que corregir un numero,
	// no cambiar de titular.
	//
	// Se compara contra el padron y no se sobrescribe en silencio: poblar el IPI
	// desde el padron ignorando lo que llego haria que la pantalla y la base
	// discrepasen sin decirlo, y una discrepancia que nadie ve es la que se
	// descubre en una auditoria.
	ErrIPIQueNoCuadra = errors.New("el IPI declarado no es el del padron")

	// ErrBolsaDuplicada: ya hay una bolsa para ese usuario, periodo y circuito.
	//
	// No es "no se pudo escribir" y no es un dato invalido: el alta estaba bien
	// formada y esa bolsa ya existe. Distinguirlo es lo que deja responder 409
	// en vez de 500, y sobre todo lo que impide que cargar dos veces el mismo
	// reporte de recaudo DUPLIQUE la bolsa de un periodo -- que aguas abajo es
	// repartir dos veces el mismo dinero.
	//
	// La decide el UNIQUE (usuario_id, periodo, circuito) del esquema, que es
	// la unica fuente de verdad; el adaptador traduce esa violacion a este
	// centinela. El circuito entra en la clave a proposito: nacional e
	// internacional del mismo usuario y periodo son dos bolsas legitimas y
	// separadas (`RD 10.3`, R-35).
	ErrBolsaDuplicada = errors.New("ya existe una bolsa para ese usuario, periodo y circuito")

	// ErrUsuarioRecaudoInexistente: la bolsa cita un pagador que no esta dado
	// de alta.
	//
	// Es la hermana de ErrTitularInexistente y se distingue de ErrNoEncontrado
	// por lo mismo: ese es "el recurso de la URL no esta", este es "un dato
	// DENTRO del cuerpo senala una entidad que no existe". Confundirlos
	// convierte un typo en `usuario_id` en un 404 que dice que la bolsa no
	// existe, que no es lo que paso.
	//
	// Y no se crea el usuario al vuelo: un `usuario_id` mal escrito se
	// convertiria en un pagador fantasma, y el reparto atribuiria a un canal
	// inexistente dinero que alguien pago de verdad.
	ErrUsuarioRecaudoInexistente = errors.New("ese usuario de recaudo no existe")

	// ErrUsuarioDeRecaudoDuplicado: ese pagador ya esta dado de alta.
	//
	// La hermana de ErrObraDuplicada, y existe por lo mismo: el alta estaba
	// bien formada, la tabla ya la tiene, y eso es un 409 y no un 500. Dos
	// filas para el mismo canal partirian su recaudo en dos y cada mitad se
	// repartiria como si fuera el total de un usuario distinto.
	ErrUsuarioDeRecaudoDuplicado = errors.New("ya existe un usuario de recaudo con ese identificador")

	// ErrCanalVacio: UsosDeCanal necesita saber contra que bolsa pondera cada
	// fila, y el canal es esa identidad (ADR 0019). Un canal vacio no es "dame
	// todo lo que no tiene canal": eso mezclaria las filas sin atribuir de
	// TODOS los pagadores en una sola corrida, un valor punto que el
	// reglamento no reconoce. Ver UsosSinCanal para detectar ese hueco.
	ErrCanalVacio = errors.New("el canal no puede quedar vacio")

	// ErrUsoSinObra: un uso sin obra identificada (`obra_id` NULL: pendiente,
	// ONI o excluido) nunca puede llegar a [reparto.Reparto]. Sin este
	// guardian, COALESCE(obra_id, '') convierte las tres en una obra fantasma
	// de id "" que suma puntos e importe de verdad y que ningun `resultados_obra`
	// puede persistir (`obra_id NOT NULL REFERENCES obras(id)`). Es defensa en
	// profundidad: el filtro real vive en el SQL de UsosDeCanal, esto es lo
	// que impide que un adaptador futuro que lo olvide pase desapercibido.
	ErrUsoSinObra = errors.New("el uso no tiene obra identificada")

	// ErrReservaYaRegistrada: ya existe una reserva para ese proceso.
	//
	// CrearReserva es de una sola vez por corrida: un segundo alta con otra
	// tasa u otro monto no puede pisar la fila en silencio -- una reserva
	// registrada dos veces con valores distintos es exactamente el tipo de
	// discrepancia que una auditoria de RD 16 encuentra y que nadie puede
	// explicar despues.
	ErrReservaYaRegistrada = errors.New("ya existe una reserva registrada para ese proceso")
)

// ErrorParametroAusente nombra las clausulas normativas que no tienen valor
// vigente en la fecha pedida.
//
// Es un tipo y no solo el centinela porque el ADR 0004 pide que un reparto que
// no encuentre un parametro "falle ruidosamente en vez de producir una cifra
// falsa", y ruidosamente quiere decir diciendo CUAL falta. Quien recibe el
// fallo -- distribucion, no un programador -- tiene que poder cargar la fila
// que falta, y para eso necesita su clave, no un "parametro normativo
// ausente" que no se puede accionar.
//
// errors.Is lo sigue reconociendo como [ErrParametroAusente], que es lo que ya
// distingue el resto del sistema; errors.As da las claves.
//
// Lleva la lista ENTERA y no la primera que falte: si faltan cinco, enterarse
// de una por intento son cinco viajes para la misma carga de datos.
type ErrorParametroAusente struct {
	// Fecha es el dia contra el que se resolvio, ya reducido a fecha en UTC.
	// Va en el mensaje porque la misma clave puede estar y no estar segun el
	// dia: un parametro "ausente" suele ser una vigencia que empieza mas
	// tarde, no una fila que nadie cargo.
	Fecha time.Time

	// Claves son las clausulas sin valor, ordenadas.
	Claves []string
}

func (e *ErrorParametroAusente) Error() string {
	return fmt.Sprintf("%s en %s: %s",
		ErrParametroAusente, e.Fecha.UTC().Format(time.DateOnly), strings.Join(e.Claves, ", "))
}

// Unwrap deja que quien solo quiera saber "falta un parametro" siga usando
// errors.Is(err, ErrParametroAusente) sin conocer este tipo.
func (e *ErrorParametroAusente) Unwrap() error { return ErrParametroAusente }

// ErrorTasaAmbigua nombra el codigo ISO y las dos claves de `parametros` que
// compiten por el.
//
// Es un tipo y no solo el centinela por la misma razon que ErrorParametroAusente:
// quien lo recibe tiene que poder actuar, y "tasa de cambio ambigua" a secas no
// dice cual de las dos filas hay que cerrar o corregir.
type ErrorTasaAmbigua struct {
	// Codigo es el ISO ya normalizado a mayusculas, p.ej. "USD".
	Codigo string
	// Claves son las dos claves originales que colisionan.
	Claves []string
}

func (e *ErrorTasaAmbigua) Error() string {
	return fmt.Sprintf("%s %s: %s", ErrTasaAmbigua, e.Codigo, strings.Join(e.Claves, ", "))
}

// Unwrap deja que quien solo quiera saber "hay una tasa ambigua" siga usando
// errors.Is(err, ErrTasaAmbigua) sin conocer este tipo.
func (e *ErrorTasaAmbigua) Unwrap() error { return ErrTasaAmbigua }

// exigirActor rechaza un actor vacio en los casos de uso que asientan un hecho
// FIRMADO por una persona. operacion es lo que se estaba haciendo, para que el
// mensaje diga que se quedo sin hacer y no solo que faltaba un campo.
//
// Vive en esta capa y no en el adaptador a proposito. El adaptador convierte
// el actor vacio en NULL -- con un NULLIF sobre la cadena vacia, ver
// bitacora.go -- sobre una columna nullable porque el ADR 0006 pide el actor
// para la DECISION MANUAL -- "y en ese ultimo caso quien la tomo y cuando" --
// y no para un hecho que el sistema produzca solo. Meter la guarda en
// [postgres.asentar] cerraria de paso esa puerta, que hoy no tiene usuario
// pero es la prevista para el calculo de una corrida o una identificacion
// automatica. Lo que hay que cerrar es lo otro: un caso de uso que SI recibe
// un actor de la sesion y lo pierde por el camino.
//
// Se recorta antes de comparar: un actor de solo espacios no lo atrapa el
// NULLIF -- solo casa con la cadena vacia --, y llega hasta la clave foranea
// contra `usuarios`, que devuelve un 500 generico en vez de decir que falta la
// firma.
func exigirActor(actorID, operacion string) error {
	if strings.TrimSpace(actorID) == "" {
		return fmt.Errorf("%s: %w", operacion, ErrActorAusente)
	}
	return nil
}
