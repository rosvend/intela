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

// Regresion: la suma cuadraba pero las partes eran imposibles. 150 y -50 dan
// 100, y sin validar cada parte la declaracion pasaba por "completa".
func TestPartesNegativasNoSonCompletas(t *testing.T) {
	d := Declaracion{Partes: []Parte{
		{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(150)},
		{TitularID: "b", IPI: "2", Porcentaje: decimal.NewFromInt(-50)},
	}}
	if d.Completa() {
		t.Fatal("150 y -50 suman 100 pero ninguna declaracion valida tiene una parte negativa")
	}
	if got := d.Estado(); got != "incompleta" {
		t.Fatalf("Estado() = %q, se esperaba \"incompleta\"", got)
	}

	// Una parte en cero tampoco: un titular al 0% no es titular.
	d.Partes = []Parte{
		{TitularID: "a", IPI: "1", Porcentaje: decimal.NewFromInt(100)},
		{TitularID: "b", IPI: "2", Porcentaje: decimal.Zero},
	}
	if d.Completa() {
		t.Fatal("una parte al 0% no puede contar como declarada")
	}
}
