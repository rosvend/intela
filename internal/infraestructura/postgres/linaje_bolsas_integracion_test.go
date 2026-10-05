package postgres

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// TestLiberarSinAsientoNoLiberaYExplicarNombraLasDosCorridas es el contrato
// de #177: la liberacion y reserva.liberada son un solo hecho, y explicar
// la cifra devuelve la corrida de origen y la de destino.
func TestLiberarSinAsientoNoLiberaYExplicarNombraLasDosCorridas(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

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
	if err := s.GuardarResultado(ctx, "proceso-1", reparto.Resultado{
		Reserva:    dec("50.00"),
		SnapshotID: "snap-2026",
		Reglamento: "IX",
		Obras: []reparto.LineaObra{
			{ObraID: "obra-1", Puntos: dec("100"), Importe: dec("50.00")},
		},
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("20.00")},
			{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("30.00")},
		},
	}); err != nil {
		t.Fatalf("congelar la corrida: %v", err)
	}

	b := aplicacion.BolsasAccesorias{
		Resultados: s,
		Reservas:   s,
		Corridas:   s,
		Bitacora:   bitacoraQueFalla{s},
		Unidad:     s,
		Reloj:      reloj.Sistema{},
	}
	if _, err := b.RegistrarReserva(ctx, "proceso-1", reparto.Nacional, dec("5")); err != nil {
		t.Fatalf("registrar reserva: %v", err)
	}
	if _, _, err := b.LiberarReservaPrescrita(ctx, "proceso-1", "proceso-2", "2026", dec("0.00"), "actor-1"); !errors.Is(err, errBitacoraCaida) {
		t.Fatalf("err = %v, se esperaba el fallo de la bitacora", err)
	}

	pool, err := s.ReservaPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("releer reserva: %v", err)
	}
	if !pool.Saldo.Equal(dec("50.00")) {
		t.Fatalf("saldo = %s, el asiento fallido no debio liberar la reserva", pool.Saldo)
	}
	var lineas int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM reservas_liberaciones WHERE proceso_id = 'proceso-1'`).Scan(&lineas); err != nil {
		t.Fatalf("contar liberaciones: %v", err)
	}
	if lineas != 0 {
		t.Fatalf("quedaron %d lineas liberadas sin asiento", lineas)
	}
	asientos, err := s.De(ctx, aplicacion.RefProceso, "proceso-2")
	if err != nil {
		t.Fatalf("leer bitacora: %v", err)
	}
	if len(asientos) != 0 {
		t.Fatalf("quedaron %d asientos tras el fallo", len(asientos))
	}

	b.Bitacora = s
	nuevas, _, err := b.LiberarReservaPrescrita(ctx, "proceso-1", "proceso-2", "2026", dec("0.00"), "actor-1")
	if err != nil {
		t.Fatalf("liberar: %v", err)
	}
	ref := aplicacion.FormarRefReservaLiberada("proceso-1", "proceso-2", nuevas[0].ObraID, nuevas[0].TitularID)
	x, err := (aplicacion.ExplicarCifra{Bitacora: s}).Explicar(ctx, aplicacion.Usuario{ID: "actor-1", Rol: aplicacion.RolAuditor}, ref)
	if err != nil {
		t.Fatalf("explicar %q: %v", ref, err)
	}
	if x.Origen == nil || x.Origen.ProcesoID != "proceso-1" || x.Origen.Periodo != "2026-01" {
		t.Fatalf("origen = %+v", x.Origen)
	}
	if x.Destino == nil || x.Destino.ProcesoID != "proceso-2" || x.Destino.Periodo != "2027-01" || x.Corrida.ProcesoID != "proceso-2" {
		t.Fatalf("destino = %+v, corrida = %+v", x.Destino, x.Corrida)
	}
	if !x.Neto.Equal(nuevas[0].Importe) {
		t.Fatalf("neto = %s, se esperaba %s", x.Neto, nuevas[0].Importe)
	}
	// La corrida se congelo con GuardarResultado, sin valorizar: faltantes lo nombra.
	if !slices.Contains(x.Faltantes, aplicacion.HechoRepartoValorizado) {
		t.Fatalf("faltantes = %v, la valorizacion de origen no esta asentada", x.Faltantes)
	}
	asientos, err = s.De(ctx, aplicacion.RefProceso, "proceso-2")
	if err != nil || len(asientos) != 1 {
		t.Fatalf("asientos del destino = %d, err = %v", len(asientos), err)
	}
	var desglose struct {
		SaldoReserva  string `json:"saldo_reserva"`
		SaldoRestante string `json:"saldo_restante"`
	}
	if err := json.Unmarshal(asientos[0].Payload, &desglose); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if desglose.SaldoReserva != "50.00" || desglose.SaldoRestante != "0.00" {
		t.Fatalf("desglose = %+v, se esperaba 50.00 liberado y 0.00 restante", desglose)
	}
}

// TestDistribuirSinAsientoNoDebitaYElAsientoCuelgaDelDestino es el gemelo de
// la liberacion: si rendimientos.distribuidos no se asienta, el ledger no
// baja, y el asiento que si se escribe cuelga de la corrida de destino.
func TestDistribuirSinAsientoNoDebitaYElAsientoCuelgaDelDestino(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

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
	if err := s.GuardarResultado(ctx, "proceso-1", reparto.Resultado{
		Reserva:    dec("0.00"),
		SnapshotID: "snap-2026",
		Reglamento: "IX",
		Obras: []reparto.LineaObra{
			{ObraID: "obra-1", Puntos: dec("100"), Importe: dec("100.00")},
		},
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("40.00")},
			{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("60.00")},
		},
	}); err != nil {
		t.Fatalf("congelar la corrida: %v", err)
	}
	if err := s.AcrecerRendimiento(ctx, reparto.Nacional, "2026", dec("100.00")); err != nil {
		t.Fatalf("acrecer rendimiento: %v", err)
	}

	b := aplicacion.BolsasAccesorias{
		Resultados:   s,
		Rendimientos: s,
		Corridas:     s,
		Bitacora:     bitacoraQueFalla{s},
		Unidad:       s,
		Reloj:        reloj.Sistema{},
	}
	if _, _, err := b.DistribuirRendimiento(ctx, "proceso-1", "proceso-2", reparto.Nacional, "2026", "actor-1"); !errors.Is(err, errBitacoraCaida) {
		t.Fatalf("err = %v, se esperaba el fallo de la bitacora", err)
	}

	rend, err := s.PorCircuitoYVigencia(ctx, reparto.Nacional, "2026")
	if err != nil {
		t.Fatalf("leer rendimiento: %v", err)
	}
	if !rend.Monto.Equal(dec("100.00")) {
		t.Fatalf("monto = %s, el asiento fallido no debio debitar el ledger", rend.Monto)
	}
	var lineas int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM rendimientos_distribuciones
		  WHERE circuito = 'nacional' AND vigencia = '2026' AND proceso_id = 'proceso-1'`,
	).Scan(&lineas); err != nil {
		t.Fatalf("contar distribuciones: %v", err)
	}
	if lineas != 0 {
		t.Fatalf("quedaron %d lineas distribuidas sin asiento", lineas)
	}

	b.Bitacora = s
	nuevas, _, err := b.DistribuirRendimiento(ctx, "proceso-1", "proceso-2", reparto.Nacional, "2026", "actor-1")
	if err != nil {
		t.Fatalf("distribuir: %v", err)
	}
	asientos, err := s.De(ctx, aplicacion.RefProceso, "proceso-2")
	if err != nil {
		t.Fatalf("leer bitacora del destino: %v", err)
	}
	if len(asientos) != 1 || asientos[0].Hecho != aplicacion.HechoRendimientosDistribuidos || asientos[0].RefID != "proceso-2" {
		t.Fatalf("asiento = %+v, tenia que colgar de proceso-2", asientos)
	}
	delOrigen, err := s.De(ctx, aplicacion.RefProceso, "proceso-1")
	if err != nil {
		t.Fatalf("leer bitacora del origen: %v", err)
	}
	for _, a := range delOrigen {
		if a.Hecho == aplicacion.HechoRendimientosDistribuidos {
			t.Fatalf("el asiento quedo colgado del origen: %+v", a)
		}
	}

	ref := aplicacion.FormarRefRendimiento("proceso-1", "proceso-2", nuevas[0].ObraID, nuevas[0].TitularID)
	x, err := (aplicacion.ExplicarCifra{Bitacora: s}).Explicar(ctx, aplicacion.Usuario{ID: "actor-1", Rol: aplicacion.RolAuditor}, ref)
	if err != nil {
		t.Fatalf("explicar %q: %v", ref, err)
	}
	if !x.Neto.Equal(nuevas[0].Importe) || x.Destino == nil || x.Destino.ProcesoID != "proceso-2" {
		t.Fatalf("explicacion neto=%s destino=%+v", x.Neto, x.Destino)
	}
}
