package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/semilla"
)

func baseDeCasos(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cadena := testhelp.DSN(t)
	t.Setenv("DATABASE_URL", cadena)
	t.Setenv("OBJECT_DIR", t.TempDir())
	pool, err := pgxpool.New(t.Context(), cadena)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func invocar(t *testing.T, p peticion) (respuesta, error) {
	t.Helper()
	return atender(mudo())(t.Context(), p)
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// colaDeCasos cuenta lo que ve la bandeja en PeriodoCasos: ONI pendientes, cuantas con candidatos, y usos sin procesar.
func colaDeCasos(t *testing.T, pool *pgxpool.Pool) (oni, conCandidatos, pendientes int) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FILTER (WHERE u.escalon = 'oni'),
		       count(*) FILTER (WHERE u.escalon = 'oni' AND EXISTS (SELECT 1 FROM candidatos_match c WHERE c.uso_id = u.id)),
		       count(*) FILTER (WHERE u.escalon = 'pendiente')
		  FROM usos u JOIN reportes r ON r.id = u.reporte_id
		 WHERE r.periodo = $1 AND r.fuente = 'caracol' AND u.reporte_id NOT LIKE 'rep-ajeno%'`,
		semilla.PeriodoCasos).Scan(&oni, &conCandidatos, &pendientes); err != nil {
		t.Fatal(err)
	}
	return oni, conCandidatos, pendientes
}

func exigirCola(t *testing.T, pool *pgxpool.Pool, cuando string) {
	t.Helper()
	if oni, con, _ := colaDeCasos(t, pool); oni != 2 || con != 1 {
		t.Fatalf("%s: %d casos pendientes (%d con candidato); se esperaban 2 (1 con candidato)", cuando, oni, con)
	}
}

// sinCola deja la base como la dejo el seed anterior a la cola de identificacion.
func sinCola(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	exec(t, pool, `DELETE FROM reportes WHERE periodo = $1`, semilla.PeriodoCasos)
}

// B1: la produccion documentada (#155, #157) tiene obras ajenas y se sembro con aditivo; la orden del runbook completa la cola.
func TestSembrarDatasetCompletaLaColaConObrasAjenas(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		ajenas  int
		aditivo bool
	}{
		{"control_solo_dataset", 0, false},
		{"produccion_con_100_obras_ajenas", 100, true},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			pool := baseDeCasos(t)
			for i := range c.ajenas {
				exec(t, pool, `INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ($1, $2, 'Drama', 2020, 'unitario')`,
					"obra-demo-"+string(rune('a'+i/26))+string(rune('a'+i%26)), "Demo ajena")
			}
			if _, err := invocar(t, peticion{Orden: ordenSembrarDataset, Aditivo: c.aditivo}); err != nil {
				t.Fatalf("siembra inicial: %v", err)
			}
			sinCola(t, pool)

			for i := range 2 {
				r, err := invocar(t, peticion{Orden: ordenSembrarDataset})
				if err != nil {
					t.Fatalf("invocacion %d: %v", i, err)
				}
				if r.Estado != "ya sembrado" && r.Estado != "cargado" {
					t.Fatalf("invocacion %d: estado %q", i, r.Estado)
				}
				exigirCola(t, pool, "invocacion "+string(rune('0'+i)))
			}
			var ajenas int
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM obras WHERE id LIKE 'obra-demo-%'`).Scan(&ajenas); err != nil {
				t.Fatal(err)
			}
			if ajenas != c.ajenas {
				t.Fatalf("obras ajenas = %d, se esperaban %d intactas", ajenas, c.ajenas)
			}
		})
	}
}

