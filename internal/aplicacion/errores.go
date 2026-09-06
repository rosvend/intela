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

	// ErrObraDuplicada: ya hay una obra con ese identificador en el catalogo.
	//
	// No es "no se pudo escribir" y no es "los datos son invalidos": el alta
	// estaba bien formada y el catalogo ya la tiene. Distinguirlo es lo que
	// deja responder 409 en vez de 500, y lo que hace comprobable el criterio
	// "un segundo alta con el mismo identificador se rechaza".
	ErrObraDuplicada = errors.New("ya existe una obra con ese identificador")
)
