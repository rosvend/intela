package identificacion

import (
	"testing"

	"github.com/shopspring/decimal"
)

func cand(obra, puntaje string) Candidato {
	return Candidato{ObraID: obra, Puntaje: decimal.RequireFromString(puntaje)}
}

func TestClaveDeTitulo(t *testing.T) {
	casos := []struct {
		titulo, original, quiere string
	}{
		{"La Casa", "", "la casa"},
		{"  LA   CASA ", "", "la casa"},
		{"Lá Casa", "", "la casa"},
		{"la casa!", "", "la casa"},
		{"", "La Casa de Papel", "la casa de papel"},
		{"", "", ""},
		{"Niño", "", "nino"},
	}
	for _, c := range casos {
		t.Run(c.quiere, func(t *testing.T) {
			if got := ClaveDeTitulo(c.titulo, c.original); got != c.quiere {
				t.Fatalf("ClaveDeTitulo(%q, %q) = %q, quiere %q", c.titulo, c.original, got, c.quiere)
			}
		})
	}
}

func TestRankear(t *testing.T) {
	casa := ClaveDeTitulo("La Casa", "")
	casos := []struct {
		nombre   string
		pedido   PedidoTriage
		decision string
		obra     string
		primera  string
	}{
		{
			nombre: "sin historia gana la similitud",
			pedido: PedidoTriage{
				Clave:      casa,
				Candidatos: []Candidato{cand("obra-baja", "0.62"), cand("obra-alta", "0.80")},
			},
			decision: string(DecisionAsignar),
			obra:     "obra-alta",
			primera:  "obra-alta",
		},
		{
			nombre: "la historia del mismo titulo levanta a la obra que las personas eligieron",
			pedido: PedidoTriage{
				Clave:      casa,
				Candidatos: []Candidato{cand("obra-alta", "0.80"), cand("obra-baja", "0.62")},
				Historia: []EjemploEtiquetado{
					{Clave: casa, Decision: DecisionAsignar, ObraID: "obra-baja"},
				},
			},
			decision: string(DecisionAsignar),
			obra:     "obra-baja",
			primera:  "obra-baja",
		},
		{
			nombre: "otro titulo no cuenta",
			pedido: PedidoTriage{
				Clave:      casa,
				Candidatos: []Candidato{cand("obra-alta", "0.80"), cand("obra-baja", "0.62")},
				Historia: []EjemploEtiquetado{
					{Clave: "otro", Decision: DecisionAsignar, ObraID: "obra-baja"},
				},
			},
			decision: string(DecisionAsignar),
			obra:     "obra-alta",
			primera:  "obra-alta",
		},
		{
			nombre: "mayoria de descartes sugiere descartar",
			pedido: PedidoTriage{
				Clave:      casa,
				Candidatos: []Candidato{cand("obra-alta", "0.90")},
				Historia: []EjemploEtiquetado{
					{Clave: casa, Decision: DecisionDescartar},
					{Clave: casa, Decision: DecisionDescartar},
				},
			},
			decision: string(DecisionDescartar),
			primera:  "obra-alta",
		},
		{
			nombre: "un solo descarte no alcanza",
			pedido: PedidoTriage{
				Clave:      casa,
				Candidatos: []Candidato{cand("obra-alta", "0.80")},
				Historia: []EjemploEtiquetado{
					{Clave: casa, Decision: DecisionDescartar},
				},
			},
			decision: string(DecisionAsignar),
			obra:     "obra-alta",
		},
		{
			nombre:   "sin candidatas ni historia no hay propuesta",
			pedido:   PedidoTriage{Clave: casa},
			decision: SugerenciaNinguna,
		},
		{
			nombre: "clave vacia no agrupa historial",
			pedido: PedidoTriage{
				Candidatos: []Candidato{cand("obra-alta", "0.50"), cand("obra-baja", "0.40")},
				Historia: []EjemploEtiquetado{
					{Clave: "", Decision: DecisionAsignar, ObraID: "obra-baja"},
					{Clave: "", Decision: DecisionAsignar, ObraID: "obra-baja"},
				},
			},
			decision: string(DecisionAsignar),
			obra:     "obra-alta",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			sug := Rankear(c.pedido)
			if sug.Decision != c.decision || sug.ObraID != c.obra {
				t.Fatalf("sugerencia = %s %s, quiere %s %s (%s)", sug.Decision, sug.ObraID, c.decision, c.obra, sug.Motivo)
			}
			if c.primera != "" && (len(sug.Orden) == 0 || sug.Orden[0] != c.primera) {
				t.Fatalf("orden = %v, quiere %s primero", sug.Orden, c.primera)
			}
			if sug.Orden == nil {
				t.Fatal("orden nil: tiene que ser una lista, aunque vacia")
			}
			if sug.Confianza.IsNegative() || sug.Confianza.GreaterThan(decimal.NewFromInt(1)) {
				t.Fatalf("confianza fuera de [0, 1]: %s", sug.Confianza)
			}
		})
	}
}

