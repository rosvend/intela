package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

// esperarCerrojoDeAvisoEnEspera espera a que alguna sesion quede bloqueada en un cerrojo de aviso.
// Usa una conexion propia: el pool de pruebas es chico y las dos pasadas ya ocupan las suyas.
func esperarCerrojoDeAvisoEnEspera(t *testing.T, s *Store) {
	t.Helper()
	conn, err := pgx.ConnectConfig(t.Context(), s.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("conexion de observacion: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	limite := time.Now().Add(15 * time.Second)
	for time.Now().Before(limite) {
		var n int
		if err := conn.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&n); err != nil {
			t.Fatalf("leer pg_locks: %v", err)
		}
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("la evaluacion concurrente nunca quedo esperando el cerrojo del periodo")
}

// Dos evaluaciones del mismo periodo se serializan y la segunda arma su foto despues del cerrojo:
// no autocierra la critica que la primera guardo mientras ella esperaba (MENOR 1, verificacion 2).
func TestEvaluacionesConcurrentesNoAutocierranUnaAlertaVigente(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()
	anomalias := servicioDeAnomalias(s, time.Now())

	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ('rep-caracol-enero-bis', 'caracol', '2026-01', repeat('c', 64), 'reportes/c.csv', 64)`); err != nil {
		t.Fatalf("sembrar la segunda entrega: %v", err)
	}
	insertarUsoDeAlertas(t, pool, "u-dup1", reporteEnero, "alias", "obra-y", "serie", "id_ficha=7", "2026-01-02", "20:00:00")

	// Pasada B: toma el cerrojo, recibe el duplicado y lo evalua mientras A espera.
	type resultado struct {
		r   aplicacion.ResumenEvaluacion
		err error
	}
	deA := make(chan resultado, 1)
	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		if err := s.BloquearAlertasDePeriodo(ctx, "2026-01"); err != nil {
			return err
		}
		go func() {
			r, err := anomalias.Evaluar(t.Context(), "2026-01", "")
			deA <- resultado{r, err}
		}()
		esperarCerrojoDeAvisoEnEspera(t, s)

		tx, _ := txDe(ctx)
		if _, err := tx.Exec(ctx,
			`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, obra_id, escalon, oni,
			                   modalidad, tipo_obra, fecha, hora, emisiones)
			 VALUES ('u-dup2', 'rep-caracol-enero-bis', 'caracol', 'Titulo de prueba', 'id_ficha=7', 'obra-y',
			         'alias', FALSE, 'tv', 'serie', '2026-01-02', '20:00:00', 1)`); err != nil {
			return err
		}
		r, err := anomalias.Evaluar(ctx, "2026-01", "")
		if err != nil {
			return err
		}
		if r.CriticasAbiertas == 0 {
			t.Errorf("la pasada B no detecto el duplicado: %+v", r)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pasada B: %v", err)
	}

	var a resultado
	select {
	case a = <-deA:
	case <-time.After(30 * time.Second):
		t.Fatal("la pasada A no termino tras soltar el cerrojo")
	}
	if a.err != nil {
		t.Fatalf("pasada A: %v", a.err)
	}
	if a.r.Autocerradas != 0 || a.r.CriticasAbiertas == 0 {
		t.Fatalf("pasada A: autocerradas = %d, criticas = %d; autocerro con una foto vieja", a.r.Autocerradas, a.r.CriticasAbiertas)
	}

	var abiertas, autocierres int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM alertas WHERE periodo = '2026-01' AND tipo = 'duplicado_registro' AND NOT resuelta`).Scan(&abiertas); err != nil {
		t.Fatalf("contar duplicados abiertos: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM asientos WHERE hecho = $1`, aplicacion.HechoAlertaAutocerrada).Scan(&autocierres); err != nil {
		t.Fatalf("contar autocierres: %v", err)
	}
	if abiertas == 0 || autocierres != 0 {
		t.Fatalf("duplicado abierto = %d, asientos de autocierre = %d; se esperaba >0 y 0", abiertas, autocierres)
	}
}
