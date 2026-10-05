package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

type resultadosFalso struct {
	porProceso map[string]reparto.Resultado
	err        error
}

func (r *resultadosFalso) GuardarResultado(context.Context, string, reparto.Resultado) error {
	return r.err
}

func (r *resultadosFalso) ResultadoPorProceso(_ context.Context, procesoID string) (reparto.Resultado, error) {
	if r.err != nil {
		return reparto.Resultado{}, r.err
	}
	res, ok := r.porProceso[procesoID]
	if !ok {
		return reparto.Resultado{}, ErrNoEncontrado
	}
	return res, nil
}

// reservasFalso no simula el bloqueo de fila que hace race-safe a
// LiberarSaldoReserva de verdad -- eso lo prueban las pruebas de
// concurrencia contra Postgres en internal/infraestructura/postgres. Aqui
// solo se comprueba el contrato: fn recibe el saldo actual y lo que
// devuelve queda persistido.
type reservasFalso struct {
	creadas map[string]bool
	porProc map[string]reparto.PoolReserva
	err     error
}

func (r *reservasFalso) CrearReserva(_ context.Context, p reparto.PoolReserva) error {
	if r.err != nil {
		return r.err
	}
	if r.porProc == nil {
		r.porProc = map[string]reparto.PoolReserva{}
	}
	if r.creadas == nil {
		r.creadas = map[string]bool{}
	}
	if r.creadas[p.ProcesoID] {
		return ErrReservaYaRegistrada
	}
	r.creadas[p.ProcesoID] = true
	r.porProc[p.ProcesoID] = p
	return nil
}

func (r *reservasFalso) ReservaPorProceso(_ context.Context, procesoID string) (reparto.PoolReserva, error) {
	if r.err != nil {
		return reparto.PoolReserva{}, r.err
	}
	p, ok := r.porProc[procesoID]
	if !ok {
		return reparto.PoolReserva{}, ErrNoEncontrado
	}
	return p, nil
}

func (r *reservasFalso) LiberarSaldoReserva(_ context.Context, procesoID, _ string, _ decimal.Decimal, fn func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error)) error {
	if r.err != nil {
		return r.err
	}
	p, ok := r.porProc[procesoID]
	if !ok {
		return ErrNoEncontrado
	}
	nuevoSaldo, _, err := fn(p.Saldo)
	if err != nil {
		return err
	}
	p.Saldo = nuevoSaldo
	r.porProc[procesoID] = p
	return nil
}

// rendimientosFalso: misma advertencia que reservasFalso sobre concurrencia.
type rendimientosFalso struct {
	porClave map[string]reparto.PoolRendimiento
	err      error
}

func claveRendimiento(circuito reparto.Circuito, vigencia string) string {
	return string(circuito) + "|" + vigencia
}

func (r *rendimientosFalso) AcrecerRendimiento(_ context.Context, circuito reparto.Circuito, vigencia string, incremento decimal.Decimal) error {
	if r.err != nil {
		return r.err
	}
	if r.porClave == nil {
		r.porClave = map[string]reparto.PoolRendimiento{}
	}
	clave := claveRendimiento(circuito, vigencia)
	p := r.porClave[clave]
	p.Circuito, p.Vigencia = circuito, vigencia
	p.Monto = p.Monto.Add(incremento)
	r.porClave[clave] = p
	return nil
}

func (r *rendimientosFalso) PorCircuitoYVigencia(_ context.Context, circuito reparto.Circuito, vigencia string) (reparto.PoolRendimiento, error) {
	if r.err != nil {
		return reparto.PoolRendimiento{}, r.err
	}
	p, ok := r.porClave[claveRendimiento(circuito, vigencia)]
	if !ok {
		return reparto.PoolRendimiento{}, ErrNoEncontrado
	}
	return p, nil
}

