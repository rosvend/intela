// Command worker procesa la cola de trabajos: matching por lotes y corridas
// de reparto.
//
// Un SIGTERM tiene que dejar terminar el trabajo en curso. Con time.Sleep en
// un bucle infinito no habia forma: la senal mataba el proceso a media
// transaccion, y una corrida de reparto a medias es dinero a medio calcular.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
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
	defer store.Cerrar()

	intervalo := config.Duracion("WORKER_INTERVALO", 5*time.Second)
	tic := time.NewTicker(intervalo)
	defer tic.Stop()

	log.Info("worker en marcha", slog.Duration("intervalo", intervalo))

	for {
		select {
		case <-ctx.Done():
			// El trabajo en curso ya termino: el select solo vuelve aqui
			// entre iteraciones.
			log.Info("senal recibida, worker detenido")
			return nil
		case <-tic.C:
			if err := tomarUnTrabajo(ctx, log); err != nil {
				// Un fallo puntual no tumba el worker; el siguiente tic
				// lo reintenta.
				log.Error("ciclo fallido", slog.Any("error", err))
			}
		}
	}
}

// tomarUnTrabajo procesara un elemento de la cola. La cola y sus
// manejadores entran con el PR de persistencia.
func tomarUnTrabajo(ctx context.Context, log *slog.Logger) error {
	log.Debug("sin cola conectada todavia")
	return nil
}