// B2: un fallo al escribir los usos de la cola no deja un acuse sin filas; el reintento la completa.
func TestSembrarDatasetTrasUnFalloAlEscribirLosUsosDeLaCola(t *testing.T) {
	pool := baseDeCasos(t)
	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err != nil {
		t.Fatalf("siembra inicial: %v", err)
	}
	sinCola(t, pool)
	exec(t, pool, `CREATE FUNCTION falla_usos_casos() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF (SELECT periodo FROM reportes WHERE id = NEW.reporte_id) = '`+semilla.PeriodoCasos+`' THEN
		    RAISE EXCEPTION 'fallo inyectado al escribir los usos de la cola';
		  END IF;
		  RETURN NEW;
		END $$`)
	exec(t, pool, `CREATE TRIGGER falla_usos_casos BEFORE INSERT ON usos FOR EACH ROW EXECUTE FUNCTION falla_usos_casos()`)

	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err == nil {
		t.Fatal("con el fallo inyectado la siembra tenia que fallar")
	}
	if oni, _, pend := colaDeCasos(t, pool); oni+pend != 0 {
		t.Fatalf("precondicion: tras el fallo no hay usos de la cola, hay %d", oni+pend)
	}
	exec(t, pool, `DROP TRIGGER falla_usos_casos ON usos`)

	for i := range 2 {
		if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err != nil {
			t.Fatalf("reintento %d: %v", i, err)
		}
		exigirCola(t, pool, "reintento "+string(rune('0'+i)))
	}
}

// B3: una cascada interrumpida deja los usos pendientes; el reintento sin reset la termina aunque la bitacora no este vacia.
func TestSembrarDatasetTrasUnaCascadaInterrumpida(t *testing.T) {
	pool := baseDeCasos(t)
	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err != nil {
		t.Fatalf("siembra inicial: %v", err)
	}
	sinCola(t, pool)
	exec(t, pool, `INSERT INTO asientos (hecho, ref_tipo, ref_id, payload, cuando) VALUES ('proceso.abierto', 'proceso', 'proc-x', '{}', now())`)
	exec(t, pool, `CREATE FUNCTION falla_cascada_casos() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		  IF OLD.escalon = 'pendiente' AND (SELECT periodo FROM reportes WHERE id = OLD.reporte_id) = '`+semilla.PeriodoCasos+`' THEN
		    RAISE EXCEPTION 'fallo inyectado en la cascada de la cola';
		  END IF;
		  RETURN NEW;
		END $$`)
	exec(t, pool, `CREATE TRIGGER falla_cascada_casos BEFORE UPDATE ON usos FOR EACH ROW EXECUTE FUNCTION falla_cascada_casos()`)

	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err == nil {
		t.Fatal("con la cascada interrumpida la siembra tenia que fallar")
	}
	if _, _, pend := colaDeCasos(t, pool); pend != 2 {
		t.Fatalf("precondicion: 2 usos de la cola pendientes, hay %d", pend)
	}
	exec(t, pool, `DROP TRIGGER falla_cascada_casos ON usos`)

	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err != nil {
		t.Fatalf("reintento sin reset: %v", err)
	}
	exigirCola(t, pool, "reintento sin reset")
}

// N1: la cascada de la cola solo toca sus propias entregas; una fila ajena del mismo periodo queda como estaba.
func TestSembrarDatasetNoTocaUsosAjenosDelPeriodoDeLaCola(t *testing.T) {
	pool := baseDeCasos(t)
	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err != nil {
		t.Fatalf("siembra inicial: %v", err)
	}
	sinCola(t, pool)
	exec(t, pool, `INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes) VALUES ('rep-ajeno-1', 'caracol', $1, $2, 'reportes/ajeno', 1)`,
		semilla.PeriodoCasos, strings.Repeat("a", 64))
	exec(t, pool, `INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, modalidad, emisiones) VALUES ('rep-ajeno-1-0', 'rep-ajeno-1', 'caracol', 'Pelicula X', 'id_ficha=PX-1', 'tv', 1)`)

	if _, err := invocar(t, peticion{Orden: ordenSembrarDataset}); err != nil {
		t.Fatalf("sembrar-dataset: %v", err)
	}
	exigirCola(t, pool, "tras sembrar")
	var escalon string
	if err := pool.QueryRow(t.Context(), `SELECT escalon FROM usos WHERE id = 'rep-ajeno-1-0'`).Scan(&escalon); err != nil {
		t.Fatal(err)
	}
	if escalon != "pendiente" {
		t.Fatalf("el uso ajeno paso a %q; la cascada de la cola no puede tocarlo", escalon)
	}
}
