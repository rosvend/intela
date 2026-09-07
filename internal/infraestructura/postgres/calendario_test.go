package postgres

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Cuatro entradas que cubren las cuatro combinaciones que decide Pendientes:
// vencida sin disparar, vencida ya disparada, futura, y el borde exacto.
func sembrarCalendario(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	store, pool := colaVacia(t)

	_, err := pool.Exec(t.Context(),
		`INSERT INTO calendario (periodo, fecha_apertura, disparado) VALUES
		   ('2024',    DATE '2024-12-01', FALSE),
		   ('2025',    DATE '2025-12-01', TRUE),
		   ('2026',    DATE '2026-12-01', FALSE),
		   ('2027',    DATE '2027-12-01', FALSE)`)
	if err != nil {
		t.Fatalf("sembrar el calendario: %v", err)
	}
	return store, pool
}

func TestPendientesSoloDevuelveLoVencidoYSinDisparar(t *testing.T) {
	store, _ := sembrarCalendario(t)

	// 2026-12-01: 2024 y 2026 estan vencidos; 2025 ya se disparo y 2027 aun no
	// llega.
	hoy := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	periodos, err := store.Pendientes(t.Context(), hoy)
	if err != nil {
		t.Fatalf("Pendientes: %v", err)
	}

	// El orden es por fecha de apertura: sin ORDER BY, dos pasadas del
	// scheduler podrian encolar los mismos periodos en distinto orden.
	if quiero := []string{"2024", "2026"}; !slices.Equal(periodos, quiero) {
		t.Fatalf("Pendientes = %v, se esperaba %v", periodos, quiero)
	}
}

// El dia exacto de apertura ya cuenta: la comparacion es <=, no <.
func TestPendientesIncluyeElDiaDeApertura(t *testing.T) {
	store, _ := sembrarCalendario(t)

	vispera := time.Date(2026, 11, 30, 23, 0, 0, 0, time.UTC)
	periodos, err := store.Pendientes(t.Context(), vispera)
	if err != nil {
		t.Fatalf("Pendientes: %v", err)
	}
	if quiero := []string{"2024"}; !slices.Equal(periodos, quiero) {
		t.Fatalf("la vispera devuelve %v, se esperaba %v", periodos, quiero)
	}
}

// La comparacion reduce el instante a fecha EN UTC, no en la zona del
// servidor. Es lo que hace que la consulta no dependa del `timezone` de la
// sesion, y de paso lo que adelanta el disparo cinco horas respecto a Bogota:
// queda escrito aqui para que el dia que el calendario tenga granularidad
// horaria esto salte como prueba y no como sorpresa.
func TestPendientesCompataEnUTCYNoEnLaZonaDelServidor(t *testing.T) {
	store, pool := sembrarCalendario(t)

	if _, err := pool.Exec(t.Context(), `SET TIME ZONE 'America/Bogota'`); err != nil {
		t.Fatalf("cambiar la zona de la sesion: %v", err)
	}

	// 2026-12-01T04:00Z son las 23:00 del 30 de noviembre en Bogota. En UTC ya
	// es el dia de apertura, asi que el periodo esta vencido.
	hoy := time.Date(2026, 12, 1, 4, 0, 0, 0, time.UTC)
	periodos, err := store.Pendientes(t.Context(), hoy)
	if err != nil {
		t.Fatalf("Pendientes: %v", err)
	}
	if !slices.Contains(periodos, "2026") {
		t.Fatalf("Pendientes = %v, se esperaba que 2026 estuviera vencido", periodos)
	}
}

func TestPendientesConElCalendarioVacioNoEsError(t *testing.T) {
	store, _ := colaVacia(t)

	periodos, err := store.Pendientes(t.Context(), time.Now())
	if err != nil {
		t.Fatalf("Pendientes: %v", err)
	}
	if len(periodos) != 0 {
		t.Fatalf("Pendientes = %v, se esperaba vacio", periodos)
	}
}

