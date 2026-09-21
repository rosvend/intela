package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// sembrarCorridaBase deja una bolsa, un proceso, dos obras y dos titulares
// listos para que resultados_obra/resultados_titular referencien por FK.
func sembrarCorridaBase(t *testing.T) *Store {
	t.Helper()
	pool := testhelp.Pool(t)
	s := &Store{pool: pool}
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('usuario-1', 'Usuario 1', 'tv_abierta')`); err != nil {
		t.Fatalf("sembrar usuario_recaudo: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
		 VALUES ('bolsa-1', 'usuario-1', '2026-01', 'nacional', 1000.00)`); err != nil {
		t.Fatalf("sembrar bolsa: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
		 VALUES ('proceso-1', 'nacional', 'importe_titular', '2026-01', 'bolsa-1', 'snap-1', 'IX')`); err != nil {
		t.Fatalf("sembrar proceso: %v", err)
	}
	for _, obraID := range []string{"obra-1", "obra-2"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ($1, $2, 'Drama', 2026, 'serie')`,
			obraID, "Obra "+obraID); err != nil {
			t.Fatalf("sembrar %q: %v", obraID, err)
		}
	}
	for _, tt := range []struct{ id, ipi string }{
		{"titular-a", "111"},
		{"titular-b", "222"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO titulares (id, nombre, ipi, clase) VALUES ($1, $2, $3, 'socio')`,
			tt.id, "Titular "+tt.id, tt.ipi); err != nil {
			t.Fatalf("sembrar %q: %v", tt.id, err)
		}
	}
	return s
}

func resultadoDeCorridaBase() reparto.Resultado {
	return reparto.Resultado{
		Neto:       dec("850.00"),
		Admin:      dec("100.00"),
		Social:     dec("50.00"),
		Reserva:    dec("0.00"),
		Retenido:   dec("0.00"),
		Residuo:    dec("0.01"),
		ValorPunto: dec("1.5"),
		SnapshotID: "snap-1",
		Reglamento: "IX",
		Obras: []reparto.LineaObra{
			{ObraID: "obra-1", Puntos: dec("300"), Importe: dec("400.00")},
			{ObraID: "obra-2", Puntos: dec("225"), Importe: dec("300.00")},
		},
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("160.00")},
			{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("240.00")},
			{ObraID: "obra-2", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("300.00")},
		},
	}
}

// TestResultadosGuardarYPorProcesoReproducenLasProporciones es el caso que
// #121 necesita de verdad: releer de Postgres las lineas de titular de una
// corrida ya cerrada, sin perder la proporcion que el replay va a usar
// (RD 14.4, RD 10.1).
func TestResultadosGuardarYPorProcesoReproducenLasProporciones(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()
	r := resultadoDeCorridaBase()

	if err := s.GuardarResultado(ctx, "proceso-1", r); err != nil {
		t.Fatalf("guardar resultado: %v", err)
	}

	leido, err := s.ResultadoPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer resultado: %v", err)
	}

	if !leido.Neto.Equal(r.Neto) || !leido.Admin.Equal(r.Admin) || !leido.Social.Equal(r.Social) ||
		!leido.Reserva.Equal(r.Reserva) || !leido.Retenido.Equal(r.Retenido) || !leido.Residuo.Equal(r.Residuo) ||
		!leido.ValorPunto.Equal(r.ValorPunto) {
		t.Fatalf("los agregados no cuadran: leido=%+v, original=%+v", leido, r)
	}
	if leido.SnapshotID != r.SnapshotID || leido.Reglamento != r.Reglamento {
		t.Fatalf("procedencia perdida: leido=%+v", leido)
	}

	if len(leido.Obras) != 2 {
		t.Fatalf("se esperaban 2 lineas de obra, hubo %d", len(leido.Obras))
	}
	if len(leido.Titulares) != 3 {
		t.Fatalf("se esperaban 3 lineas de titular, hubo %d", len(leido.Titulares))
	}
	// La proporcion que el replay va a usar es el Importe original: tiene que
	// sobrevivir exacto al round-trip.
	for i, orig := range r.Titulares {
		got := leido.Titulares[i]
		if got.ObraID != orig.ObraID || got.TitularID != orig.TitularID || !got.Importe.Equal(orig.Importe) {
			t.Fatalf("linea %d cambio: leido=%+v, original=%+v", i, got, orig)
		}
	}
}

func TestResultadosPorProcesoSinCorridaEsNoEncontrado(t *testing.T) {
	s := sembrarCorridaBase(t)
	_, err := s.ResultadoPorProceso(t.Context(), "proceso-que-no-existe")
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}
