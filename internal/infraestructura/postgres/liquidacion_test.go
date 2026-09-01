package postgres

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestDeTitularDevuelveNetoYTotalesDelProceso(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularAna, "2026-01")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, se esperaba 1", len(filas))
	}
	f := filas[0]
	if f.ObraID != obraCompleta || f.Periodo != "2026-01" {
		t.Fatalf("fila = %+v", f)
	}
	if !f.Neto.Equal(decimal.RequireFromString("3900")) {
		t.Fatalf("neto = %s", f.Neto)
	}
	if !f.ProcesoAdmin.Equal(decimal.RequireFromString("2000")) {
		t.Fatalf("admin proceso = %s", f.ProcesoAdmin)
	}
}

func TestDeTitularRespetaElPeriodo(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularAna, "2025-06")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 0 {
		t.Fatalf("un periodo sin corridas no es error, es lista vacia: %+v", filas)
	}
}

func TestDeTitularSinFiltroTraeTodosLosPeriodos(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)
	sembrarOtraCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularAna, "")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2 periodos", len(filas))
	}
}

func TestDeTitularNoMezclaTitulares(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularBeto, "2026-01")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("beto tiene su propia linea: %d", len(filas))
	}
	if !filas[0].Neto.Equal(decimal.RequireFromString("2600")) {
		t.Fatalf("neto de beto = %s", filas[0].Neto)
	}
}

func TestDeTitularSinCorridasEsListaVacia(t *testing.T) {
	s, _ := sembrar(t)

	filas, err := s.DeTitular(t.Context(), titularAna, "")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if filas != nil && len(filas) != 0 {
		t.Fatalf("sin corridas: %+v", filas)
	}
}

func sembrarCorrida(t *testing.T, s *Store) {
	t.Helper()
	sembrarProceso(t, s, "bolsa-1", "proc-1", "2026-01",
		"10000", "2000", "1000", "500", "6500", "3900", "2600")
}

func sembrarOtraCorrida(t *testing.T, s *Store) {
	t.Helper()
	sembrarProceso(t, s, "bolsa-2", "proc-2", "2026-02",
		"2000", "400", "200", "100", "1300", "780", "520")
}

func sembrarProceso(t *testing.T, s *Store, bolsaID, procesoID, periodo, bruto, admin, social, reserva, neto, anaNeto, betoNeto string) {
	t.Helper()
	ctx := t.Context()
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar proceso (%s): %v", sql, err)
		}
	}

	ejecutar(`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
	          VALUES ($1, 'usr-canal', $2, 'nacional', $3)`,
		bolsaID, periodo, bruto)
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
	          VALUES ($1, 'nacional', 'liquidacion_final', $2, $3, 'snap-1', 'RD IX')`,
		procesoID, periodo, bolsaID)
	ejecutar(`INSERT INTO resultados_proceso
	            (proceso_id, bruto, admin, social, reserva, neto, snapshot_id, reglamento)
	          VALUES ($1, $2, $3, $4, $5, $6, 'snap-1', 'RD IX')`,
		procesoID, bruto, admin, social, reserva, neto)
	ejecutar(`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
	          VALUES ($1, $2, 100, $3, FALSE)`,
		procesoID, obraCompleta, neto)
	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
	          VALUES ($1, $2, $3, 'IPI-00000001', 60, $4),
	                 ($1, $2, $5, 'IPI-00000002', 40, $6)`,
		procesoID, obraCompleta, titularAna, anaNeto, titularBeto, betoNeto)
}