func TestMarcarDisparadoSacaElPeriodoDeLosPendientes(t *testing.T) {
	store, _ := sembrarCalendario(t)
	ctx := t.Context()
	hoy := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	if err := store.MarcarDisparado(ctx, "2026"); err != nil {
		t.Fatalf("MarcarDisparado: %v", err)
	}
	periodos, err := store.Pendientes(ctx, hoy)
	if err != nil {
		t.Fatalf("Pendientes: %v", err)
	}
	if slices.Contains(periodos, "2026") {
		t.Fatalf("2026 sigue pendiente: %v", periodos)
	}

	// Idempotente: marcar dos veces actualiza la misma fila al mismo valor.
	if err := store.MarcarDisparado(ctx, "2026"); err != nil {
		t.Fatalf("MarcarDisparado por segunda vez: %v", err)
	}
}

// Marcar un periodo que no esta en el calendario seria una fecha inventada por
// el codigo en vez de leida del dato que administra el Consejo Directivo
// (ADR 0004).
func TestMarcarDisparadoDeUnPeriodoAusenteEsErrNoEncontrado(t *testing.T) {
	store, _ := sembrarCalendario(t)

	if err := store.MarcarDisparado(t.Context(), "1999"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

// El scheduler completo contra un calendario sembrado: los dos puertos los
// satisface el mismo *Store, que es como lo cablea cmd/scheduler.
func TestPlanificadorEncolaDesdeElCalendarioYLoMarca(t *testing.T) {
	store, pool := sembrarCalendario(t)
	ctx := t.Context()

	planificador := aplicacion.Planificador{
		Calendario: store,
		Cola:       store,
		Reloj:      reloj.Fijo{Instante: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)},
	}

	encoladas, err := planificador.Disparar(ctx)
	if err != nil {
		t.Fatalf("Disparar: %v", err)
	}
	if len(encoladas) != 2 {
		t.Fatalf("encoladas = %v, se esperaban 2024 y 2026", encoladas)
	}

	if n := contarTrabajos(t, pool, `tipo = 'ejecutar_reparto' AND corrida = 1 AND estado = 'pendiente'`); n != 2 {
		t.Fatalf("hay %d trabajos encolados, se esperaban 2", n)
	}

	// Segunda pasada: el calendario ya no tiene nada vencido y no se encola
	// ningun duplicado.
	encoladas, err = planificador.Disparar(ctx)
	if err != nil {
		t.Fatalf("Disparar por segunda vez: %v", err)
	}
	if len(encoladas) != 0 {
		t.Fatalf("encoladas = %v, la segunda pasada no encola nada", encoladas)
	}
	if n := contarTrabajos(t, pool, `TRUE`); n != 2 {
		t.Fatalf("hay %d filas en la cola, se esperaban 2", n)
	}
}

// La reconciliacion: el periodo quedo encolado y sin marcar porque el proceso
// murio entre las dos operaciones. La pasada siguiente no duplica el trabajo y
// si termina de marcarlo.
func TestPlanificadorReconciliaUnPeriodoEncoladoYSinMarcar(t *testing.T) {
	store, pool := sembrarCalendario(t)
	ctx := t.Context()
	hoy := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	// Lo que habria dejado la pasada interrumpida.
	if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoEjecutarReparto, "2026", 1), nil); err != nil {
		t.Fatalf("Encolar: %v", err)
	}

	planificador := aplicacion.Planificador{
		Calendario: store, Cola: store, Reloj: reloj.Fijo{Instante: hoy},
	}
	encoladas, err := planificador.Disparar(ctx)
	if err != nil {
		t.Fatalf("Disparar: %v", err)
	}

	// Solo 2024 es nuevo; 2026 ya estaba.
	if len(encoladas) != 1 || encoladas[0].Periodo != "2024" {
		t.Fatalf("encoladas = %v, se esperaba solo 2024", encoladas)
	}
	if n := contarTrabajos(t, pool, `periodo = '2026'`); n != 1 {
		t.Fatalf("hay %d trabajos para 2026, se esperaba 1", n)
	}
	pendientes, err := store.Pendientes(ctx, hoy)
	if err != nil {
		t.Fatalf("Pendientes: %v", err)
	}
	if len(pendientes) != 0 {
		t.Fatalf("quedan periodos sin marcar: %v", pendientes)
	}
}
