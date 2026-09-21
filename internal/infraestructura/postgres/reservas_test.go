package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestReservasGuardarYReservaPorProcesoRedondaLaFilaCompleta(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}

	if err := s.GuardarReserva(ctx, pool); err != nil {
		t.Fatalf("guardar reserva: %v", err)
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if leido.ProcesoID != pool.ProcesoID || leido.Circuito != pool.Circuito ||
		!leido.MontoInicial.Equal(pool.MontoInicial) || !leido.Saldo.Equal(pool.Saldo) ||
		!leido.TasaPct.Equal(pool.TasaPct) || leido.OrganoAprobador != pool.OrganoAprobador {
		t.Fatalf("leido = %+v, se esperaba %+v", leido, pool)
	}
}

func TestReservasGuardarActualizaElSaldoExistente(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.GuardarReserva(ctx, pool); err != nil {
		t.Fatalf("guardar reserva: %v", err)
	}

	pool.Saldo = dec("0.00")
	if err := s.GuardarReserva(ctx, pool); err != nil {
		t.Fatalf("actualizar reserva: %v", err)
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if !leido.Saldo.IsZero() {
		t.Fatalf("saldo = %s, se esperaba cero tras la actualizacion", leido.Saldo)
	}
}

func TestReservaPorProcesoSinFilaEsNoEncontrado(t *testing.T) {
	s := sembrarCorridaBase(t)
	_, err := s.ReservaPorProceso(t.Context(), "proceso-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}
