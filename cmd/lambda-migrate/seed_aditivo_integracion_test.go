package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// El cableado completo del caso que motivo #155: sembrar-dataset con
// aditivo:true escribe encima de obras ajenas que reset (o el camino normal)
// rechazaria.
func TestSembrarDatasetAditivoDeExtremoAExtremo(t *testing.T) {
	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)
	t.Setenv("OBJECT_DIR", t.TempDir())

	pool, err := pgxpool.New(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir pool de escritura: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(t.Context(), `
		INSERT INTO obras (id, titulo, genero, anio, tipo)
		VALUES ('obra-demo-001', 'Ajena', 'Drama', 2020, 'unitario')`,
	); err != nil {
		t.Fatalf("insertar obra ajena: %v", err)
	}

	r, err := atender(mudo())(t.Context(), peticion{Orden: ordenSembrarDataset, Aditivo: true})
	if err != nil {
		t.Fatalf("sembrar-dataset aditivo: %v", err)
	}
	if r.Estado != "cargado" {
		t.Fatalf("estado = %q, se esperaba \"cargado\"", r.Estado)
	}

	var nTitulares int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM titulares`).Scan(&nTitulares); err != nil {
		t.Fatalf("contar titulares: %v", err)
	}
	if nTitulares == 0 {
		t.Fatal("titulares sigue en cero tras sembrar-dataset aditivo")
	}
}
