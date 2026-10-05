package herramientas

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/rosvend/intela/internal/aplicacion"
)

// MaxRunasTextoSeccion acota cada seccion devuelta al modelo; la cita lleva al texto completo.
const MaxRunasTextoSeccion = 4000

// LectorReglamento es lo unico que la herramienta necesita de aplicacion.ConsultarReglamento.
type LectorReglamento interface {
	Consultar(ctx context.Context, actor aplicacion.Usuario, pregunta string) (aplicacion.RespuestaReglamento, error)
}

type resultadoReglamento struct {
	Encontrado bool               `json:"encontrado"`
	Mensaje    string             `json:"mensaje,omitempty"`
	Secciones  []seccionResultado `json:"secciones,omitempty"`
}

type seccionResultado struct {
	Cita       string  `json:"cita"`
	Reglamento string  `json:"reglamento"`
	Titulo     string  `json:"titulo"`
	Texto      string  `json:"texto"`
	Similitud  float64 `json:"similitud"`
}

// BuscarReglamento envuelve ConsultarReglamento: texto verbatim y su cita (RD 9.1.1), nunca texto sin cita.
func BuscarReglamento(uc LectorReglamento) aplicacion.Herramienta {
	return aplicacion.Herramienta{
		Nombre: "buscar_reglamento",
		Descripcion: "Busca por significado en los reglamentos de REDES SGC (Distribución RD, Tarifas RT, Socios RS, Anticipos RA) " +
			"y devuelve el texto verbatim de las secciones más parecidas con su cita (p. ej. RD 9.1.1). " +
			"Cita siempre el numeral que devuelve. Si encontrado es false, di que no lo encontraste en el reglamento; no lo completes de memoria.",
		Esquema: json.RawMessage(`{"type":"object","properties":{"pregunta":{"type":"string","description":"La duda en lenguaje natural, p. ej. 'que pasa si los porcentajes declarados no suman 100%'"}},"required":["pregunta"],"additionalProperties":false}`),
		Ejecutar: func(ctx context.Context, actor aplicacion.Usuario, args json.RawMessage) (any, error) {
			var a struct {
				Pregunta string `json:"pregunta"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, fmt.Errorf("%w: %v", aplicacion.ErrArgumentosInvalidos, err)
			}
			r, err := uc.Consultar(ctx, actor, a.Pregunta)
			if err != nil {
				return nil, err
			}
			out := resultadoReglamento{Encontrado: r.Encontrado, Mensaje: r.Mensaje}
			for _, c := range r.Secciones {
				out.Secciones = append(out.Secciones, seccionResultado{
					Cita:       c.Seccion.Cita,
					Reglamento: c.Seccion.Reglamento,
					Titulo:     c.Seccion.Titulo,
					Texto:      recortar(c.Seccion.Texto, MaxRunasTextoSeccion),
					Similitud:  math.Round(c.Similitud*1000) / 1000,
				})
			}
			return out, nil
		},
	}
}

func recortar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
