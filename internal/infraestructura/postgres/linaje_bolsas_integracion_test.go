package postgres

import (
	"errors"
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
}
