package identificacion

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// candidatosDePrueba es la banda ambigua tipica: dos obras propuestas.
func candidatosDePrueba() []Candidato {
	return []Candidato{
		{ObraID: "obra-12", Puntaje: punt("0.52941"), TituloConsultado: "titulo emitido"},
		{ObraID: "obra-40", Puntaje: punt("0.41000"), TituloConsultado: "titulo original"},
	}
}

func TestResolverCaso(t *testing.T) {
	casos := []struct {
		nombre     string
		escalon    string
		decision   Decision
		obraID     string
		candidatos []Candidato
		quiero     Resultado
	}{
		{
			// La candidata conserva SU puntaje: es lo que deja medir despues si
			// la persona confirmo al motor o lo corrigio.
			nombre:   "asignar a una candidata deja manual con el puntaje del candidato",
			escalon:  EscalonONI,
			decision: DecisionAsignar,
			obraID:   "obra-12",
			quiero: Resultado{
				ObraID: "obra-12", Escalon: EscalonManual, Puntaje: punt("0.52941"), ONI: false,
				Evidencia: "manual: candidata obra-12 (0.52941) de 2 propuestas",
			},
		},
		{
			// Una obra que el motor no propuso no tiene puntaje que heredar.
			nombre:   "asignar a una obra buscada en el catalogo deja puntaje cero",
			escalon:  EscalonONI,
			decision: DecisionAsignar,
			obraID:   "obra-99",
			quiero: Resultado{
				ObraID: "obra-99", Escalon: EscalonManual, Puntaje: decimal.Zero, ONI: false,
				Evidencia: "manual: obra obra-99 buscada en el catalogo, fuera de las 2 candidatas",
			},
		},
		{
			nombre:   "descartar deja descartado, sin obra y sin ONI",
			escalon:  EscalonONI,
			decision: DecisionDescartar,
			obraID:   "",
			quiero: Resultado{
				Escalon: EscalonDescartado, Puntaje: decimal.Zero, ONI: false,
				Evidencia: "descartado a mano: no es un uso del repertorio",
			},
		},
		{
			// Sin candidatos, la evidencia lo dice con un cero explicito.
			nombre:     "asignar sin candidatos no inventa puntaje",
			escalon:    EscalonONI,
			decision:   DecisionAsignar,
			obraID:     "obra-99",
			candidatos: []Candidato{},
			quiero: Resultado{
				ObraID: "obra-99", Escalon: EscalonManual, Puntaje: decimal.Zero, ONI: false,
				Evidencia: "manual: obra obra-99 buscada en el catalogo, fuera de las 0 candidatas",
			},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			candidatos := c.candidatos
			if candidatos == nil {
				candidatos = candidatosDePrueba()
			}
			got, err := ResolverCaso(c.escalon, c.decision, c.obraID, candidatos)
			if err != nil {
				t.Fatalf("ResolverCaso: %v", err)
			}
			if got.ObraID != c.quiero.ObraID || got.Escalon != c.quiero.Escalon ||
				got.ONI != c.quiero.ONI || got.Evidencia != c.quiero.Evidencia {
				t.Fatalf("Resultado = %+v, se esperaba %+v", got, c.quiero)
			}
			if !got.Puntaje.Equal(c.quiero.Puntaje) {
				t.Fatalf("puntaje = %s, se esperaba %s", got.Puntaje, c.quiero.Puntaje)
			}
		})
	}
}

// Determinismo (ADR 0005): la misma entrada da el mismo Resultado, incluida la
// evidencia, que es lo que se guarda en la fila y viaja al asiento.
func TestResolverCasoEsDeterminista(t *testing.T) {
	primero, err := ResolverCaso(EscalonONI, DecisionAsignar, "obra-12", candidatosDePrueba())
	if err != nil {
		t.Fatalf("ResolverCaso: %v", err)
	}
	segundo, err := ResolverCaso(EscalonONI, DecisionAsignar, "obra-12", candidatosDePrueba())
	if err != nil {
		t.Fatalf("ResolverCaso: %v", err)
	}
	if primero.Evidencia != segundo.Evidencia || primero.Escalon != segundo.Escalon {
		t.Fatalf("dos llamadas iguales dieron %+v y %+v", primero, segundo)
	}
}

