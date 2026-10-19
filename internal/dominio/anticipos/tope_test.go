package anticipos

import (
	"testing"
)

// TestTope es R-31: el menor de los dos topes, 25% de los derechos de los
// repartos ordinarios de los ultimos dos anos (RA 3.1.g) y 5 SMMLV (RA 3.1.h).
func TestTope(t *testing.T) {
	casos := []struct {
		nombre, derechos, smmlv, quiero string
	}{
		{"sin derechos no hay cupo", "0", smmlvPrueba, "0"},
		{"manda el 25 por ciento cuando es menor", "1000000", smmlvPrueba, "250000"},
		{"manda 5 smmlv cuando es menor", "40000000", smmlvPrueba, "6500000"},
		{"los dos topes empatan", "26000000", smmlvPrueba, "6500000"},
		// 100.03 * 0.25 = 25.0075. Redondear a la mitad daria 25.01, un
		// centavo por encima del 25% que el reglamento fija como techo.
		{"el 25 por ciento se trunca al centavo", "100.03", smmlvPrueba, "25.00"},
		{"unos derechos negativos no dan cupo negativo", "-500", smmlvPrueba, "0"},
		{"el smmlv del ano decide el tope fijo", "40000000", "1423500", "7117500"},
		// 1300000.001 * 5 = 6500000.005: medio centavo que redondeado subiria
		// el techo de RA 3.1.h a 6500000.01.
		{"5 smmlv tambien se truncan al centavo", "40000000", "1300000.001", "6500000.00"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := Tope(dec(c.derechos), dec(c.smmlv))
			if !got.Equal(dec(c.quiero)) {
				t.Fatalf("Tope(%s, %s) = %s, se esperaba %s", c.derechos, c.smmlv, got, c.quiero)
			}
		})
	}
}