func (r *rendimientosFalso) ActualizarMontoRendimiento(_ context.Context, circuito reparto.Circuito, vigencia, _ string, fn func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error)) error {
	if r.err != nil {
		return r.err
	}
	clave := claveRendimiento(circuito, vigencia)
	p, ok := r.porClave[clave]
	if !ok {
		return ErrNoEncontrado
	}
	nuevoMonto, _, err := fn(p.Monto)
	if err != nil {
		return err
	}
	p.Monto = nuevoMonto
	r.porClave[clave] = p
	return nil
}

type reclamacionesFalso struct {
	guardadas []reparto.ReclamacionReserva
	porID     map[string]reparto.ReclamacionReserva
	err       error
}

func (r *reclamacionesFalso) GuardarReclamacion(_ context.Context, c reparto.ReclamacionReserva) error {
	if r.err != nil {
		return r.err
	}
	r.guardadas = append(r.guardadas, c)
	if r.porID == nil {
		r.porID = map[string]reparto.ReclamacionReserva{}
	}
	r.porID[c.ID] = c
	return nil
}

func (r *reclamacionesFalso) ReclamacionPorID(_ context.Context, id string) (reparto.ReclamacionReserva, error) {
	if r.err != nil {
		return reparto.ReclamacionReserva{}, r.err
	}
	c, ok := r.porID[id]
	if !ok {
		return reparto.ReclamacionReserva{}, ErrNoEncontrado
	}
	return c, nil
}

func resultadoConDosTitulares() reparto.Resultado {
	return reparto.Resultado{
		Reserva: decimal.RequireFromString("50.00"),
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: decimal.RequireFromString("40"), Importe: decimal.RequireFromString("400.00")},
			{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: decimal.RequireFromString("60"), Importe: decimal.RequireFromString("600.00")},
		},
	}
}

func TestRegistrarReservaRechazaCircuitoInternacional(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{}
	b := BolsasAccesorias{Resultados: resultados, Reservas: reservas}

	_, err := b.RegistrarReserva(context.Background(), "p1", reparto.Internacional, decimal.RequireFromString("5"))
	if !errors.Is(err, reparto.ErrReservaInternacional) {
		t.Fatalf("error = %v, se esperaba ErrReservaInternacional", err)
	}
	if len(reservas.porProc) != 0 {
		t.Fatal("no debio persistir nada cuando el dominio rechaza")
	}
}

