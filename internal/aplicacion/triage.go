package aplicacion

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// PuertoRankeadorDeResoluciones propone un orden y una accion para un caso de
// la cola. Es el hermano de PuertoMotorDeSimilitud: el caso de uso no sabe si
// por detras hay una heuristica o, cuando haya datos, un modelo (#53, ADR 0007).
//
// Rankear no resuelve. No hay metodo que escriba el escalon.
type PuertoRankeadorDeResoluciones interface {
	Rankear(pedido identificacion.PedidoTriage) identificacion.Sugerencia
}

// RepositorioEjemplosResolucion guarda cada resolucion manual como ejemplo
// etiquetado y la vuelve a leer para rankear y para decir si la sugerencia se
// acepto.
type RepositorioEjemplosResolucion interface {
	HistoriaPorClaves(ctx context.Context, claves []string) ([]identificacion.EjemploEtiquetado, error)
	GuardarEjemplo(ctx context.Context, e EjemploResolucion) error
	EjemplosDe(ctx context.Context, usoIDs []string) (map[string]EjemploGuardado, error)
}

// EjemploResolucion es la escritura: las features de la entrada, la obra que
// eligio la persona (o el descarte) y si eso coincidio con lo sugerido.
type EjemploResolucion struct {
	UsoID              string
	Clave              string
	Fuente             string
	Candidatos         []identificacion.Candidato
	Decision           identificacion.Decision
	ObraElegida        string
	SugerenciaDecision string
	SugerenciaObraID   string
	Confianza          decimal.Decimal
	Motivo             string
	Orden              []string
	Aceptada           bool
	ActorID            string
}

// EjemploGuardado es la sugerencia que se mostro cuando una persona resolvio
// el caso, tal como quedo guardada. No se recalcula: un historial posterior
// no reescribe lo que esa persona vio.
type EjemploGuardado struct {
	SugerenciaDecision string
	SugerenciaObraID   string
	Titulo             string
	Confianza          decimal.Decimal
	Motivo             string
	Orden              []string
	Aceptada           bool
}

// SugerenciaCaso es lo que la bandeja muestra. Aceptada es nil mientras el
// caso sigue pendiente: todavia no hay con que medirla.
type SugerenciaCaso struct {
	Decision  string
	ObraID    string
	Titulo    string
	Confianza decimal.Decimal
	Motivo    string
	Orden     []string
	Aceptada  *bool
}

func sugerenciaNinguna() *SugerenciaCaso {
	return &SugerenciaCaso{
		Decision:  identificacion.SugerenciaNinguna,
		Confianza: decimal.Zero,
		Orden:     []string{},
	}
}

func sugerenciaVisible(s identificacion.Sugerencia, candidatos []CandidatoCaso, aceptada *bool) *SugerenciaCaso {
	out := &SugerenciaCaso{
		Decision:  s.Decision,
		ObraID:    s.ObraID,
		Confianza: s.Confianza,
		Motivo:    s.Motivo,
		Orden:     s.Orden,
		Aceptada:  aceptada,
	}
	if out.Decision == "" {
		out.Decision = identificacion.SugerenciaNinguna
	}
	if out.Orden == nil {
		out.Orden = []string{}
	}
	if s.ObraID != "" {
		out.Titulo = tituloDeCandidato(candidatos, s.ObraID)
	}
	return out
}

func tituloDeCandidato(candidatos []CandidatoCaso, obraID string) string {
	for _, c := range candidatos {
		if c.ObraID == obraID {
			return c.Titulo
		}
	}
	return ""
}

func candidatosDeRanking(cs []CandidatoCaso) []identificacion.Candidato {
	out := make([]identificacion.Candidato, 0, len(cs))
	for _, c := range cs {
		out = append(out, identificacion.Candidato{
			ObraID: c.ObraID, Puntaje: c.Puntaje, TituloConsultado: c.TituloConsultado,
		})
	}
	return out
}
