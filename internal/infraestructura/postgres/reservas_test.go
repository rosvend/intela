package postgres

import (
	"errors"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestReservasCrearYReservaPorProcesoRedondaLaFilaCompleta(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}

	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
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

func TestReservasCrearRechazaUnaSegundaVezParaElMismoProceso(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	pool.TasaPct = dec("4")
	err = s.CrearReserva(ctx, pool)
	if !errors.Is(err, aplicacion.ErrReservaYaRegistrada) {
		t.Fatalf("error = %v, se esperaba ErrReservaYaRegistrada (N2)", err)
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if !leido.TasaPct.Equal(dec("5")) {
		t.Fatalf("tasa_pct = %s, el segundo alta no debio pisarla", leido.TasaPct)
	}
}

func TestReservaPorProcesoSinFilaEsNoEncontrado(t *testing.T) {
	s := sembrarCorridaBase(t)
	_, err := s.ReservaPorProceso(t.Context(), "proceso-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}

func TestActualizarSaldoReservaPersisteLoQueDevuelveFn(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	var saldoRecibido decimal.Decimal
	err = s.ActualizarSaldoReserva(ctx, "proceso-1", func(saldoActual decimal.Decimal) (decimal.Decimal, error) {
		saldoRecibido = saldoActual
		return dec("12.34"), nil
	})
	if err != nil {
		t.Fatalf("actualizar saldo: %v", err)
	}
	if !saldoRecibido.Equal(dec("50.00")) {
		t.Fatalf("saldo recibido por fn = %s, se esperaba 50.00", saldoRecibido)
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if !leido.Saldo.Equal(dec("12.34")) {
		t.Fatalf("saldo persistido = %s, se esperaba 12.34", leido.Saldo)
	}
}

func TestActualizarSaldoReservaNoPersisteSiFnFalla(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	errFn := errors.New("fallo de negocio")
	err = s.ActualizarSaldoReserva(ctx, "proceso-1", func(decimal.Decimal) (decimal.Decimal, error) {
		return dec("999.99"), errFn
	})
	if !errors.Is(err, errFn) {
		t.Fatalf("error = %v, se esperaba que se propagara errFn", err)
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if !leido.Saldo.Equal(dec("50.00")) {
		t.Fatalf("saldo = %s, un fn que falla no debio cambiar nada", leido.Saldo)
	}
}

// TestActualizarSaldoReservaEsAtomicoBajoConcurrencia es la reproduccion de
// B1: N liberaciones concurrentes sobre la MISMA reserva nunca deben
// repartir mas de lo que habia. Sin el FOR UPDATE de ActualizarSaldoReserva,
// las N goroutines leerian el mismo saldo de 50 y las N lo consumirian.
func TestActualizarSaldoReservaEsAtomicoBajoConcurrencia(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	const goroutines = 8
	var listas sync.WaitGroup
	arranca := make(chan struct{})
	saldosVistos := make([]decimal.Decimal, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		listas.Add(1)
		go func(i int) {
			defer listas.Done()
			<-arranca
			errs[i] = s.ActualizarSaldoReserva(ctx, "proceso-1", func(saldoActual decimal.Decimal) (decimal.Decimal, error) {
				saldosVistos[i] = saldoActual
				// Consume todo lo que ve: si dos goroutines ven 50, las dos
				// intentan dejar el saldo en cero y el total repartido
				// (fuera de este test, en BolsasAccesorias) se duplicaria.
				return decimal.Zero, nil
			})
		}(i)
	}
	close(arranca)
	listas.Wait()

	sumaVista := decimal.Zero
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: error inesperado: %v", i, err)
		}
		sumaVista = sumaVista.Add(saldosVistos[i])
	}
	// Si el bloqueo funciona, solo UNA goroutine puede haber visto 50.00; el
	// resto tuvo que ver 0.00 (la reserva ya consumida por la primera).
	if !sumaVista.Equal(dec("50.00")) {
		t.Fatalf("suma de saldos vistos por las %d goroutines = %s, se esperaba 50.00 "+
			"(cada una deberia ver el saldo YA consumido por las anteriores, no el original)",
			goroutines, sumaVista)
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if !leido.Saldo.IsZero() {
		t.Fatalf("saldo final = %s, se esperaba cero", leido.Saldo)
	}
}
