package repertorio

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCompletaSoloConCienEIPI(t *testing.T) {
	d := Declaracion{Partes: []Parte{{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(60)}, {TitularID: "b", IPI: "2", Porcentaje: decimal.NewFromInt(40)}}}
	if !d.Completa() {
		t.Fatal("deberia ser completa")
	}
	d.Partes[1].Porcentaje = decimal.NewFromInt(30)
	if d.Completa() {
		t.Fatal("40+30 no es 100")
	}
}