func TestRegistrarReservaPersisteElMontoDeLaCorrida(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{}
	b := BolsasAccesorias{Resultados: resultados, Reservas: reservas}

	p, err := b.RegistrarReserva(context.Background(), "p1", reparto.Nacional, decimal.RequireFromString("5"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !p.MontoInicial.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("monto inicial = %s, se esperaba el Reserva de la corrida (50.00)", p.MontoInicial)
	}
	if len(reservas.porProc) != 1 {
		t.Fatalf("se esperaba 1 reserva persistida, hubo %d", len(reservas.porProc))
	}
}

func TestRegistrarReservaNoPuedeRegistrarseDosVeces(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{}
	b := BolsasAccesorias{Resultados: resultados, Reservas: reservas}
	ctx := context.Background()

	if _, err := b.RegistrarReserva(ctx, "p1", reparto.Nacional, decimal.RequireFromString("5")); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err := b.RegistrarReserva(ctx, "p1", reparto.Nacional, decimal.RequireFromString("4"))
	if !errors.Is(err, ErrReservaYaRegistrada) {
		t.Fatalf("error = %v, se esperaba ErrReservaYaRegistrada: un segundo alta no puede pisar tasa ni monto en silencio (N2)", err)
	}
	if !reservas.porProc["p1"].TasaPct.Equal(decimal.RequireFromString("5")) {
		t.Fatalf("tasa_pct = %s, el segundo alta no debio cambiarla", reservas.porProc["p1"].TasaPct)
	}
}

func TestLiberarReservaPrescritaReplicaLasProporcionesDeLaCorridaOriginal(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	libro := &bitacoraFalsa{}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Reservas: reservas}, libro)

	nuevas, residuo, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p2", "2026", decimal.Zero, "actor-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !nuevas[0].Importe.Equal(decimal.RequireFromString("20.00")) || !nuevas[1].Importe.Equal(decimal.RequireFromString("30.00")) {
		t.Fatalf("importes = %s, %s; se esperaba 40/60 de 50.00 = 20.00/30.00", nuevas[0].Importe, nuevas[1].Importe)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero", residuo)
	}
	if !reservas.porProc["p1"].Saldo.IsZero() {
		t.Fatalf("saldo tras liberar = %s, se esperaba cero (todo el remanente se distribuyo)", reservas.porProc["p1"].Saldo)
	}
	if len(libro.asientos) != 1 || libro.asientos[0].Hecho != HechoReservaLiberada || libro.asientos[0].RefID != "p2" {
		t.Fatalf("asiento = %+v, se esperaba reserva.liberada sobre la corrida de destino", libro.asientos)
	}
	var payload asientoReservaLiberada
	if err := json.Unmarshal(libro.asientos[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Origen.ProcesoID != "p1" || payload.Destino.ProcesoID != "p2" || payload.VigenciaRendimiento != "2026" {
		t.Fatalf("payload = %+v, el linaje tiene que nombrar las dos corridas", payload)
	}
}

func TestLiberarReservaPrescritaIncluyeElRendimientoAcumulado(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	libro := &bitacoraFalsa{}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Reservas: reservas}, libro)

	// RD 10.4: el rendimiento acumulado sobre la reserva se incluye al liberarla.
	nuevas, _, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p2", "2026", decimal.RequireFromString("10.00"), "actor-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	suma := nuevas[0].Importe.Add(nuevas[1].Importe)
	if !suma.Equal(decimal.RequireFromString("60.00")) {
		t.Fatalf("suma repartida = %s, se esperaba 60.00 (50.00 de saldo + 10.00 de rendimiento)", suma)
	}
	// El importe de cada linea mezcla reserva y rendimiento; el asiento dice
	// cuanto puso cada uno y que quedo, sin recalcular.
	var payload asientoReservaLiberada
	if err := json.Unmarshal(libro.asientos[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.SaldoReserva != "50.00" || payload.RendimientoSumado != "10.00" || payload.SaldoRestante != "0.00" {
		t.Fatalf("desglose = saldo %q, rendimiento %q, restante %q; se esperaba 50.00/10.00/0.00",
			payload.SaldoReserva, payload.RendimientoSumado, payload.SaldoRestante)
	}
}

func TestLiberarReservaPrescritaRechazaRendimientoNegativo(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	b := BolsasAccesorias{Resultados: resultados, Reservas: reservas}

	_, _, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p2", "2026", decimal.RequireFromString("-20.00"), "actor-1")
	if !errors.Is(err, reparto.ErrRepartoInvalido) {
		t.Fatalf("error = %v, se esperaba ErrRepartoInvalido: el mismo PR ya rechaza un rendimiento negativo en RegistrarRendimiento", err)
	}
	if !reservas.porProc["p1"].Saldo.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("saldo = %s, el rechazo no debio tocar la reserva", reservas.porProc["p1"].Saldo)
	}
}

func TestLiberarReservaPrescritaDejaElSaldoIgualAlResiduoSinTitulares(t *testing.T) {
	t.Parallel()
	// Corrida con la reserva retenida pero sin ninguna linea de titular --
	// todas las obras quedaron con declaracion_incompleta. No hay a quien
	// repartir, y el importe no puede evaporarse (B5).
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": {Reserva: decimal.RequireFromString("50.00")}}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Reservas: reservas}, &bitacoraFalsa{})

	nuevas, residuo, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p2", "2026", decimal.Zero, "actor-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(nuevas) != 0 {
		t.Fatalf("se esperaban 0 lineas, hubo %d", len(nuevas))
	}
	if !residuo.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("residuo = %s, se esperaba 50.00 (nada que repartir)", residuo)
	}
	if !reservas.porProc["p1"].Saldo.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("saldo = %s, se esperaba que se quedara en 50.00 -- no se evapora lo que no se pudo repartir", reservas.porProc["p1"].Saldo)
	}
}

