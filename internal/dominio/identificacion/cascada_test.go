package identificacion

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCascadaAliasGana(t *testing.T) {
	r := Cascada(Entrada{Fuente: "caracol", TipoID: "id_ficha", ValorID: "123"}, "obra-1", "", nil, decimal.NewFromFloat(0.92))
	if r.ONI || r.ObraID != "obra-1" || r.Escalon != "alias" {
		t.Fatalf("%+v", r)
	}
}

func TestCascadaONISiDifusoBajoUmbral(t *testing.T) {
	r := Cascada(Entrada{Titulo: "x"}, "", "", []Candidato{{ObraID: "o", Puntaje: decimal.NewFromFloat(0.4)}}, decimal.NewFromFloat(0.92))
	if !r.ONI {
		t.Fatalf("esperado ONI, %+v", r)
	}
}

func TestSimilitudExacta(t *testing.T) {
	s := SimilitudTitulo("El Patrón del Mal", "el patron del mal")
	if !s.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("similitud %s", s)
	}
}
