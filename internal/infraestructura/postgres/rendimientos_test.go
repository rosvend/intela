package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

func TestRendimientosGuardarYPorCircuitoYVigenciaRedondaLaFila(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	pool, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "2026", dec("100.00"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.GuardarRendimiento(ctx, pool); err != nil {
		t.Fatalf("guardar rendimiento: %v", err)
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if leido.Circuito != pool.Circuito || leido.Vigencia != pool.Vigencia || !leido.Monto.Equal(pool.Monto) {
		t.Fatalf("leido = %+v, se esperaba %+v", leido, pool)
	}
}

func TestRendimientosGuardarActualizaElMontoExistente(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	pool, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "2026", dec("100.00"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.GuardarRendimiento(ctx, pool); err != nil {
		t.Fatalf("guardar rendimiento: %v", err)
	}

	pool, err = reparto.AcrecerRendimiento(pool, reparto.Nacional, dec("25.00"))
	if err != nil {
		t.Fatalf("acrecer: %v", err)
	}
	if err := s.GuardarRendimiento(ctx, pool); err != nil {
		t.Fatalf("actualizar rendimiento: %v", err)
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !leido.Monto.Equal(dec("125.00")) {
		t.Fatalf("monto = %s, se esperaba 125.00", leido.Monto)
	}
}

func TestRendimientosSegregaCircuitosEnFilasDistintas(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	nal, err := reparto.NuevoPoolRendimiento(reparto.Nacional, "2026", dec("100.00"))
	if err != nil {
		t.Fatalf("construir pool nacional: %v", err)
	}
	intl, err := reparto.NuevoPoolRendimiento(reparto.Internacional, "2026", dec("40.00"))
	if err != nil {
		t.Fatalf("construir pool internacional: %v", err)
	}
	if err := s.GuardarRendimiento(ctx, nal); err != nil {
		t.Fatalf("guardar nacional: %v", err)
	}
	if err := s.GuardarRendimiento(ctx, intl); err != nil {
		t.Fatalf("guardar internacional: %v", err)
	}

	leidoNal, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer nacional: %v", err)
	}
	leidoIntl, err := s.PorCircuitoYVigencia(ctx, reparto.Internacional, "2026")
	if err != nil {
		t.Fatalf("leer internacional: %v", err)
	}
	if !leidoNal.Monto.Equal(dec("100.00")) || !leidoIntl.Monto.Equal(dec("40.00")) {
		t.Fatalf("los ledgers se mezclaron: nacional=%s internacional=%s", leidoNal.Monto, leidoIntl.Monto)
	}
}

func TestPorCircuitoYVigenciaSinFilaEsNoEncontrado(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	_, err := s.PorCircuitoYVigencia(t.Context(), reparto.Nacional, "2099")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}