func TestRegistrarRendimientoAbreElPoolLaPrimeraVez(t *testing.T) {
	t.Parallel()
	rendimientos := &rendimientosFalso{}
	b := BolsasAccesorias{Rendimientos: rendimientos}
	ctx := context.Background()

	if err := b.RegistrarRendimiento(ctx, reparto.Nacional, "2026", decimal.RequireFromString("100.00")); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err := rendimientos.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !p.Monto.Equal(decimal.RequireFromString("100.00")) {
		t.Fatalf("monto = %s, se esperaba 100.00", p.Monto)
	}
}

func TestRegistrarRendimientoAcreceElPoolExistente(t *testing.T) {
	t.Parallel()
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
	}}
	b := BolsasAccesorias{Rendimientos: rendimientos}
	ctx := context.Background()

	if err := b.RegistrarRendimiento(ctx, reparto.Nacional, "2026", decimal.RequireFromString("50.00")); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err := rendimientos.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !p.Monto.Equal(decimal.RequireFromString("150.00")) {
		t.Fatalf("monto = %s, se esperaba 150.00 (acrecido)", p.Monto)
	}
}

func TestRegistrarRendimientoRechazaVigenciaInvalida(t *testing.T) {
	t.Parallel()
	rendimientos := &rendimientosFalso{}
	b := BolsasAccesorias{Rendimientos: rendimientos}

	err := b.RegistrarRendimiento(context.Background(), reparto.Nacional, "2026-01", decimal.RequireFromString("100.00"))
	if !errors.Is(err, reparto.ErrRepartoInvalido) {
		t.Fatalf("error = %v, se esperaba ErrRepartoInvalido (vigencia de 4 digitos)", err)
	}
	if len(rendimientos.porClave) != 0 {
		t.Fatal("no debio llegar al repositorio con una vigencia invalida")
	}
}

func TestDistribuirRendimientoNoRevalorizaLaCorrida(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
	}}
	libro := &bitacoraFalsa{}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Rendimientos: rendimientos}, libro)

	nuevas, residuo, err := b.DistribuirRendimiento(context.Background(), "p1", "p2", reparto.Nacional, "2026", "actor-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !nuevas[0].Importe.Equal(decimal.RequireFromString("40.00")) || !nuevas[1].Importe.Equal(decimal.RequireFromString("60.00")) {
		t.Fatalf("importes = %s, %s; se esperaba 40/60 de 100.00", nuevas[0].Importe, nuevas[1].Importe)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero", residuo)
	}
	if len(libro.asientos) != 1 || libro.asientos[0].Hecho != HechoRendimientosDistribuidos || libro.asientos[0].RefID != "p2" {
		t.Fatalf("asiento = %+v", libro.asientos)
	}
	var payload asientoRendimientosDistribuidos
	if err := json.Unmarshal(libro.asientos[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Origen.ProcesoID != "p1" || payload.Destino.ProcesoID != "p2" || payload.Vigencia != "2026" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestDistribuirRendimientoDosVecesNoRepartDosVeces(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
	}}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Rendimientos: rendimientos}, &bitacoraFalsa{})
	ctx := context.Background()

	if _, _, err := b.DistribuirRendimiento(ctx, "p1", "p2", reparto.Nacional, "2026", "actor-1"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	nuevas, residuo, err := b.DistribuirRendimiento(ctx, "p1", "p2", reparto.Nacional, "2026", "actor-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	for _, n := range nuevas {
		if !n.Importe.IsZero() {
			t.Fatalf("segunda distribucion repartio %s a %s: el pool ya se habia consumido (B3)", n.Importe, n.TitularID)
		}
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero", residuo)
	}
}

