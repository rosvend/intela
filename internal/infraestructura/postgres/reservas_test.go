package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
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
//
// El pool pide una conexion por goroutine. [testhelp.Pool] deja MaxConns=1,
// y con ese techo las transacciones se serializan en el pool aunque se quite
// el FOR UPDATE: la prueba seguiria en verde y no mediria el cerrojo (#156,
// B10). La pausa dentro de fn abre la ventana: sin el cerrojo las N leen 50
// antes de que ninguna escriba; con el, cada una espera a que la anterior
// confirme.
func TestLiberarSaldoReservaEsAtomicoBajoConcurrencia(t *testing.T) {
	const goroutines = 4
	s := sembrarCorridaSobre(t, poolDePrueba(t, goroutines))
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

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
				time.Sleep(50 * time.Millisecond)
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

// TestLiberarSaldoReservaRechazaRendimientoSinFila cubre el hallazgo de
// CodeRabbit: sin fila (nacional, vigencia) en rendimientos, un FOR UPDATE
// sobre cero filas no bloquea ni falla, y la liberacion seguiria adelante
// usando dinero que el ledger de rendimientos no tiene.
func TestLiberarSaldoReservaRechazaRendimientoSinFila(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	llamadoFn := false
	err = s.LiberarSaldoReserva(ctx, "proceso-1", "2026-sin-fila", dec("10.00"),
		func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
			llamadoFn = true
			return decimal.Zero, nil, nil
		})
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
	if llamadoFn {
		t.Fatal("fn no debio invocarse: el rendimiento no existe")
	}

	leido, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer reserva: %v", err)
	}
	if !leido.Saldo.Equal(dec("50.00")) {
		t.Fatalf("saldo = %s, no debio cambiar: la transaccion tuvo que revertir", leido.Saldo)
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
			// Sin lineas el rendimiento entero es residuo y el ledger no se
			// mueve: las dos liberaciones saldrian bien. Una linea con el
			// importe completo parte exacto y si descuenta los 50.00.
			errs[i] = s.LiberarSaldoReserva(ctx, procesos[i], "2026", dec("50.00"),
				func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
					return decimal.Zero, []reparto.LineaTitular{{
						ObraID: "obra-1", TitularID: "titular-a", IPI: "111",
						Porcentaje: dec("100"), Importe: dec("50.00"),
					}}, nil
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

// TestLiberarSaldoReservaDejaRastroDelRendimientoConsumido es B9 (#156).
// Consumir un rendimiento al liberar la reserva baja rendimientos.monto y
// deja en rendimientos_distribuciones la parte que salio de ese ledger. Sin
// ese rastro, sum(reservas_liberaciones.importe) puede superar
// reservas.monto_inicial y nadie explica la diferencia.
func TestLiberarSaldoReservaDejaRastroDelRendimientoConsumido(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("20.00")); err != nil {
		t.Fatalf("acrecer rendimiento: %v", err)
	}
	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("50.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	// 28+42 = 70, que es saldo 50 mas rendimiento 20. Las proporciones 40/60
	// parten el rendimiento en 8 y 12, sin residuo.
	lineas := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("28.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("42.00")},
	}
	err = s.LiberarSaldoReserva(ctx, "proceso-1", "2026", dec("20.00"),
		func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
			return decimal.Zero, lineas, nil
		})
	if err != nil {
		t.Fatalf("liberar saldo: %v", err)
	}

	rendimiento, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !rendimiento.Monto.IsZero() {
		t.Fatalf("monto = %s, se esperaba 0 (20.00 consumidos)", rendimiento.Monto)
	}

	var liberado decimal.Decimal
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(importe), 0) FROM reservas_liberaciones WHERE proceso_id = 'proceso-1'`,
	).Scan(&liberado); err != nil {
		t.Fatalf("sumar liberaciones: %v", err)
	}
	if !liberado.Equal(dec("70.00")) {
		t.Fatalf("liberado = %s, se esperaba 70.00", liberado)
	}
	if !liberado.GreaterThan(dec("50.00")) {
		t.Fatal("la liberacion no supero monto_inicial: el rastro del rendimiento no tendria que explicar nada")
	}

	esperadas, residuo, err := reparto.DistribuirSobreProporciones(dec("20.00"), lineas)
	if err != nil {
		t.Fatalf("partir el rendimiento: %v", err)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, este caso parte exacto", residuo)
	}

	filas, err := s.pool.Query(ctx,
		`SELECT obra_id, titular_id, importe FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'
		  ORDER BY titular_id`)
	if err != nil {
		t.Fatalf("leer rendimientos_distribuciones: %v", err)
	}
	defer filas.Close()

	var vistas int
	var distribuido decimal.Decimal
	for filas.Next() {
		var obraID, titularID string
		var importe decimal.Decimal
		if err := filas.Scan(&obraID, &titularID, &importe); err != nil {
			t.Fatalf("escanear fila: %v", err)
		}
		if vistas >= len(esperadas) || obraID != esperadas[vistas].ObraID || titularID != esperadas[vistas].TitularID ||
			!importe.Equal(esperadas[vistas].Importe) {
			t.Fatalf("fila %d = %s/%s %s, se esperaba %+v", vistas, obraID, titularID, importe, esperadas)
		}
		distribuido = distribuido.Add(importe)
		vistas++
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("leer rendimientos_distribuciones: %v", err)
	}
	if vistas != len(esperadas) {
		t.Fatalf("se persistieron %d filas, se esperaban %d", vistas, len(esperadas))
	}
	if !distribuido.Equal(dec("20.00")) {
		t.Fatalf("distribuido = %s, se esperaba 20.00: es lo que bajo rendimientos.monto", distribuido)
	}
}

// TestLiberarSaldoReservaReconciliaUnRepartoConResiduo es la prueba que B1
// exige: un reparto que no parte exacto. Tres lineas iguales de 10 dejan
// residuo 0.01; ese centavo se queda en el ledger y la suma distribuida
// iguala lo que rendimientos.monto bajo.
func TestLiberarSaldoReservaReconciliaUnRepartoConResiduo(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("10.00")); err != nil {
		t.Fatalf("acrecer rendimiento: %v", err)
	}
	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, dec("30.00"), dec("5"))
	if err != nil {
		t.Fatalf("construir pool: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}

	lineas := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("10.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("100"), Importe: dec("10.00")},
		{ObraID: "obra-2", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("10.00")},
	}
	_, residuo, err := reparto.DistribuirSobreProporciones(dec("10.00"), lineas)
	if err != nil {
		t.Fatalf("partir el rendimiento: %v", err)
	}
	if !residuo.Equal(dec("0.01")) {
		t.Fatalf("precondicion: residuo = %s, se esperaba 0.01", residuo)
	}

	if err := s.LiberarSaldoReserva(ctx, "proceso-1", "2026", dec("10.00"),
		func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
			return decimal.Zero, lineas, nil
		}); err != nil {
		t.Fatalf("liberar saldo: %v", err)
	}

	despues, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !despues.Monto.Equal(dec("0.01")) {
		t.Fatalf("monto = %s, el residuo 0.01 tenia que quedarse en el ledger", despues.Monto)
	}
	distribuido := sumarImportes(t, s,
		`SELECT COALESCE(SUM(importe), 0) FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'`)
	if !distribuido.Equal(dec("10.00").Sub(despues.Monto)) {
		t.Fatalf("distribuido = %s, el ledger bajo %s", distribuido, dec("10.00").Sub(despues.Monto))
	}
	if !distribuido.Equal(dec("9.99")) {
		t.Fatalf("distribuido = %s, se esperaba 9.99", distribuido)
	}

	filas, err := s.pool.Query(ctx,
		`SELECT importe FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'`)
	if err != nil {
		t.Fatalf("leer distribuciones: %v", err)
	}
	defer filas.Close()
	var n int
	for filas.Next() {
		var importe decimal.Decimal
		if err := filas.Scan(&importe); err != nil {
			t.Fatalf("escanear importe: %v", err)
		}
		if !importe.Equal(dec("3.33")) {
			t.Fatalf("importe = %s, el residuo no se absorbe en una linea", importe)
		}
		n++
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("leer distribuciones: %v", err)
	}
	if n != 3 {
		t.Fatalf("se persistieron %d filas, se esperaban 3", n)
	}
}

