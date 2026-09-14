package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// La cola no necesita el juego de datos de semilla_test.go: no referencia
// titulares, obras ni usuarios. Una base recien restaurada basta.
func colaVacia(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool := testhelp.Pool(t)
	return &Store{pool: pool}, pool
}

func clave(tipo aplicacion.TipoTrabajo, periodo string, corrida int) aplicacion.ClaveTrabajo {
	return aplicacion.ClaveTrabajo{Tipo: tipo, Periodo: periodo, Corrida: corrida}
}

// filaTrabajo lee el estado crudo de una fila. Las pruebas comprueban contra
// la tabla y no contra lo que devuelve el propio adaptador: si Cerrar no
// escribiera nada, una comprobacion hecha sobre su valor de retorno pasaria.
type filaTrabajo struct {
	estado     string
	errorMsg   string
	intentos   int
	disponible time.Time
}

func leerFila(t *testing.T, pool *pgxpool.Pool, id int64) filaTrabajo {
	t.Helper()
	var f filaTrabajo
	err := pool.QueryRow(t.Context(),
		`SELECT estado, error, intentos, disponible_en FROM cola_trabajos WHERE id = $1`, id).
		Scan(&f.estado, &f.errorMsg, &f.intentos, &f.disponible)
	if err != nil {
		t.Fatalf("leer la fila %d: %v", id, err)
	}
	return f
}

