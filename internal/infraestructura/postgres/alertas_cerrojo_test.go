package postgres

import (
	"context"
	"errors"
	"sync/atomic"
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

// relojQueAvanza da un instante un segundo mayor en cada lectura: el orden de las lecturas queda en la bitacora.
type relojQueAvanza struct {
	base time.Time
	n    atomic.Int64
}

func (r *relojQueAvanza) Ahora() time.Time {
	return r.base.Add(time.Duration(r.n.Add(1)) * time.Second)
}

// La pasada que espera el cerrojo lee el reloj DESPUES de tomarlo: si reabre una alerta que la pasada
// ganadora autocerro, el asiento de reapertura queda despues del de autocierre (MENOR 1, verificacion 3).
func TestReaperturaTrasEsperarElCerrojoQuedaDespuesDelAutocierre(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()
	anomalias := servicioDeAnomalias(s, time.Time{})
	anomalias.Reloj = &relojQueAvanza{base: instanteAlertas}

	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ('rep-caracol-enero-bis', 'caracol', '2026-01', repeat('c', 64), 'reportes/c.csv', 64)`); err != nil {
		t.Fatalf("sembrar la segunda entrega: %v", err)
	}
	insertarUsoDeAlertas(t, pool, "u-dup1", reporteEnero, "alias", "obra-y", "serie", "id_ficha=7", "2026-01-02", "20:00:00")
	const segundoUso = `INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, obra_id, escalon, oni,
	                                      modalidad, tipo_obra, fecha, hora, emisiones)
	 VALUES ('u-dup2', 'rep-caracol-enero-bis', 'caracol', 'Titulo de prueba', 'id_ficha=7', 'obra-y',
	         'alias', FALSE, 'tv', 'serie', '2026-01-02', '20:00:00', 1)`
	if _, err := pool.Exec(ctx, segundoUso); err != nil {
		t.Fatalf("sembrar el duplicado: %v", err)
	}
	if r, err := anomalias.Evaluar(ctx, "2026-01", ""); err != nil || r.CriticasAbiertas == 0 {
		t.Fatalf("pasada inicial: %+v, %v; se esperaba el duplicado abierto", r, err)
	}

	type resultado struct {
		r   aplicacion.ResumenEvaluacion
		err error
	}
	deA := make(chan resultado, 1)
	// Pasada B: gana el cerrojo, ve el duplicado ya ido y lo autocierra; la ingesta lo trae de vuelta antes de soltar.
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
		if _, err := tx.Exec(ctx, `DELETE FROM usos WHERE id = 'u-dup2'`); err != nil {
			return err
		}
		r, err := anomalias.Evaluar(ctx, "2026-01", "")
		if err != nil {
			return err
		}
		if r.Autocerradas == 0 {
			t.Errorf("la pasada B no autocerro el duplicado: %+v", r)
		}
		_, err = tx.Exec(ctx, segundoUso)
		return err
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

	filas, err := pool.Query(ctx,
		`SELECT id::text FROM alertas WHERE periodo = '2026-01' AND tipo = 'duplicado_registro' AND NOT resuelta`)
	if err != nil {
		t.Fatalf("leer duplicados abiertos: %v", err)
	}
	ids, err := pgx.CollectRows(filas, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("leer duplicados abiertos: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("la pasada A no reabrio el duplicado")
	}
	for _, id := range ids {
		asientos, err := s.De(ctx, aplicacion.RefAlerta, id)
		if err != nil {
			t.Fatalf("bitacora de %s: %v", id, err)
		}
		if len(asientos) == 0 {
			t.Fatalf("alerta %s reabierta sin asientos", id)
		}
		if ultimo := asientos[len(asientos)-1]; ultimo.Hecho != aplicacion.HechoAlertaReabierta {
			t.Errorf("alerta %s abierta pero su bitacora termina en %q (cuando %s)", id, ultimo.Hecho, ultimo.Cuando)
		}
	}
}

// unidadConEpilogo corre `despues` al volver de la unidad, ya confirmada: es otra pasada que llega justo detras.
type unidadConEpilogo struct {
	*Store
	despues func()
}

func (u unidadConEpilogo) EnUnidad(ctx context.Context, fn func(ctx context.Context) error) error {
	err := u.Store.EnUnidad(ctx, fn)
	if err == nil {
		u.despues()
	}
	return err
}

// El conteo de criticas del resumen sale de la foto de la pasada, no de lo que otra hizo al soltar el cerrojo (NIT 1).
func TestCriticasDelResumenSeCuentanDentroDeLaUnidad(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()
	anomalias := servicioDeAnomalias(s, instanteAlertas)

	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ('rep-caracol-enero-bis', 'caracol', '2026-01', repeat('c', 64), 'reportes/c.csv', 64)`); err != nil {
		t.Fatalf("sembrar la segunda entrega: %v", err)
	}
	insertarUsoDeAlertas(t, pool, "u-dup1", reporteEnero, "alias", "obra-y", "serie", "id_ficha=7", "2026-01-02", "20:00:00")
	insertarUsoDeAlertas(t, pool, "u-dup2", "rep-caracol-enero-bis", "alias", "obra-y", "serie", "id_ficha=7", "2026-01-02", "20:00:00")

	anomalias.Unidad = unidadConEpilogo{Store: s, despues: func() {
		if _, err := pool.Exec(context.Background(),
			`UPDATE alertas SET resuelta = TRUE, autocerrada = TRUE, resuelta_en = now(), nota = 'otra pasada'
			  WHERE periodo = '2026-01' AND NOT resuelta`); err != nil {
			t.Errorf("epilogo: %v", err)
		}
	}}
	r, err := anomalias.Evaluar(ctx, "2026-01", "")
	if err != nil {
		t.Fatalf("evaluar: %v", err)
	}
	if r.CriticasAbiertas == 0 {
		t.Fatalf("criticas abiertas = 0 con el duplicado detectado en la pasada (%+v): se contaron fuera de la unidad", r)
	}
}

// Fuera de una unidad el cerrojo se soltaria al volver de la sentencia: tiene que fallar, no proteger en falso (MENOR 2).
func TestBloquearAlertasDePeriodoFueraDeUnidadFalla(t *testing.T) {
	s, _ := sembrarProcesoNacionalListoParaValorizar(t)
	err := s.BloquearAlertasDePeriodo(t.Context(), "2026-01")
	if !errors.Is(err, errFueraDeUnidad) {
		t.Fatalf("BloquearAlertasDePeriodo sin unidad = %v; se esperaba errFueraDeUnidad", err)
	}
}
