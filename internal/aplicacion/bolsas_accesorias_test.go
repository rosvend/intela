package aplicacion

import (
	"context"
	"errors"
	"testing"

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

type reservasFalso struct {
	guardadas []reparto.PoolReserva
	porProc   map[string]reparto.PoolReserva
	err       error
}

func (r *reservasFalso) GuardarReserva(_ context.Context, p reparto.PoolReserva) error {
	if r.err != nil {
		return r.err
	}
	r.guardadas = append(r.guardadas, p)
	if r.porProc == nil {
		r.porProc = map[string]reparto.PoolReserva{}
	}
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

type rendimientosFalso struct {
	guardados []reparto.PoolRendimiento
	porClave  map[string]reparto.PoolRendimiento
	err       error
}

func claveRendimiento(circuito reparto.Circuito, vigencia string) string {
	return string(circuito) + "|" + vigencia
}

func (r *rendimientosFalso) GuardarRendimiento(_ context.Context, p reparto.PoolRendimiento) error {
	if r.err != nil {
		return r.err
	}
	r.guardados = append(r.guardados, p)
	if r.porClave == nil {
		r.porClave = map[string]reparto.PoolRendimiento{}
	}
	r.porClave[claveRendimiento(p.Circuito, p.Vigencia)] = p
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
	if len(reservas.guardadas) != 0 {
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
	if len(reservas.guardadas) != 1 {
		t.Fatalf("se esperaba 1 reserva persistida, hubo %d", len(reservas.guardadas))
	}
}

func TestLiberarReservaPrescritaReplicaLasProporcionesDeLaCorridaOriginal(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	b := BolsasAccesorias{Resultados: resultados, Reservas: reservas}

	nuevas, residuo, err := b.LiberarReservaPrescrita(context.Background(), "p1", decimal.Zero)
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
}

func TestLiberarReservaPrescritaIncluyeElRendimientoAcumulado(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	reservas := &reservasFalso{porProc: map[string]reparto.PoolReserva{
		"p1": {ProcesoID: "p1", Circuito: reparto.Nacional, MontoInicial: decimal.RequireFromString("50.00"), Saldo: decimal.RequireFromString("50.00")},
	}}
	b := BolsasAccesorias{Resultados: resultados, Reservas: reservas}

	// RD 10.4: el rendimiento acumulado sobre la reserva se incluye al liberarla.
	nuevas, _, err := b.LiberarReservaPrescrita(context.Background(), "p1", decimal.RequireFromString("10.00"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	suma := nuevas[0].Importe.Add(nuevas[1].Importe)
	if !suma.Equal(decimal.RequireFromString("60.00")) {
		t.Fatalf("suma repartida = %s, se esperaba 60.00 (50.00 de saldo + 10.00 de rendimiento)", suma)
	}
}

func TestRegistrarRendimientoAbreElPoolLaPrimeraVez(t *testing.T) {
	t.Parallel()
	rendimientos := &rendimientosFalso{}
	b := BolsasAccesorias{Rendimientos: rendimientos}

	p, err := b.RegistrarRendimiento(context.Background(), reparto.Nacional, "2026", decimal.RequireFromString("100.00"))
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

	p, err := b.RegistrarRendimiento(context.Background(), reparto.Nacional, "2026", decimal.RequireFromString("50.00"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !p.Monto.Equal(decimal.RequireFromString("150.00")) {
		t.Fatalf("monto = %s, se esperaba 150.00 (acrecido)", p.Monto)
	}
}

func TestDistribuirRendimientoNoRevalorizaLaCorrida(t *testing.T) {
	t.Parallel()
	resultados := &resultadosFalso{porProceso: map[string]reparto.Resultado{"p1": resultadoConDosTitulares()}}
	rendimientos := &rendimientosFalso{porClave: map[string]reparto.PoolRendimiento{
		claveRendimiento(reparto.Nacional, "2026"): {Circuito: reparto.Nacional, Vigencia: "2026", Monto: decimal.RequireFromString("100.00")},
	}}
	b := BolsasAccesorias{Resultados: resultados, Rendimientos: rendimientos}

	nuevas, residuo, err := b.DistribuirRendimiento(context.Background(), "p1", reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !nuevas[0].Importe.Equal(decimal.RequireFromString("40.00")) || !nuevas[1].Importe.Equal(decimal.RequireFromString("60.00")) {
		t.Fatalf("importes = %s, %s; se esperaba 40/60 de 100.00", nuevas[0].Importe, nuevas[1].Importe)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero", residuo)
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

	r, err = b.FirmarReclamacion(ctx, "rec-1", reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-1")
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
