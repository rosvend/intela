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

func TestLiberarSaldoReservaPersisteLoQueDevuelveFn(t *testing.T) {
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
	err = s.LiberarSaldoReserva(ctx, "proceso-1", "", decimal.Zero, func(saldoActual decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
		saldoRecibido = saldoActual
		return dec("12.34"), nil, nil
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

func TestLiberarSaldoReservaNoPersisteSiFnFalla(t *testing.T) {
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
	err = s.LiberarSaldoReserva(ctx, "proceso-1", "", decimal.Zero, func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
		return dec("999.99"), nil, errFn
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

// TestLiberarSaldoReservaEsAtomicoBajoConcurrencia es la reproduccion de
// B1: N liberaciones concurrentes sobre la MISMA reserva nunca deben
// repartir mas de lo que habia. Sin el FOR UPDATE de LiberarSaldoReserva,
// las N goroutines leerian el mismo saldo de 50 y las N lo consumirian.
func TestLiberarSaldoReservaEsAtomicoBajoConcurrencia(t *testing.T) {
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
			errs[i] = s.LiberarSaldoReserva(ctx, "proceso-1", "", decimal.Zero, func(saldoActual decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
				saldosVistos[i] = saldoActual
				// Consume todo lo que ve: si dos goroutines ven 50, las dos
				// intentan dejar el saldo en cero y el total repartido
				// (fuera de este test, en BolsasAccesorias) se duplicaria.
				return decimal.Zero, nil, nil
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

// TestLiberarSaldoReservaPersisteLasLineas es B2: las lineas que fn reparte
// quedan en reservas_liberaciones en la misma transaccion que baja el saldo.
func TestLiberarSaldoReservaPersisteLasLineas(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	lineas := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("20.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("30.00")},
	}
	err = s.LiberarSaldoReserva(ctx, "proceso-1", "", decimal.Zero,
		func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
			return decimal.Zero, lineas, nil
		})
	if err != nil {
		t.Fatalf("liberar saldo: %v", err)
	}

	filas, err := s.pool.Query(ctx,
		`SELECT obra_id, titular_id, importe FROM reservas_liberaciones WHERE proceso_id = $1 ORDER BY titular_id`,
		"proceso-1")
	if err != nil {
		t.Fatalf("leer reservas_liberaciones: %v", err)
	}
	defer filas.Close()

	var vistas int
	for filas.Next() {
		var obraID, titularID string
		var importe decimal.Decimal
		if err := filas.Scan(&obraID, &titularID, &importe); err != nil {
			t.Fatalf("escanear fila: %v", err)
		}
		if !importe.Equal(lineas[vistas].Importe) {
			t.Fatalf("linea %d: importe = %s, se esperaba %s", vistas, importe, lineas[vistas].Importe)
		}
		vistas++
	}
	if vistas != len(lineas) {
		t.Fatalf("se persistieron %d lineas, se esperaban %d", vistas, len(lineas))
	}
}

// TestLiberarSaldoReservaDescuentaRendimientoAtomicamenteBajoConcurrencia es
// la reproduccion de B4: sin bloquear la fila de rendimientos, dos
// liberaciones concurrentes verian el mismo monto disponible y las dos lo
// usarian -- el mismo rendimiento financiando dos liberaciones a la vez.
func TestLiberarSaldoReservaDescuentaRendimientoAtomicamenteBajoConcurrencia(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("50.00")); err != nil {
		t.Fatalf("acrecer rendimiento: %v", err)
	}

	for _, procesoID := range []string{"proceso-1", "proceso-2"} {
		if procesoID != "proceso-1" {
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
				 VALUES ('bolsa-2', 'usuario-1', '2026-02', 'nacional', 1000.00)`); err != nil {
				t.Fatalf("sembrar bolsa-2: %v", err)
			}
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
				 VALUES ($1, 'nacional', 'importe_titular', '2026-02', 'bolsa-2', 'snap-1', 'IX')`,
				procesoID); err != nil {
				t.Fatalf("sembrar %q: %v", procesoID, err)
			}
		}
		pool, err := reparto.NuevaPoolReserva(procesoID, reparto.Nacional, dec("10.00"), dec("5"))
		if err != nil {
			t.Fatalf("construir pool: %v", err)
		}
		if err := s.CrearReserva(ctx, pool); err != nil {
			t.Fatalf("crear reserva de %q: %v", procesoID, err)
		}
	}

	var listas sync.WaitGroup
	arranca := make(chan struct{})
	errs := make([]error, 2)
	procesos := []string{"proceso-1", "proceso-2"}

	for i := 0; i < 2; i++ {
		listas.Add(1)
		go func(i int) {
			defer listas.Done()
			<-arranca
			errs[i] = s.LiberarSaldoReserva(ctx, procesos[i], "2026", dec("50.00"),
				func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
					return decimal.Zero, nil, nil
				})
		}(i)
	}
	close(arranca)
	listas.Wait()

	exitos := 0
	for _, err := range errs {
		if err == nil {
			exitos++
		}
	}
	if exitos != 1 {
		t.Fatalf("liberaciones exitosas = %d, se esperaba exactamente 1: el rendimiento de 50.00 solo alcanza para una", exitos)
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !leido.Monto.IsZero() {
		t.Fatalf("monto de rendimiento tras la unica liberacion exitosa = %s, se esperaba cero", leido.Monto)
	}
}
