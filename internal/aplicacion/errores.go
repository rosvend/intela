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

	// ErrConflicto: la fila ya existe (correo o IPI duplicado en una
	// solicitud activa). Distinto de ErrNoEncontrado: aqui la consulta
	// encontro de mas, no de menos.
	ErrConflicto = errors.New("ya existe una solicitud o afiliacion con esos datos")

	// ErrDocumentoInvalido: el adjunto no es un PDF o una imagen, o viene
	// vacio. El dominio no mira bytes; esto lo decide el caso de uso antes
	// de mandarlos al almacen.
	ErrDocumentoInvalido = errors.New("el documento tiene que ser un pdf o una imagen y no puede estar vacio")
)