// Los escalones que NO son un caso pendiente, cada uno con su centinela y su
// mensaje: "ya lo resolvio alguien" y "esto nunca fue un caso" no son lo mismo,
// y el front los pinta distinto (409 en los dos, mensaje distinto).
func TestResolverCasoRechazaEscalonesNoResolubles(t *testing.T) {
	casos := []struct {
		nombre  string
		escalon string
		quiero  error
	}{
		{"manual", EscalonManual, ErrCasoYaResuelto},
		{"descartado", EscalonDescartado, ErrCasoYaResuelto},
		{"pendiente", EscalonPendiente, ErrCasoNoPendiente},
		{"alias", EscalonAlias, ErrCasoNoPendiente},
		{"id_global", EscalonIDGlobal, ErrCasoNoPendiente},
		{"difuso", EscalonDifuso, ErrCasoNoPendiente},
		{"excluido", EscalonExcluido, ErrCasoNoPendiente},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			res, err := ResolverCaso(c.escalon, DecisionAsignar, "obra-12", candidatosDePrueba())
			if !errors.Is(err, c.quiero) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiero)
			}
			if !strings.Contains(err.Error(), c.escalon) {
				t.Errorf("el error no nombra el escalon actual: %v", err)
			}
			if res.Escalon != "" || res.ObraID != "" || res.ONI {
				t.Errorf("un escalon no resoluble no deja Resultado: %+v", res)
			}
		})
	}
}

func TestValidarDecision(t *testing.T) {
	casos := []struct {
		nombre      string
		decision    Decision
		obraID      string
		quiereError bool
	}{
		{"asignar con obra", DecisionAsignar, "obra-12", false},
		{"asignar con obra y espacios alrededor", DecisionAsignar, "  obra-12  ", false},
		{"asignar sin obra", DecisionAsignar, "", true},
		{"asignar con obra de solo espacios", DecisionAsignar, "   ", true},
		{"descartar sin obra", DecisionDescartar, "", false},
		{"descartar con obra de solo espacios", DecisionDescartar, "  ", false},
		{"descartar con obra", DecisionDescartar, "obra-12", true},
		{"decision vacia", Decision(""), "", true},
		{"decision desconocida", Decision("reasignar"), "obra-12", true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarDecision(c.decision, c.obraID)
			if c.quiereError && !errors.Is(err, ErrDecisionInvalida) {
				t.Fatalf("error = %v, se esperaba ErrDecisionInvalida", err)
			}
			if !c.quiereError && err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
		})
	}
}

func TestNormalizarNota(t *testing.T) {
	casos := []struct {
		nombre string
		nota   string
		quiero string
		err    error
	}{
		{"vacia", "", "", ErrNotaVacia},
		{"solo espacios", "   ", "", ErrNotaVacia},
		{"solo saltos de linea y tabuladores", "\n\t  \n", "", ErrNotaVacia},
		{"recorta los extremos", "  coincide la ficha  ", "coincide la ficha", nil},
		{"conserva los espacios de dentro", "a  b", "a  b", nil},
		{"300 runas multibyte pasan", strings.Repeat("ñ", 300), strings.Repeat("ñ", 300), nil},
		{"301 runas multibyte no pasan", strings.Repeat("ñ", 301), "", ErrNotaDemasiadoLarga},
		{"300 caracteres justos pasan", strings.Repeat("a", 300), strings.Repeat("a", 300), nil},
		{"301 caracteres no pasan", strings.Repeat("a", 301), "", ErrNotaDemasiadoLarga},
		// El tope se mide sobre el texto RECORTADO: si no, 300 caracteres con
		// espacios alrededor se rechazarian por un texto que no se guarda.
		{"el tope se mide tras recortar", "  " + strings.Repeat("a", 300) + "  ", strings.Repeat("a", 300), nil},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := NormalizarNota(c.nota)
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("error = %v, se esperaba %v", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if got != c.quiero {
				t.Fatalf("nota = %q, se esperaba %q", got, c.quiero)
			}
		})
	}
}
