package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// TestMain apaga el contenedor cuando termina el binario de pruebas.
//
// os.Exit se salta los defer, asi que el apagado va explicito entre m.Run() y
// la salida. Es el mismo patron que el TestMain del paquete postgres: cada
// binario de pruebas levanta el suyo.
func TestMain(m *testing.M) {
	codigo := m.Run()
	testhelp.Terminar()
	os.Exit(codigo)
}

func mudo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// relojDetenido es un reloj que solo avanza cuando alguien lo mueve.
//
// Por puntero a proposito: el manejador de la prueba lo adelanta para simular
// lo que tarda una corrida de verdad, sin que la prueba espere ese tiempo. Es
// el doble mas pequeno que sirve, y satisface aplicacion.Reloj.
type relojDetenido struct{ instante time.Time }

func (r *relojDetenido) Ahora() time.Time { return r.instante }

// EL AGUJERO QUE ESTA PRUEBA TAPA: que la espera exponencial se calcule bien no
// demuestra que el bucle de vaciado la respete.
//
// aplicacion.TestDespachadorReprogramaTrasUnFalloTransitorio prueba que
// `Cierre.Volver` sale del instante en que TERMINA el manejador y no del de la
// toma. Es la mitad del asunto. La otra mitad -que con ese instante bien puesto
// `vaciar` deja de retomar el mismo trabajo en la misma pasada- no la puede
// demostrar un doble en memoria: quien decide si un trabajo vuelve a estar
// disponible es el `disponible_en <= $1` de Tomar, dentro de PostgreSQL, contra
// la fila que Cerrar acaba de escribir.
//
// Por eso esta prueba junta las tres piezas reales -el `vaciar` de este binario,
// aplicacion.Despachador y *postgres.Store contra un Postgres de verdad- y no
// dobles de ninguna.
//
// # Como falla si el arreglo no esta
//
// El manejador tarda MAS (5 min de reloj) que la espera de la politica (1 min).
// Con el instante de la toma, `disponible_en` queda un minuto DESPUES DEL
// PRINCIPIO del intento, es decir cuatro minutos en el pasado para cuando el
// intento acaba: la vuelta del bucle lo encuentra disponible y lo retoma en la
// misma pasada. Con `Maximo: 2` -un solo reintento- esa segunda toma quema el
// unico reintento que habia, el trabajo se abandona y la fila acaba `fallido`
// con `disponible_en` en el pasado.
//
// Verificado en rojo: devuelto `cierreTrasFallo` al instante de la toma, las
// cinco comprobaciones caen a la vez -el manejador corre 2 veces, `intentos`
// queda en 2, la fila queda `fallido` y `disponible_en` se queda cuatro minutos
// por detras del fin del intento-. Reaplicado el arreglo, verde.
func TestVaciarNoRetomaEnLaMismaPasadaElTrabajoQueAcabaDeFallar(t *testing.T) {
	const (
		// Mas que la espera de la politica: es lo que separa las dos lecturas
		// del reloj, y sin esa separacion la prueba pasa con arreglo y sin el.
		tardanza = 5 * time.Minute
		espera   = time.Minute
	)

	ctx := t.Context()
	cadena := testhelp.DSN(t)

	// postgres.Abrir y no un pool prestado: es como cmd/worker cablea su cola
	// en ejecutar(), y lo que se prueba aqui es ese cableado.
	store, err := postgres.Abrir(ctx, cadena)
	if err != nil {
		t.Fatalf("abrir el store: %v", err)
	}
	t.Cleanup(store.CerrarPool)

	// Pool aparte para leer la tabla. La comprobacion va contra la fila y no
	// contra lo que devuelve el despachador: si Cerrar escribiera el instante
	// equivocado, una comprobacion hecha sobre el valor en memoria pasaria.
	lector, err := pgxpool.New(ctx, cadena)
	if err != nil {
		t.Fatalf("abrir el pool lector: %v", err)
	}
	t.Cleanup(lector.Close)

	clave := aplicacion.ClaveTrabajo{
		Tipo:    aplicacion.TrabajoEjecutarReparto,
		Periodo: "2026",
		Corrida: 1,
	}
	if _, err := store.Encolar(ctx, clave, nil); err != nil {
		t.Fatalf("Encolar: %v", err)
	}

	// Por delante del now() con el que la base sella disponible_en al insertar,
	// para que el trabajo ya este disponible. Truncado a microsegundos, que es
	// la precision de timestamptz: sin eso el instante no vuelve igual de la
	// base y la comparacion exacta de mas abajo seria un falso rojo.
	inicio := time.Now().UTC().Truncate(time.Microsecond).Add(time.Second)
	reloj := &relojDetenido{instante: inicio}

	ejecuciones := 0
	despachador := aplicacion.Despachador{
		Cola:  store,
		Reloj: reloj,
		// Maximo 2 deja exactamente un reintento. Es lo que convierte una
		// retoma indebida en un estado observable: el reintento se gasta en
		// esta misma pasada y la fila termina `fallido` en vez de esperando.
		Reintentos: aplicacion.Reintentos{Maximo: 2, Base: espera, Techo: 10 * time.Minute},
		Manejadores: map[aplicacion.TipoTrabajo]aplicacion.Manejador{
			aplicacion.TrabajoEjecutarReparto: aplicacion.ManejadorFunc(
				func(context.Context, aplicacion.Trabajo) error {
					ejecuciones++
					reloj.instante = reloj.instante.Add(tardanza)
					return errors.New("la base no responde")
				}),
		},
	}

	// Una sola pasada, que es lo que hace el worker en cada tic.
	vaciar(ctx, despachador, mudo())

	finManejo := inicio.Add(tardanza)

	// Errorf y no Fatalf: los tres sintomas de la retoma -las ejecuciones de
	// mas, el reintento gastado y el instante en el pasado- son la misma
	// averia vista por tres sitios, y en rojo interesan los tres a la vez.
	if ejecuciones != 1 {
		t.Errorf("el manejador corrio %d veces en una sola pasada, se esperaba 1: "+
			"vaciar retomo el trabajo que acababa de fallar", ejecuciones)
	}

	var (
		estado     string
		intentos   int
		disponible time.Time
	)
	err = lector.QueryRow(ctx,
		`SELECT estado, intentos, disponible_en
		   FROM cola_trabajos
		  WHERE tipo = $1 AND periodo = $2 AND corrida = $3`,
		string(clave.Tipo), clave.Periodo, clave.Corrida).
		Scan(&estado, &intentos, &disponible)
	if err != nil {
		t.Fatalf("leer la fila de la cola: %v", err)
	}

	// Pendiente y no fallido: al trabajo le queda su reintento, y le queda
	// porque la pasada no se lo comio.
	if estado != "pendiente" {
		t.Errorf("estado = %q, se esperaba pendiente: el reintento se gasto en la misma pasada", estado)
	}
	if intentos != 1 {
		t.Errorf("intentos = %d, se esperaba 1: la cola entrego el trabajo mas de una vez", intentos)
	}

	// El corazon del asunto: el instante guardado esta en el FUTURO respecto al
	// reloj con el que termino el intento. Si quedara en el pasado, el trabajo
	// seria retomable ya, y que esta pasada no lo retomara solo diria que el
	// bucle salio antes por casualidad.
	if !disponible.After(finManejo) {
		t.Errorf("disponible_en = %v, no es posterior al fin del manejador (%v): "+
			"la espera exponencial queda anulada y el trabajo es retomable de inmediato",
			disponible.UTC(), finManejo)
	}
	if quiero := finManejo.Add(espera); !disponible.Equal(quiero) {
		t.Errorf("disponible_en = %v, se esperaba %v (fin del manejador + la Base de la politica)",
			disponible.UTC(), quiero)
	}
}
