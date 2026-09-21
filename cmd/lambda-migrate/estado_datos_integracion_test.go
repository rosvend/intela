package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// Contra PostgreSQL de verdad: una base vacia cuenta cero en todo, y tras
// insertar filas sueltas (sin pasar por semilla.Cargar) los conteos las ven.
// Es justo el caso que motiva la orden: una base con datos que no son del
// dataset sintetico, y hace falta saber cuantos hay antes de decidir algo.
func TestEstadoDatosDeExtremoAExtremo(t *testing.T) {
	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)

	r, err := atender(mudo())(t.Context(), peticion{Orden: ordenEstadoDatos})
	if err != nil {
		t.Fatalf("base vacia: %v", err)
	}
	if r.Estado != "contado" {
		t.Fatalf("estado = %q, se esperaba \"contado\"", r.Estado)
	}
	for _, tabla := range tablasEstadoDatos {
		if r.Conteos[tabla] != 0 {
			t.Errorf("conteos[%q] = %d, se esperaba 0 en base vacia", tabla, r.Conteos[tabla])
		}
	}

	pool, err := pgxpool.New(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir pool de escritura: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO obras (id, titulo, genero, anio, tipo)
		 VALUES ('obra-ajena-001', 'Ajena', 'Drama', 2020, 'unitario')`); err != nil {
		t.Fatalf("sembrar obra ajena: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO titulares (id, nombre, ipi, clase) VALUES ('tit-1', 'Titular Uno', '111', 'socio')`); err != nil {
		t.Fatalf("sembrar titular: %v", err)
	}

	r2, err := atender(mudo())(t.Context(), peticion{Orden: ordenEstadoDatos})
	if err != nil {
		t.Fatalf("tras insertar: %v", err)
	}
	if r2.Conteos["obras"] != 1 {
		t.Errorf("conteos[obras] = %d, se esperaba 1", r2.Conteos["obras"])
	}
	if r2.Conteos["titulares"] != 1 {
		t.Errorf("conteos[titulares] = %d, se esperaba 1", r2.Conteos["titulares"])
	}
	if r2.Conteos["declaraciones"] != 0 {
		t.Errorf("conteos[declaraciones] = %d, se esperaba 0: nada las inserto", r2.Conteos["declaraciones"])
	}
}
