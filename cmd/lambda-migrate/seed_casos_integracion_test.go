package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/semilla"
)

// Produccion ya tiene el dataset sin la cola de identificacion: sembrar-dataset la agrega y repetirlo no duplica.
func TestSembrarDatasetCompletaLaColaDeIdentificacion(t *testing.T) {
	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)
	t.Setenv("OBJECT_DIR", t.TempDir())
	pool, err := pgxpool.New(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := atender(mudo())(t.Context(), peticion{Orden: ordenSembrarDataset}); err != nil {
		t.Fatalf("sembrar-dataset: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM reportes WHERE periodo = $1`, semilla.PeriodoCasos); err != nil {
		t.Fatalf("simular una base sembrada antes de la cola: %v", err)
	}

	for i := range 2 {
		r, err := atender(mudo())(t.Context(), peticion{Orden: ordenSembrarDataset})
		if err != nil {
			t.Fatalf("invocacion %d: %v", i, err)
		}
		if r.Estado != "ya sembrado" {
			t.Fatalf("invocacion %d: estado %q, se esperaba \"ya sembrado\"", i, r.Estado)
		}
		var pendientes int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM usos u JOIN reportes r ON r.id = u.reporte_id
			WHERE r.periodo = $1 AND u.escalon = 'oni'`, semilla.PeriodoCasos).Scan(&pendientes); err != nil {
			t.Fatal(err)
		}
		if pendientes != 2 {
			t.Fatalf("invocacion %d: %d casos pendientes, se esperaban 2", i, pendientes)
		}
	}
}