// Dos vigencias sobre el mismo par comparten ref. La cifra explicada es la
// suma de lo pagado, no solo la vigencia mas reciente.
func TestDistribuirDosVigenciasExplicaLaSuma(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2025"): {Circuito: reparto.Nacional, Vigencia: "2025", Monto: decimal.RequireFromString("100.00")},
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("50.00")},
	}}
	libro := &bitacoraFalsa{}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Rendimientos: rendimientos}, libro)
	ctx := context.Background()

	primera, _, err := b.DistribuirRendimiento(ctx, "p1", "p2", reparto.Nacional, "2025", "actor-1")
	if err != nil {
		t.Fatalf("2025: %v", err)
	}
	segunda, _, err := b.DistribuirRendimiento(ctx, "p1", "p2", reparto.Nacional, "2026", "actor-1")
	if err != nil {
		t.Fatalf("2026: %v", err)
	}
	ref := FormarRefRendimiento("p1", "p2", primera[0].ObraID, primera[0].TitularID)
	if otra := FormarRefRendimiento("p1", "p2", segunda[0].ObraID, segunda[0].TitularID); otra != ref {
		t.Fatalf("refs %q y %q, el par origen-destino comparte ref", ref, otra)
	}
	for _, a := range libro.asientos {
		if a.RefID != "p2" {
			t.Fatalf("RefID = %q, el asiento cuelga de la corrida de destino", a.RefID)
		}
	}

	x, err := (ExplicarCifra{Bitacora: libro}).Explicar(ctx, Usuario{ID: "usr-aud", Rol: RolAuditor}, ref)
	if err != nil {
		t.Fatalf("explicar: %v", err)
	}
	esperado := primera[0].Importe.Add(segunda[0].Importe)
	if !x.Neto.Equal(esperado) {
		t.Fatalf("neto = %s, se esperaba %s (2025+2026)", x.Neto, esperado)
	}
}

func TestAbrirYFirmarReclamacionHastaSerPagable(t *testing.T) {
	t.Parallel()
	reclamaciones := &reclamacionesFalso{}
	b := BolsasAccesorias{Reclamaciones: reclamaciones}
	ctx := context.Background()

	r, err := b.AbrirReclamacion(ctx, "rec-1", "titular-1", "p1", "detalle", decimal.RequireFromString("30.00"), true, false)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if r.Pagable() {
		t.Fatal("recien abierta no deberia ser pagable")
	}

	_, err = b.FirmarReclamacion(ctx, "rec-1", reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	r, err = b.FirmarReclamacion(ctx, "rec-1", reparto.RolDistribucionYContabilidad, "actor-2")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !r.Pagable() {
		t.Fatal("con los dos avales deberia ser pagable")
	}
	if !reclamaciones.porID["rec-1"].Pagable() {
		t.Fatal("el estado pagable debe quedar persistido")
	}
}

func TestFirmarReclamacionPropagaElRechazoDelDominio(t *testing.T) {
	t.Parallel()
	reclamaciones := &reclamacionesFalso{}
	b := BolsasAccesorias{Reclamaciones: reclamaciones}
	ctx := context.Background()

	if _, err := b.AbrirReclamacion(ctx, "rec-1", "titular-1", "p1", "detalle", decimal.RequireFromString("30.00"), true, false); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if _, err := b.FirmarReclamacion(ctx, "rec-1", reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-unico"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// El mismo actor no puede cubrir los dos roles (domain invariant).
	if _, err := b.FirmarReclamacion(ctx, "rec-1", reparto.RolDistribucionYContabilidad, "actor-unico"); err == nil {
		t.Fatal("se esperaba que el caso de uso propague el rechazo del dominio")
	}
}

func TestLiberarReservaPrescritaNoLiberaSiElAsientoFalla(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Reservas: reservas}, &bitacoraFalsa{err: errors.New("bitacora caida")})
	b.Unidad = unidadQueRevierteReservas{reservas: reservas}

	if _, _, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p2", "2026", decimal.Zero, "actor-1"); err == nil {
		t.Fatal("se esperaba el fallo del asiento")
	}
	if !reservas.porProc["p1"].Saldo.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("saldo = %s, el asiento fallido no debio liberar la reserva", reservas.porProc["p1"].Saldo)
	}
}

