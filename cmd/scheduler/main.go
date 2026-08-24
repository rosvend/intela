// Command scheduler dispara las corridas segun el calendario de RD 10 y
// RD 12: al menos una distribucion por ano calendario, en la practica en la
// primera semana de diciembre.
//
// Tiene reloj propio (ADR 0003). Es el unico proceso que decide "ya toca".
package main

import (
	"context"
	"errors"
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
	log := config.Logger("scheduler")
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

	var rel aplicacion.Reloj = reloj.Sistema{}

	intervalo := config.Duracion("SCHEDULER_INTERVALO", time.Minute)
	tic := time.NewTicker(intervalo)
	defer tic.Stop()

	log.Info("scheduler en marcha", slog.Duration("intervalo", intervalo))

	for {
		select {
		case <-ctx.Done():
			log.Info("senal recibida, scheduler detenido")
			return nil
		case <-tic.C:
			if err := revisarCalendario(ctx, rel, log); err != nil {
				log.Error("revision de calendario fallida", slog.Any("error", err))
			}
		}
	}
}

// revisarCalendario mirara que periodos toca disparar. El calendario entra
// con el PR de persistencia.
//
// La hora entra por el puerto y no por time.Now(): probar que una ventana de
// 15 dias se abre cuando toca no puede depender de esperar quince dias.
func revisarCalendario(ctx context.Context, rel aplicacion.Reloj, log *slog.Logger) error {
	log.Debug("calendario revisado", slog.Time("ahora", rel.Ahora()))
	return nil
}
