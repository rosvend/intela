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
)
