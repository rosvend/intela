package postgres

import (
	"errors"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

func TestRendimientosAcrecerYPorCircuitoYVigenciaRedondaLaFila(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("100.00")); err != nil {
		t.Fatalf("acrecer rendimiento: %v", err)
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if leido.Circuito != reparto.Nacional || leido.Vigencia != "2026" || !leido.Monto.Equal(dec("100.00")) {
		t.Fatalf("leido = %+v, se esperaba nacional/2026/100.00", leido)
	}
}

func TestRendimientosAcrecerSumaAlMontoExistente(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("100.00")); err != nil {
		t.Fatalf("acrecer: %v", err)
	}
	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("25.00")); err != nil {
		t.Fatalf("acrecer de nuevo: %v", err)
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !leido.Monto.Equal(dec("125.00")) {
		t.Fatalf("monto = %s, se esperaba 125.00", leido.Monto)
	}
}

// TestRendimientosAcrecerEsAtomicoBajoConcurrencia es la reproduccion de B2:
// el upsert anterior sustituia el monto (ON CONFLICT ... SET monto =
// EXCLUDED.monto) en vez de sumarlo. N acrecimientos concurrentes deben
// sumar exactamente N * incremento.
func TestRendimientosAcrecerEsAtomicoBajoConcurrencia(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	const goroutines = 10
	var listas sync.WaitGroup
	arranca := make(chan struct{})
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		listas.Add(1)
		go func(i int) {
			defer listas.Done()
			<-arranca
			errs[i] = s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("10.00"))
		}(i)
	}
	close(arranca)
	listas.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: error inesperado: %v", i, err)
		}
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !leido.Monto.Equal(dec("100.00")) {
		t.Fatalf("monto final = %s, se esperaba 100.00 (%d x 10.00 sin perder ninguno)", leido.Monto, goroutines)
	}
}

func TestRendimientosSegregaCircuitosEnFilasDistintas(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("100.00")); err != nil {
		t.Fatalf("acrecer nacional: %v", err)
	}
	if err := s.AcrecerRendimiento(ctx, reparto.Internacional, "2026", dec("40.00")); err != nil {
		t.Fatalf("acrecer internacional: %v", err)
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

func TestActualizarMontoRendimientoPersisteLoQueDevuelveFn(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("100.00")); err != nil {
		t.Fatalf("acrecer: %v", err)
	}

	var montoRecibido decimal.Decimal
	err := s.ActualizarMontoRendimiento(ctx, reparto.Nacional, "2026", func(montoActual decimal.Decimal) (decimal.Decimal, error) {
		montoRecibido = montoActual
		return dec("7.00"), nil
	})
	if err != nil {
		t.Fatalf("actualizar monto: %v", err)
	}
	if !montoRecibido.Equal(dec("100.00")) {
		t.Fatalf("monto recibido por fn = %s, se esperaba 100.00", montoRecibido)
	}

	leido, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !leido.Monto.Equal(dec("7.00")) {
		t.Fatalf("monto persistido = %s, se esperaba 7.00", leido.Monto)
	}
}

// TestDistribuirDosVecesNoPagaDosVeces es la reproduccion secuencial de B3:
// distribuir un rendimiento dos veces no debe repartir el mismo importe dos
// veces. ActualizarMontoRendimiento consume el monto (lo deja en el
// residuo), asi que la segunda llamada ve 0.
func TestActualizarMontoRendimientoConsumeElMontoParaLlamadasSiguientes(t *testing.T) {
	s := &Store{pool: testhelp.Pool(t)}
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("100.00")); err != nil {
		t.Fatalf("acrecer: %v", err)
	}

	consumir := func() decimal.Decimal {
		var visto decimal.Decimal
		if err := s.ActualizarMontoRendimiento(ctx, reparto.Nacional, "2026", func(montoActual decimal.Decimal) (decimal.Decimal, error) {
			visto = montoActual
			return decimal.Zero, nil
		}); err != nil {
			t.Fatalf("actualizar monto: %v", err)
		}
		return visto
	}

	primero := consumir()
	segundo := consumir()
	if !primero.Equal(dec("100.00")) {
		t.Fatalf("primer consumo vio %s, se esperaba 100.00", primero)
	}
	if !segundo.IsZero() {
		t.Fatalf("segundo consumo vio %s, se esperaba cero: el monto ya estaba consumido (B3)", segundo)
	}
}
