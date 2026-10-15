package aplicacion

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"
)

// Limites de la busqueda en el reglamento.
const (
	TopKReglamento              = 3
	MaxRunasPreguntaReglamento  = 1000
	TextoReglamentoNoEncontrado = "No encontré ninguna sección del reglamento que responda a esto con suficiente similitud. No adivines: dilo así o reformula la pregunta."
)

// SeccionReglamento es un numeral citable (RD 9.1.1) con su texto verbatim.
type SeccionReglamento struct {
	Cita       string
	Reglamento string
	Titulo     string
	Texto      string
}

// Embedding es un vector junto al modelo que lo produjo: vectores de modelos distintos no se comparan.
type Embedding struct {
	Modelo string
	Vector []float32
}

// MotorEmbeddings convierte texto en vector; neutral al proveedor.
type MotorEmbeddings interface {
	Embeber(ctx context.Context, texto string) (Embedding, error)
}

// SeccionIndexada es una seccion con su vector, lista para guardar.
type SeccionIndexada struct {
	SeccionReglamento
	Vector []float32
}

// CoincidenciaReglamento es una seccion recuperada con su similitud coseno (1 = identica).
type CoincidenciaReglamento struct {
	Seccion   SeccionReglamento
	Similitud float64
}

// AlmacenVectorial persiste y consulta secciones por similitud; separado del motor para que la herramienta no vea la BD.
type AlmacenVectorial interface {
	// IndexarSecciones sustituye atomicamente todo el indice de un modelo: sin numerales huerfanos tras reindexar.
	IndexarSecciones(ctx context.Context, modelo string, secciones []SeccionIndexada) error
	// BuscarSecciones devuelve hasta topK secciones del mismo modelo, de mas a menos similar.
	BuscarSecciones(ctx context.Context, e Embedding, topK int) ([]CoincidenciaReglamento, error)
}

// RespuestaReglamento: o secciones con su cita por encima del piso, o Encontrado=false con Mensaje.
type RespuestaReglamento struct {
	Encontrado bool
	Secciones  []CoincidenciaReglamento
	Mensaje    string
}

// ConsultarReglamento busca por similitud con un piso: prefiere "no encontrado" a una coincidencia debil (ADR 0004).
type ConsultarReglamento struct {
	Motor   MotorEmbeddings
	Almacen AlmacenVectorial
	// Piso es la similitud coseno minima; depende del modelo de embeddings.
	Piso float64
	// Log recibe el aviso de indice vacio; nil = slog.Default().
	Log *slog.Logger
}

// Consultar embebe la pregunta y devuelve las secciones con cita que superan el piso.
func (c ConsultarReglamento) Consultar(ctx context.Context, actor Usuario, pregunta string) (RespuestaReglamento, error) {
	if !rolConocido(actor.Rol) {
		return RespuestaReglamento{}, ErrNoAutorizado
	}
	pregunta = strings.TrimSpace(pregunta)
	if pregunta == "" || utf8.RuneCountInString(pregunta) > MaxRunasPreguntaReglamento {
		return RespuestaReglamento{}, fmt.Errorf("%w: la pregunta tiene que tener entre 1 y %d caracteres", ErrArgumentosInvalidos, MaxRunasPreguntaReglamento)
	}
	e, err := c.Motor.Embeber(ctx, pregunta)
	if err != nil {
		return RespuestaReglamento{}, fmt.Errorf("embeber la pregunta: %w", err)
	}
	cs, err := c.Almacen.BuscarSecciones(ctx, e, TopKReglamento)
	if err != nil {
		return RespuestaReglamento{}, fmt.Errorf("buscar en el reglamento: %w", err)
	}
	if len(cs) == 0 {
		// Sin ninguna fila no es "nada supera el piso": nadie indexo este modelo o la API y el indexador difieren.
		c.log().WarnContext(ctx, "reglamento: indice vacio para el modelo; correr make indexar-reglamento con el mismo proveedor", slog.String("modelo", e.Modelo))
	}
	var r RespuestaReglamento
	for _, co := range cs {
		if co.Similitud >= c.Piso && co.Seccion.Cita != "" {
			r.Secciones = append(r.Secciones, co)
		}
	}
	if len(r.Secciones) == 0 {
		return RespuestaReglamento{Mensaje: TextoReglamentoNoEncontrado}, nil
	}
	r.Encontrado = true
	return r, nil
}

func (c ConsultarReglamento) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}

// rolConocido falla cerrado: el usuario cero o un rol fuera de la matriz no leen nada.
func rolConocido(r Rol) bool {
	switch r {
	case RolAdministrador, RolDistribucion, RolContabilidad, RolAuditor, RolTitular:
		return true
	default:
		return false
	}
}

// IndexarReglamento embebe cada seccion y reemplaza el indice del modelo; lo corre cmd/indexadorreglamento.
type IndexarReglamento struct {
	Motor   MotorEmbeddings
	Almacen AlmacenVectorial
}

// Indexar devuelve cuantas secciones quedaron en el indice.
func (ix IndexarReglamento) Indexar(ctx context.Context, secciones []SeccionReglamento) (int, error) {
	if len(secciones) == 0 {
		return 0, fmt.Errorf("%w: sin secciones que indexar", ErrArgumentosInvalidos)
	}
	vistas := make(map[string]bool, len(secciones))
	out := make([]SeccionIndexada, 0, len(secciones))
	modelo := ""
	for _, s := range secciones {
		if s.Cita == "" || vistas[s.Cita] {
			return 0, fmt.Errorf("%w: cita vacia o duplicada %q", ErrArgumentosInvalidos, s.Cita)
		}
		vistas[s.Cita] = true
		e, err := ix.Motor.Embeber(ctx, textoAEmbeber(s))
		if err != nil {
			return 0, fmt.Errorf("embeber %s: %w", s.Cita, err)
		}
		if modelo != "" && e.Modelo != modelo {
			return 0, fmt.Errorf("el motor cambio de modelo a mitad del indice: %q y %q", modelo, e.Modelo)
		}
		modelo = e.Modelo
		out = append(out, SeccionIndexada{SeccionReglamento: s, Vector: e.Vector})
	}
	if err := ix.Almacen.IndexarSecciones(ctx, modelo, out); err != nil {
		return 0, fmt.Errorf("guardar el indice: %w", err)
	}
	return len(out), nil
}

// textoAEmbeber antepone cita y titulo: una pregunta por "9.1.1" o por el titulo tambien debe acercarse.
func textoAEmbeber(s SeccionReglamento) string {
	return s.Cita + "\n" + s.Titulo + "\n" + s.Texto
}