func TestDistribuirRendimientoNoDebitaSiElAsientoFalla(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
	}}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Rendimientos: rendimientos}, &bitacoraFalsa{err: errors.New("bitacora caida")})
	b.Unidad = unidadQueRevierteRendimientos{rendimientos: rendimientos}

	if _, _, err := b.DistribuirRendimiento(context.Background(), "p1", "p2", reparto.Nacional, "2026", "actor-1"); err == nil {
		t.Fatal("se esperaba el fallo del asiento")
	}
	monto := rendimientos.porClave[claveRendimiento(reparto.Nacional, "2026")].Monto
	if !monto.Equal(decimal.RequireFromString("100.00")) {
		t.Fatalf("monto = %s, el asiento fallido no debio debitar el pool", monto)
	}
}

func TestLiberarReservaPrescritaRechazaLaMismaCorrida(t *testing.T) {
	t.Parallel()
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Saldo: decimal.RequireFromString("50.00")},
	}}
	b := BolsasAccesorias{Reservas: reservas}

	if _, _, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p1", "2026", decimal.Zero, "actor-1"); !errors.Is(err, reparto.ErrRepartoInvalido) {
		t.Fatalf("error = %v, se esperaba ErrRepartoInvalido", err)
	}
	if !reservas.porProc["p1"].Saldo.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("saldo = %s, el rechazo no debio tocar la reserva", reservas.porProc["p1"].Saldo)
	}
}

type corridasFalso struct {
	porID map[string]ProcesoVista
}

func (c *corridasFalso) ProcesoPorID(_ context.Context, id string) (ProcesoVista, error) {
	p, ok := c.porID[id]
	if !ok {
		return ProcesoVista{}, ErrNoEncontrado
	}
	return p, nil
}

func corridasDePrueba() *corridasFalso {
	return &corridasFalso{porID: map[string]ProcesoVista{
		"p1":    {ID: "p1", Periodo: "2026-01", Circuito: reparto.Nacional},
		"p2":    {ID: "p2", Periodo: "2027-01", Circuito: reparto.Nacional},
		"p-int": {ID: "p-int", Periodo: "2027-01", Circuito: reparto.Internacional},
	}}
}

type unidadDirecta struct{}

func (unidadDirecta) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type unidadQueRevierteReservas struct {
	reservas *reservasFalso
}

func (u unidadQueRevierteReservas) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	copia := map[string]reparto.PoolReserva{}
	for k, v := range u.reservas.porProc {
		copia[k] = v
	}
	err := fn(ctx)
	if err != nil {
		u.reservas.porProc = copia
	}
	return err
}

type unidadQueRevierteRendimientos struct {
	rendimientos *rendimientosFalso
}

func (u unidadQueRevierteRendimientos) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	copia := map[string]reparto.PoolRendimiento{}
	for k, v := range u.rendimientos.porClave {
		copia[k] = v
	}
	err := fn(ctx)
	if err != nil {
		u.rendimientos.porClave = copia
	}
	return err
}