// TestLiberarReservaPrescritaRetieneElResiduoDelRendimiento recorre el caso
// de uso: tres titulares iguales, reserva 30 y rendimiento 10. La segunda
// particion deja 0.01 en el ledger y lo distribuido iguala esa baja.
func TestLiberarReservaPrescritaRetieneElResiduoDelRendimiento(t *testing.T) {
	s, saldo := liberarPrescrita(t, dec("30.00"), dec("10.00"), dec("10.00"), []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("100.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("100"), Importe: dec("100.00")},
		{ObraID: "obra-2", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("100.00")},
	})
	ctx := t.Context()

	rend, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !rend.Monto.Equal(dec("0.01")) {
		t.Fatalf("monto = %s, se esperaba 0.01 en el ledger", rend.Monto)
	}
	if !saldo.IsZero() {
		t.Fatalf("saldo devuelto = %s, se esperaba 0", saldo)
	}
	debeCuadrarLiberacion(t, s, dec("30.00"), dec("10.00"), saldo, rend.Monto)
}

// TestLiberarReservaPrescritaSinTitularesDejaElRendimientoEnElLedger es la
// sonda de lineas vacias: DistribuirSobreProporciones devuelve el importe
// entero como residuo. No sale del ledger y no entra en la reserva.
func TestLiberarReservaPrescritaSinTitularesDejaElRendimientoEnElLedger(t *testing.T) {
	s, saldo := liberarPrescrita(t, dec("50.00"), dec("20.00"), dec("20.00"), nil)
	ctx := t.Context()

	if !saldo.Equal(dec("50.00")) {
		t.Fatalf("saldo = %s, la reserva no debio absorber el rendimiento", saldo)
	}
	rend, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !rend.Monto.Equal(dec("20.00")) {
		t.Fatalf("monto = %s, los 20.00 tenian que seguir en el ledger", rend.Monto)
	}
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'`,
	).Scan(&n); err != nil {
		t.Fatalf("contar distribuciones: %v", err)
	}
	if n != 0 {
		t.Fatalf("se persistieron %d filas, se esperaban 0", n)
	}
	debeCuadrarLiberacion(t, s, dec("50.00"), dec("20.00"), saldo, rend.Monto)
}

