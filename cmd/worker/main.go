// Command worker procesa la cola de trabajos: matching por lotes y corridas
// de reparto.
//
// Un SIGTERM tiene que dejar terminar el trabajo en curso. Con time.Sleep en
// un bucle infinito no habia forma: la senal mataba el proceso a media
// transaccion, y una corrida de reparto a medias es dinero a medio calcular.
//
// # Que decide este binario y que no
//
// Aqui vive el ciclo de vida -conexion, tic, senal, apagado- y la TABLA DE
// DESPACHO, que es cableado: que manejador atiende cada tipo de trabajo. La
// politica -que se reintenta, cuando y cuantas veces- vive en
// aplicacion.Despachador, donde se prueba sin levantar un proceso ni una base
// de datos (ADR 0002).
//
// Se pueden correr varias replicas: la exclusion la da el
// SELECT ... FOR UPDATE SKIP LOCKED del adaptador, no una eleccion de lider.
// Escalar el reparto es levantar mas workers (ADR 0003).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

func main() {
	log := config.Logger("worker")
	if err := ejecutar(log); err != nil {
		log.Error("arranque fallido", slog.Any("error", err))
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	dsn := config.Cadena("DATABASE_URL", "")
	if dsn == "" {
		return errors.New("falta DATABASE_URL")
	}

	ctxConexion, cancelar := context.WithTimeout(ctx, 10*time.Second)
	defer cancelar()

	store, err := postgres.Abrir(ctxConexion, dsn)
	if err != nil {
		return err
	}
	defer store.CerrarPool()

	// Aqui se juntan las dos orillas: el nucleo declara ColaTrabajos y Reloj,
	// y este es el unico sitio que sabe que los satisfacen *postgres.Store y
	// reloj.Sistema.
	despachador := aplicacion.Despachador{
		Cola:        store,
		Reloj:       reloj.Sistema{},
		Reintentos:  reintentos(),
		Manejadores: manejadores(log),
	}

	intervalo := config.Duracion("WORKER_INTERVALO", 5*time.Second)
	tic := time.NewTicker(intervalo)
	defer tic.Stop()

	log.Info("worker en marcha",
		slog.Duration("intervalo", intervalo),
		slog.Int("reintentos", despachador.Reintentos.Maximo))

	for {
		select {
		case <-ctx.Done():
			// El trabajo en curso ya termino: el select solo vuelve aqui
			// entre iteraciones.
			log.Info("senal recibida, worker detenido")
			return nil
		case <-tic.C:
			vaciar(ctx, despachador, log)
		}
	}
}

// reintentos lee la politica del entorno.
//
// Los tres valores NO son normativos: no salen del reglamento y no entran por
// ParametrosNormativos (ADR 0004). Son configuracion de operacion, y por eso
// se leen aqui y no de la tabla `parametros`.
//
// Los valores por defecto -cinco tomas, esperas de 30 s, 1 m, 2 m y 4 m- son
// una eleccion de operacion, no un requisito: cubren de sobra un corte de red
// o un reinicio de la base, y agotan los cinco intentos en menos de diez
// minutos, que con una corrida al ano deja tiempo largo para que alguien mire.
func reintentos() aplicacion.Reintentos {
	return aplicacion.Reintentos{
		Maximo: config.Entero("WORKER_REINTENTOS", 5),
		Base:   config.Duracion("WORKER_ESPERA_BASE", 30*time.Second),
		Techo:  config.Duracion("WORKER_ESPERA_TECHO", 10*time.Minute),
	}
}

// manejadores es la tabla de despacho: un manejador por tipo de trabajo.
//
// # Los dos son stubs, a proposito
//
// El mecanismo de cola -reclamo exclusivo, idempotencia, reintentos, apagado
// limpio- es el alcance del issue #35. Los manejadores de verdad son otros
// issues: la cascada de identificacion es #37 y el motor de reparto son #33 y
// #34. Escribir aqui una version provisional de cualquiera de los dos seria
// inventar logica de negocio que este PR no puede defender ante el reglamento.
//
// Un stub que devolviera nil seria peor que no tenerlo: dejaria el trabajo en
// `hecho`, y la clave natural impide volver a encolarlo. El periodo quedaria
// marcado como repartido sin haberse repartido -que es exactamente el fallo
// silencioso que la idempotencia existe para evitar, solo que al reves-. Por
// eso fallan, y fallan como ErrPermanente: reintentar no va a hacer que
// aparezca el manejador, y la fila queda en `fallido` con el numero del issue
// que lo va a implementar escrito en la columna `error`.
func manejadores(log *slog.Logger) map[aplicacion.TipoTrabajo]aplicacion.Manejador {
	return map[aplicacion.TipoTrabajo]aplicacion.Manejador{
		aplicacion.TrabajoResolverUsos:    pendiente("#37", log),
		aplicacion.TrabajoEjecutarReparto: pendiente("#33 y #34", log),
	}
}

func pendiente(issues string, log *slog.Logger) aplicacion.Manejador {
	return aplicacion.ManejadorFunc(func(_ context.Context, t aplicacion.Trabajo) error {
		log.Warn("tipo de trabajo todavia sin implementar",
			slog.String("trabajo", t.Clave.String()),
			slog.String("issues", issues))
		return fmt.Errorf("%s: lo implementan los issues %s: %w",
			t.Clave, issues, aplicacion.ErrPermanente)
	})
}

// vaciar procesa trabajos hasta que la cola quede vacia o llegue la senal.
//
// El tic marca cuando MIRAR, no cuantos hacer. Con un trabajo por tic, una
// cola sembrada con veinte trabajos tardaria veinte tics -casi dos minutos con
// el intervalo por defecto- en vaciarse, y con varias replicas el reparto de
// carga dependeria del desfase entre relojes.
//
// El bucle termina siempre: un trabajo cerrado ya no esta pendiente, y uno
// reprogramado lo esta con la fecha por delante, que es justo lo que el filtro
// de Tomar descarta.
func vaciar(ctx context.Context, d aplicacion.Despachador, log *slog.Logger) {
	for ctx.Err() == nil {
		t, err := d.ProcesarUno(ctx)
		switch {
		case errors.Is(err, aplicacion.ErrSinTrabajo):
			return

		case err != nil && t.ID == 0:
			// No se llego a tomar nada: el fallo es de la cola, no de un
			// trabajo. Insistir en el mismo tic solo repetiria el error.
			log.Error("no se pudo tomar de la cola", slog.Any("error", err))
			return

		case err != nil:
			// El trabajo YA quedo cerrado -reprogramado o abandonado- por el
			// despachador. Esto es el rastro para quien opera; el de la fila
			// es el rastro para quien consulta el estado.
			log.Error("trabajo fallido",
				slog.String("trabajo", t.Clave.String()),
				slog.Int64("id", t.ID),
				slog.Int("intentos", t.Intentos),
				slog.Any("error", err))

		default:
			log.Info("trabajo hecho",
				slog.String("trabajo", t.Clave.String()),
				slog.Int64("id", t.ID))
		}
	}
}
