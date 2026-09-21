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

func TestAcrecerRendimientoRechazaMezclarCircuitos(t *testing.T) {
	pool, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "2026", d("1000.00"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	_, err = reparto.AcrecerRendimiento(pool, reparto.Internacional, d("50.00"))
	if !errors.Is(err, reparto.ErrRendimientoCircuitoMezclado) {
		t.Fatalf("error = %v, se esperaba ErrRendimientoCircuitoMezclado (RD 10.3)", err)
	}
}

func TestAcrecerRendimientoSumaMismoCircuito(t *testing.T) {
	pool, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "2026", d("1000.00"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	pool, err = reparto.AcrecerRendimiento(pool, reparto.Nacional, d("50.00"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !pool.Monto.Equal(d("1050.00")) {
		t.Fatalf("monto = %s, se esperaba 1050.00", pool.Monto)
	}
}
