package reparto_test

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestNuevoPoolRendimientoSegregaPorCircuitoYVigencia(t *testing.T) {
	p, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "2026", d("1000.00"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Circuito != reparto.Nacional || p.Vigencia != "2026" {
		t.Fatalf("pool = %+v, se esperaba nacional/2026", p)
	}
}

func TestNuevoPoolRendimientoVigenciaVaciaEsError(t *testing.T) {
	_, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "", d("100.00"))
	if err == nil {
		t.Fatal("se esperaba error con vigencia vacia")
	}
}

func TestNuevoPoolRendimientoVigenciaConFormatoInvalidoEsError(t *testing.T) {
	for _, vigencia := range []string{"2026-01", "26", "202a", "20266"} {
		if _, err := reparto.NuevoPoolRendimiento(reparto.Nacional, vigencia, d("100.00")); err == nil {
			t.Fatalf("vigencia %q: se esperaba error, vigencia es un ano de 4 digitos (RD 10.1)", vigencia)
		}
	}
}

func TestNuevoPoolRendimientoRechazaCircuitoDesconocido(t *testing.T) {
	_, err := reparto.NuevoPoolRendimiento(reparto.Circuito("marciano"), "2026", d("100.00"))
	if err == nil {
		t.Fatal("se esperaba error con circuito desconocido")
	}
}

func TestExigirMismoCircuitoRechazaCruzarCircuitos(t *testing.T) {
	casos := []struct{ rendimiento, origen, destino reparto.Circuito }{
		{reparto.Internacional, reparto.Nacional, reparto.Internacional},
		{reparto.Nacional, reparto.Nacional, reparto.Internacional},
		{reparto.Nacional, reparto.Internacional, reparto.Nacional},
	}
	for _, c := range casos {
		if err := reparto.ExigirMismoCircuito(c.rendimiento, c.origen, c.destino); !errors.Is(err, reparto.ErrCircuitoCruzado) {
			t.Fatalf("%+v: error = %v, se esperaba ErrCircuitoCruzado (RD 10.3)", c, err)
		}
	}
	if err := reparto.ExigirMismoCircuito(reparto.Internacional, reparto.Internacional, reparto.Internacional); err != nil {
		t.Fatalf("mismo circuito: %v", err)
	}
}