// TestLiberarReservaPrescritaReconciliaCuandoElRepartoSePasa es la fixture
// 160/240/300: repartir 100 redondea a 100.01. Si el ledger tiene el centavo,
// sale de ahi y las lineas del dominio se guardan enteras.
func TestLiberarReservaPrescritaReconciliaCuandoElRepartoSePasa(t *testing.T) {
	titulares := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("160.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("240.00")},
		{ObraID: "obra-2", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("300.00")},
	}
	s, saldo := liberarPrescrita(t, dec("600.00"), dec("200.00"), dec("100.00"), titulares)
	ctx := t.Context()

	rend, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !rend.Monto.Equal(dec("99.99")) {
		t.Fatalf("monto = %s, se esperaba 99.99 (200.00 - 100.01)", rend.Monto)
	}
	if !saldo.Equal(dec("0.01")) {
		t.Fatalf("saldo = %s, el centavo de mas no puede evaporarse", saldo)
	}
	debeCuadrarLiberacion(t, s, dec("600.00"), dec("200.00"), saldo, rend.Monto)

	partes, residuo, err := reparto.DistribuirSobreProporciones(dec("100.00"), []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", Importe: dec("160.00")},
		{ObraID: "obra-1", TitularID: "titular-b", Importe: dec("240.00")},
		{ObraID: "obra-2", TitularID: "titular-a", Importe: dec("300.00")},
	})
	if err != nil {
		t.Fatalf("partir: %v", err)
	}
	if !residuo.Equal(dec("-0.01")) {
		t.Fatalf("precondicion: residuo = %s, se esperaba -0.01", residuo)
	}
	filas, err := s.pool.Query(ctx,
		`SELECT importe FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'
		  ORDER BY obra_id, titular_id`)
	if err != nil {
		t.Fatalf("leer distribuciones: %v", err)
	}
	defer filas.Close()
	for i := 0; filas.Next(); i++ {
		var importe decimal.Decimal
		if err := filas.Scan(&importe); err != nil {
			t.Fatalf("escanear: %v", err)
		}
		if i >= len(partes) || !importe.Equal(partes[i].Importe) {
			t.Fatalf("fila %d = %s, se esperaba la linea del dominio", i, importe)
		}
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("leer distribuciones: %v", err)
	}
}