func cablearBolsas(b BolsasAccesorias, libro *bitacoraFalsa) BolsasAccesorias {
	b.Corridas = corridasDePrueba()
	b.Bitacora = libro
	b.Unidad = unidadDirecta{}
	b.Reloj = relojFijo{instante: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	return b
}

func TestLiberarYDistribuirSinActorNoTocanNada(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
	}}
	libro := &bitacoraFalsa{}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Reservas: reservas, Rendimientos: rendimientos}, libro)
	ctx := context.Background()

	for _, actor := range []string{"", "   "} {
		if _, _, err := b.LiberarReservaPrescrita(ctx, "p1", "p2", "2026", decimal.Zero, actor); !errors.Is(err, ErrActorAusente) {
			t.Fatalf("liberar con actor %q: error = %v, se esperaba ErrActorAusente", actor, err)
		}
		if _, _, err := b.DistribuirRendimiento(ctx, "p1", "p2", reparto.Nacional, "2026", actor); !errors.Is(err, ErrActorAusente) {
			t.Fatalf("distribuir con actor %q: error = %v, se esperaba ErrActorAusente", actor, err)
		}
	}
	if !reservas.porProc["p1"].Saldo.Equal(decimal.RequireFromString("50.00")) {
		t.Fatalf("saldo = %s, sin actor no se libera", reservas.porProc["p1"].Saldo)
	}
	if monto := rendimientos.porClave[claveRendimiento(reparto.Nacional, "2026")].Monto; !monto.Equal(decimal.RequireFromString("100.00")) {
		t.Fatalf("monto = %s, sin actor no se debita", monto)
	}
	if len(libro.asientos) != 0 {
		t.Fatalf("asientos = %+v, sin actor no se asienta", libro.asientos)
	}
}

// RD 14.5.4: la reserva es solo nacional, asi que no se paga en una corrida internacional.
func TestLiberarReservaPrescritaRechazaUnDestinoInternacional(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	libro := &bitacoraFalsa{}
	b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Reservas: reservas}, libro)

	if _, _, err := b.LiberarReservaPrescrita(context.Background(), "p1", "p-int", "2026", decimal.Zero, "actor-1"); !errors.Is(err, reparto.ErrReservaInternacional) {
		t.Fatalf("error = %v, se esperaba ErrReservaInternacional (RD 14.5.4)", err)
	}
	if !reservas.porProc["p1"].Saldo.Equal(decimal.RequireFromString("50.00")) || len(libro.asientos) != 0 {
		t.Fatalf("saldo = %s, asientos = %d; el rechazo no debio liberar ni asentar", reservas.porProc["p1"].Saldo, len(libro.asientos))
	}
}

// RD 10.3: un rendimiento se reparte sobre una corrida de su circuito y se paga en otra del mismo.
func TestDistribuirRendimientoRechazaCruzarCircuitos(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre, origen, destino string
		circuito                reparto.Circuito
	}{
		{"internacional sobre origen nacional", "p1", "p-int", reparto.Internacional},
		{"nacional pagado en destino internacional", "p1", "p-int", reparto.Nacional},
		{"nacional sobre origen internacional", "p-int", "p2", reparto.Nacional},
	}
	for _, c := range casos {
		resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{c.origen: resultadoConDosTitulares()}}
		rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
			claveRendimiento(c.circuito, "2026"): {Circuito: c.circuito, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
		}}
		libro := &bitacoraFalsa{}
		b := cablearBolsas(BolsasAccesorias{Resultados: resultados, Rendimientos: rendimientos}, libro)

		if _, _, err := b.DistribuirRendimiento(context.Background(), c.origen, c.destino, c.circuito, "2026", "actor-1"); !errors.Is(err, reparto.ErrCircuitoCruzado) {
			t.Fatalf("%s: error = %v, se esperaba ErrCircuitoCruzado (RD 10.3)", c.nombre, err)
		}
		if monto := rendimientos.porClave[claveRendimiento(c.circuito, "2026")].Monto; !monto.Equal(decimal.RequireFromString("100.00")) || len(libro.asientos) != 0 {
			t.Fatalf("%s: monto = %s, asientos = %d; el rechazo no debio debitar ni asentar", c.nombre, monto, len(libro.asientos))
		}
	}
}