func contarTrabajos(t *testing.T, pool *pgxpool.Pool, cond string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM cola_trabajos WHERE `+cond, args...).Scan(&n); err != nil {
		t.Fatalf("contar trabajos: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Encolar
// ---------------------------------------------------------------------------

// El criterio de aceptacion: encolar dos veces el mismo trabajo logico
// produce UNA ejecucion. La restriccion de la base es la que arbitra.
func TestEncolarDosVecesLaMismaClaveDejaUnaSolaFila(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()
	k := clave(aplicacion.TrabajoEjecutarReparto, "2026", 1)

	encolado, err := store.Encolar(ctx, k, nil)
	if err != nil {
		t.Fatalf("Encolar: %v", err)
	}
	if !encolado {
		t.Fatal("el primer encolado tiene que decir true")
	}

	// Reintentar el encolado es normal -el scheduler lo hace cada vez que no
	// llego a marcar el periodo- y no es un fallo.
	encolado, err = store.Encolar(ctx, k, nil)
	if err != nil {
		t.Fatalf("Encolar por segunda vez: %v", err)
	}
	if encolado {
		t.Fatal("el segundo encolado tiene que decir false")
	}

	if n := contarTrabajos(t, pool, `TRUE`); n != 1 {
		t.Fatalf("hay %d filas, se esperaba 1", n)
	}
}

// Las tres partes de la clave son parte de la clave. Cambiar cualquiera de
// ellas es otro trabajo, y el caso que importa es la corrida: es la unica
// forma legitima de volver sobre un periodo ya repartido (RD 14.5.10-12).
func TestEncolarDistingueLasTresPartesDeLaClave(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()

	claves := []aplicacion.ClaveTrabajo{
		clave(aplicacion.TrabajoEjecutarReparto, "2026", 1),
		clave(aplicacion.TrabajoEjecutarReparto, "2026", 2), // corrida de ajuste
		clave(aplicacion.TrabajoEjecutarReparto, "2026-12", 1),
		clave(aplicacion.TrabajoResolverUsos, "2026", 1),
	}
	for _, k := range claves {
		encolado, err := store.Encolar(ctx, k, nil)
		if err != nil {
			t.Fatalf("Encolar %s: %v", k, err)
		}
		if !encolado {
			t.Fatalf("Encolar %s tenia que encolar", k)
		}
	}

	if n := contarTrabajos(t, pool, `TRUE`); n != len(claves) {
		t.Fatalf("hay %d filas, se esperaban %d", n, len(claves))
	}
}

// Una clave invalida se rechaza ANTES de tocar la base: el error nombra el
// campo que venia mal, no una restriccion de PostgreSQL.
func TestEncolarRechazaUnaClaveInvalidaSinEscribir(t *testing.T) {
	store, pool := colaVacia(t)

	if _, err := store.Encolar(t.Context(), clave(aplicacion.TrabajoEjecutarReparto, "dic-2026", 1), nil); err == nil {
		t.Fatal("un periodo con formato ajeno tiene que fallar")
	}
	if n := contarTrabajos(t, pool, `TRUE`); n != 0 {
		t.Fatalf("hay %d filas, no se esperaba ninguna", n)
	}
}

// El payload vacio entra como el objeto JSON vacio: la columna es JSONB NOT
// NULL y la cadena vacia no es JSON valido.
func TestEncolarGuardaElPayload(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()

	if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoResolverUsos, "2026", 1), nil); err != nil {
		t.Fatalf("Encolar sin payload: %v", err)
	}
	if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoResolverUsos, "2026", 2), []byte(`{"reporte":"rep-1"}`)); err != nil {
		t.Fatalf("Encolar con payload: %v", err)
	}

	if n := contarTrabajos(t, pool, `corrida = 1 AND payload = '{}'::jsonb`); n != 1 {
		t.Errorf("el payload vacio tiene que quedar como '{}', filas = %d", n)
	}
	if n := contarTrabajos(t, pool, `corrida = 2 AND payload->>'reporte' = 'rep-1'`); n != 1 {
		t.Errorf("el payload no se guardo, filas = %d", n)
	}
}

// ---------------------------------------------------------------------------
// Tomar
// ---------------------------------------------------------------------------

// Cola vacia es la condicion normal de un worker, no un fallo. Y NO es
// ErrNoEncontrado: si lo fuera, el bucle registraria un error en cada tic.
func TestTomarConLaColaVaciaEsErrSinTrabajo(t *testing.T) {
	store, _ := colaVacia(t)

	_, err := store.Tomar(t.Context(), time.Now())
	if !errors.Is(err, aplicacion.ErrSinTrabajo) {
		t.Fatalf("err = %v, se esperaba ErrSinTrabajo", err)
	}
	if errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatal("cola vacia no es ErrNoEncontrado")
	}
}

func TestTomarEsFIFOYMarcaElTrabajoEnCurso(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()
	ahora := time.Now().Add(time.Second)

	for corrida := 1; corrida <= 3; corrida++ {
		if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoResolverUsos, "2026", corrida), nil); err != nil {
			t.Fatalf("Encolar: %v", err)
		}
	}

	for corrida := 1; corrida <= 3; corrida++ {
		trabajo, err := store.Tomar(ctx, ahora)
		if err != nil {
			t.Fatalf("Tomar: %v", err)
		}
		if trabajo.Clave.Corrida != corrida {
			t.Fatalf("se tomo la corrida %d, se esperaba la %d", trabajo.Clave.Corrida, corrida)
		}
		if trabajo.Clave.Tipo != aplicacion.TrabajoResolverUsos {
			t.Errorf("tipo = %q", trabajo.Clave.Tipo)
		}
		// Intentos cuenta esta toma: sin el, la politica de reintentos no
		// sabria por que intento va.
		if trabajo.Intentos != 1 {
			t.Errorf("Intentos = %d, se esperaba 1", trabajo.Intentos)
		}
		if f := leerFila(t, pool, trabajo.ID); f.estado != "en_curso" {
			t.Errorf("estado = %q, se esperaba en_curso", f.estado)
		}
	}
}

// Un trabajo reprogramado no se vuelve a tomar antes de tiempo. Sin este
// filtro, el worker giraria sobre el hasta agotar los intentos en segundos y
// la espera exponencial no serviria de nada.
func TestTomarRespetaLaEsperaDeReintento(t *testing.T) {
	store, _ := colaVacia(t)
	ctx := t.Context()
	ahora := time.Now().Add(time.Second)

	if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoEjecutarReparto, "2026", 1), nil); err != nil {
		t.Fatalf("Encolar: %v", err)
	}

	trabajo, err := store.Tomar(ctx, ahora)
	if err != nil {
		t.Fatalf("Tomar: %v", err)
	}
	if err := store.Cerrar(ctx, trabajo.ID, aplicacion.Reintentar(ahora.Add(time.Hour), "la base no responde")); err != nil {
		t.Fatalf("Cerrar con reintento: %v", err)
	}

	if _, err := store.Tomar(ctx, ahora); !errors.Is(err, aplicacion.ErrSinTrabajo) {
		t.Fatalf("err = %v, el trabajo no puede volver antes de su hora", err)
	}

	segunda, err := store.Tomar(ctx, ahora.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Tomar pasada la espera: %v", err)
	}
	if segunda.ID != trabajo.ID {
		t.Fatalf("ID = %d, se esperaba %d", segunda.ID, trabajo.ID)
	}
	if segunda.Intentos != 2 {
		t.Fatalf("Intentos = %d, se esperaba 2", segunda.Intentos)
	}
}

// EL CRITERIO DE ACEPTACION DEL ISSUE: dos workers tirando a la vez nunca
// procesan el mismo trabajo.
//
// Es lo que compra el FOR UPDATE SKIP LOCKED, y es lo unico de este PR que un
// doble en memoria no puede probar: la exclusion la da PostgreSQL. Sin
// SKIP LOCKED los workers se serializan sobre la misma fila; sin FOR UPDATE
// los dos leen la misma fila pendiente y la corrida se ejecuta dos veces.
//
// Corre con -race en CI (test-go.yml usa -race), asi que ademas cubre las
// carreras del lado de Go.
func TestTomarNoEntregaElMismoTrabajoADosWorkers(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()
	ahora := time.Now().Add(time.Second)

	const trabajos, workers = 24, 4
	for corrida := 1; corrida <= trabajos; corrida++ {
		if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoResolverUsos, "2026", corrida), nil); err != nil {
			t.Fatalf("Encolar: %v", err)
		}
	}

	var (
		mu      sync.Mutex
		vistos  = map[int64]int{}
		fallos  []error
		esperar sync.WaitGroup
	)
	for range workers {
		esperar.Add(1)
		go func() {
			defer esperar.Done()
			for {
				trabajo, err := store.Tomar(ctx, ahora)
				if errors.Is(err, aplicacion.ErrSinTrabajo) {
					return
				}
				mu.Lock()
				if err != nil {
					fallos = append(fallos, err)
					mu.Unlock()
					return
				}
				vistos[trabajo.ID]++
				mu.Unlock()

				if err := store.Cerrar(ctx, trabajo.ID, aplicacion.Hecho()); err != nil {
					mu.Lock()
					fallos = append(fallos, err)
					mu.Unlock()
					return
				}
			}
		}()
	}
	esperar.Wait()

	if len(fallos) > 0 {
		t.Fatalf("los workers fallaron: %v", fallos)
	}
	if len(vistos) != trabajos {
		t.Fatalf("se tomaron %d trabajos distintos, se esperaban %d", len(vistos), trabajos)
	}
	for id, veces := range vistos {
		if veces != 1 {
			t.Errorf("el trabajo %d se tomo %d veces", id, veces)
		}
	}
	if n := contarTrabajos(t, pool, `estado = 'hecho'`); n != trabajos {
		t.Fatalf("hay %d trabajos hechos, se esperaban %d", n, trabajos)
	}
}

// ---------------------------------------------------------------------------
// Cerrar
// ---------------------------------------------------------------------------

func TestCerrarEscribeLasTresFormas(t *testing.T) {
	ahora := time.Now().Add(time.Second)
	vuelta := ahora.Add(90 * time.Minute)

	casos := []struct {
		nombre    string
		cierre    aplicacion.Cierre
		estado    string
		conError  bool
		mueveHora bool
	}{
		{"exito", aplicacion.Hecho(), "hecho", false, false},
		{"reintento", aplicacion.Reintentar(vuelta, "la base no responde"), "pendiente", true, true},
		{"abandono", aplicacion.Abandonar("sin manejador"), "fallido", true, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			store, pool := colaVacia(t)
			ctx := t.Context()

			if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoEjecutarReparto, "2026", 1), nil); err != nil {
				t.Fatalf("Encolar: %v", err)
			}
			trabajo, err := store.Tomar(ctx, ahora)
			if err != nil {
				t.Fatalf("Tomar: %v", err)
			}
			if err := store.Cerrar(ctx, trabajo.ID, c.cierre); err != nil {
				t.Fatalf("Cerrar: %v", err)
			}

			f := leerFila(t, pool, trabajo.ID)
			if f.estado != c.estado {
				t.Errorf("estado = %q, se esperaba %q", f.estado, c.estado)
			}
			if (f.errorMsg != "") != c.conError {
				t.Errorf("error = %q, conError = %v", f.errorMsg, c.conError)
			}
			// La hora solo se mueve cuando hay reintento: dejarla intacta en
			// los otros dos casos es lo que permite ver, en un trabajo
			// abandonado, cuando se intento por ultima vez.
			if movida := f.disponible.After(ahora); movida != c.mueveHora {
				t.Errorf("disponible_en = %v (ahora %v), se esperaba movida = %v",
					f.disponible, ahora, c.mueveHora)
			}
		})
	}
}

// Un cierre repetido no puede reescribir un estado que ya no le pertenece:
// dejaria `hecho` un trabajo reprogramado, y la corrida no se ejecutaria.
func TestCerrarDosVecesElMismoTrabajoEsErrNoEncontrado(t *testing.T) {
	store, _ := colaVacia(t)
	ctx := t.Context()
	ahora := time.Now().Add(time.Second)

	if _, err := store.Encolar(ctx, clave(aplicacion.TrabajoEjecutarReparto, "2026", 1), nil); err != nil {
		t.Fatalf("Encolar: %v", err)
	}
	trabajo, err := store.Tomar(ctx, ahora)
	if err != nil {
		t.Fatalf("Tomar: %v", err)
	}
	if err := store.Cerrar(ctx, trabajo.ID, aplicacion.Hecho()); err != nil {
		t.Fatalf("Cerrar: %v", err)
	}

	if err := store.Cerrar(ctx, trabajo.ID, aplicacion.Hecho()); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

func TestCerrarUnTrabajoQueNoExisteEsErrNoEncontrado(t *testing.T) {
	store, _ := colaVacia(t)

	if err := store.Cerrar(t.Context(), 9999, aplicacion.Hecho()); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

// Un trabajo hecho sigue ocupando su clave natural: la corrida del periodo no
// se puede volver a encolar en silencio. Es el criterio "una corrida ya
// completada no se repite"; volver sobre el periodo exige una corrida de
// ajuste explicita, que es otra clave.
func TestUnTrabajoHechoBloqueaElReencoladoDeLaMismaCorrida(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()
	ahora := time.Now().Add(time.Second)
	original := clave(aplicacion.TrabajoEjecutarReparto, "2026", 1)

	if _, err := store.Encolar(ctx, original, nil); err != nil {
		t.Fatalf("Encolar: %v", err)
	}
	trabajo, err := store.Tomar(ctx, ahora)
	if err != nil {
		t.Fatalf("Tomar: %v", err)
	}
	if err := store.Cerrar(ctx, trabajo.ID, aplicacion.Hecho()); err != nil {
		t.Fatalf("Cerrar: %v", err)
	}

	encolado, err := store.Encolar(ctx, original, nil)
	if err != nil {
		t.Fatalf("Encolar de nuevo: %v", err)
	}
	if encolado {
		t.Fatal("una corrida ya completada no se vuelve a encolar")
	}

	// La corrida de ajuste si entra, y entra como trabajo aparte.
	ajuste := clave(aplicacion.TrabajoEjecutarReparto, "2026", 2)
	encolado, err = store.Encolar(ctx, ajuste, nil)
	if err != nil {
		t.Fatalf("Encolar el ajuste: %v", err)
	}
	if !encolado {
		t.Fatal("una corrida de ajuste explicita si se encola")
	}
	if n := contarTrabajos(t, pool, `estado = 'pendiente'`); n != 1 {
		t.Fatalf("hay %d pendientes, se esperaba solo el ajuste", n)
	}
}