func TestSugerenciaAceptadaPor(t *testing.T) {
	asignar := Sugerencia{Decision: string(DecisionAsignar), ObraID: "obra-1"}
	if !asignar.AceptadaPor(DecisionAsignar, "obra-1") {
		t.Fatal("confirmar la misma obra tenia que contar como aceptada")
	}
	if asignar.AceptadaPor(DecisionAsignar, "obra-2") {
		t.Fatal("elegir otra obra no es aceptar la sugerencia")
	}
	if asignar.AceptadaPor(DecisionDescartar, "") {
		t.Fatal("descartar no es aceptar una asignacion")
	}
	descartar := Sugerencia{Decision: string(DecisionDescartar)}
	if !descartar.AceptadaPor(DecisionDescartar, "") {
		t.Fatal("descartar cuando se sugirio descartar es aceptar")
	}
	ninguna := Sugerencia{Decision: SugerenciaNinguna}
	if ninguna.AceptadaPor(DecisionAsignar, "obra-1") || ninguna.AceptadaPor(DecisionDescartar, "") {
		t.Fatal("sin propuesta no hay nada que aceptar")
	}
}

// El conjunto reservado es el criterio de la issue: alimentar el rankeador con
// resoluciones anteriores mejora el acierto sobre casos que no vio, frente a
// ordenar solo por similitud.
func TestRankearLaHistoriaMejoraElConjuntoReservado(t *testing.T) {
	casa := ClaveDeTitulo("La Casa", "")
	noticiero := ClaveDeTitulo("Noticiero central", "")

	type caso struct {
		clave      string
		candidatos []Candidato
		decision   Decision
		obra       string
	}
	// Cuatro resoluciones de entrenamiento y una reservada, por titulo.
	// En "la casa" las personas eligieron la obra de MENOR similitud.
	// En "noticiero central" las descartaron todas.
	casaCaso := caso{
		clave:      casa,
		candidatos: []Candidato{cand("obra-alta", "0.80"), cand("obra-baja", "0.62")},
		decision:   DecisionAsignar,
		obra:       "obra-baja",
	}
	noticieroCaso := caso{
		clave:      noticiero,
		candidatos: []Candidato{cand("obra-x", "0.90")},
		decision:   DecisionDescartar,
	}

	var entrenamiento []caso
	for range 4 {
		entrenamiento = append(entrenamiento, casaCaso, noticieroCaso)
	}
	reservados := []caso{casaCaso, noticieroCaso}

	historia := make([]EjemploEtiquetado, 0, len(entrenamiento))
	for _, c := range entrenamiento {
		historia = append(historia, EjemploEtiquetado{Clave: c.clave, Decision: c.decision, ObraID: c.obra})
	}

	aciertos := func(historia []EjemploEtiquetado) int {
		t.Helper()
		n := 0
		for _, c := range reservados {
			sug := Rankear(PedidoTriage{Clave: c.clave, Candidatos: c.candidatos, Historia: historia})
			if sug.AceptadaPor(c.decision, c.obra) {
				n++
			}
		}
		return n
	}

	sinHistoria := aciertos(nil)
	conHistoria := aciertos(historia)
	if conHistoria <= sinHistoria {
		t.Fatalf("aciertos con historia = %d, sin historia = %d: la historia no mejoro el conjunto reservado", conHistoria, sinHistoria)
	}
	if conHistoria != len(reservados) {
		t.Fatalf("con la historia acerto %d de %d", conHistoria, len(reservados))
	}
	if sinHistoria != 0 {
		t.Fatalf("sin historia acerto %d: el conjunto esta armado para que la similitud sola falle", sinHistoria)
	}
}
