package falso

import (
	"math"
	"testing"
)

func coseno(a, b []float32) float64 {
	var p, na, nb float64
	for i := range a {
		p += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return p / math.Sqrt(na*nb)
}

func TestMotorEsDeterministaYNormalizado(t *testing.T) {
	a, err := Motor{}.Embeber(t.Context(), "Declaración de Obra")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Motor{}.Embeber(t.Context(), "declaracion de obra")
	if a.Modelo != Modelo || len(a.Vector) != Dimension {
		t.Fatalf("modelo=%q dim=%d", a.Modelo, len(a.Vector))
	}
	if c := coseno(a.Vector, b.Vector); c < 0.999 {
		t.Errorf("mayusculas y tildes cambian el vector: coseno %f", c)
	}
}

func TestMotorAcercaTextosQueCompartenPalabras(t *testing.T) {
	q, _ := Motor{}.Embeber(t.Context(), "prescripcion de obras no identificadas")
	cerca, _ := Motor{}.Embeber(t.Context(), "La prescripción del recaudo por obras no identificadas (ONI)")
	lejos, _ := Motor{}.Embeber(t.Context(), "Tarifa de salas de cine por pantalla")
	if coseno(q.Vector, cerca.Vector) <= coseno(q.Vector, lejos.Vector) {
		t.Error("el texto que comparte palabras no quedo mas cerca")
	}
}

func TestMotorTextoVacioDaVectorCeroSinPanico(t *testing.T) {
	e, err := Motor{}.Embeber(t.Context(), "  ")
	if err != nil || len(e.Vector) != Dimension {
		t.Fatalf("e=%v err=%v", e, err)
	}
}