// TestLiberarReservaPrescritaNoAtribuyeElCentavoQueElLedgerNoTiene: la misma
// fixture con el ledger justo en 100. El redondeo pide 100.01 y el CHECK
// (monto >= 0) no deja escribirlo. El centavo no se atribuye; lo que baja
// el ledger iguala la suma persistida.
func TestLiberarReservaPrescritaNoAtribuyeElCentavoQueElLedgerNoTiene(t *testing.T) {
	titulares := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("160.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("240.00")},
		{ObraID: "obra-2", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("300.00")},
	}
	s, saldo := liberarPrescrita(t, dec("600.00"), dec("100.00"), dec("100.00"), titulares)
	ctx := t.Context()

	rend, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !rend.Monto.IsZero() {
		t.Fatalf("monto = %s, se esperaba 0", rend.Monto)
	}
	if !saldo.IsZero() {
		t.Fatalf("saldo = %s, se esperaba 0", saldo)
	}
	distribuido := sumarImportes(t, s,
		`SELECT COALESCE(SUM(importe), 0) FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'`)
	if !distribuido.Equal(dec("100.00")) {
		t.Fatalf("distribuido = %s, se esperaba 100.00", distribuido)
	}
	debeCuadrarLiberacion(t, s, dec("600.00"), dec("100.00"), saldo, rend.Monto)
}

func liberarPrescrita(t *testing.T, reserva, rendimientoEnLedger, rendimientoAUsar decimal.Decimal, titulares []reparto.LineaTitular) (*Store, decimal.Decimal) {
	t.Helper()
	s := sembrarCorridaBase(t)
	ctx := t.Context()
	obras := make([]reparto.LineaObra, 0, 2)
	vistas := map[string]bool{}
	for _, l := range titulares {
		if vistas[l.ObraID] {
			continue
		}
		vistas[l.ObraID] = true
		obras = append(obras, reparto.LineaObra{ObraID: l.ObraID, Puntos: dec("1"), Importe: l.Importe})
	}
	if err := s.GuardarResultado(ctx, "proceso-1", reparto.Resultado{
		Reserva:    reserva,
		SnapshotID: "snap-1",
		Reglamento: "IX",
		Obras:      obras,
		Titulares:  titulares,
	}); err != nil {
		t.Fatalf("guardar resultado: %v", err)
	}
	pool, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, reserva, dec("5"))
	if err != nil {
		t.Fatalf("construir reserva: %v", err)
	}
	if err := s.CrearReserva(ctx, pool); err != nil {
		t.Fatalf("crear reserva: %v", err)
	}
	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", rendimientoEnLedger); err != nil {
		t.Fatalf("acrecer rendimiento: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
		 VALUES ('proceso-2', 'nacional', 'importe_titular', '2027-01', 'bolsa-1', 'snap-1', 'IX')`); err != nil {
		t.Fatalf("sembrar la corrida de destino: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 VALUES ('actor-1', 'actor-1@redes.test', 'Actor 1', 'auditor', 'hash-de-prueba-suficientemente-larga')`); err != nil {
		t.Fatalf("sembrar actor: %v", err)
	}
	b := aplicacion.BolsasAccesorias{
		Resultados: s, Reservas: s, Rendimientos: s,
		Corridas: s, Bitacora: s, Unidad: s, Reloj: reloj.Sistema{},
	}
	_, saldo, err := b.LiberarReservaPrescrita(ctx, "proceso-1", "proceso-2", "2026", rendimientoAUsar, "actor-1")
	if err != nil {
		t.Fatalf("liberar reserva: %v", err)
	}
	return s, saldo
}

func sumarImportes(t *testing.T, s *Store, query string, args ...any) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := s.pool.QueryRow(t.Context(), query, args...).Scan(&v); err != nil {
		t.Fatalf("sumar importes: %v", err)
	}
	return v
}

func debeCuadrarLiberacion(t *testing.T, s *Store, reservaAntes, rendimientoAntes, reservaDespues, rendimientoDespues decimal.Decimal) {
	t.Helper()
	liberado := sumarImportes(t, s, `SELECT COALESCE(SUM(importe), 0) FROM reservas_liberaciones WHERE proceso_id = 'proceso-1'`)
	distribuido := sumarImportes(t, s,
		`SELECT COALESCE(SUM(importe), 0) FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'`)
	if !distribuido.Equal(rendimientoAntes.Sub(rendimientoDespues)) {
		t.Fatalf("distribuido = %s, el ledger bajo %s", distribuido, rendimientoAntes.Sub(rendimientoDespues))
	}
	izquierda := reservaAntes.Add(rendimientoAntes)
	derecha := reservaDespues.Add(rendimientoDespues).Add(liberado)
	if !izquierda.Equal(derecha) {
		t.Fatalf("conservacion: habia %s, quedo %s (reserva %s + rendimiento %s + liberado %s)",
			izquierda, derecha, reservaDespues, rendimientoDespues, liberado)
	}
}
