package aplicacion

import "errors"

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
	ErrParametroAusente = errors.New("parametro normativo ausente")

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
)
