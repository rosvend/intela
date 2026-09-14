// Command scheduler dispara las corridas segun el calendario de RD 10 y
// RD 12: al menos una distribucion por ano calendario, en la practica en la
// primera semana de diciembre.
//
// Tiene reloj propio (ADR 0003). Es el unico proceso que decide "ya toca".
//
// # No es un cron
//
// Las fechas las fija el Consejo Directivo y las puede modificar por fuerza
// mayor con re-notificacion (RD 12). Son DATO que el administrador edita, no
// configuracion de operacion, y por eso viven en la tabla `calendario` y se
// leen en cada pasada (ADR 0004). Una expresion cron en el sistema operativo
// obligaria a un despliegue para mover una fecha que aprobo un organo social.
//
// # Encola, no ejecuta
//
// El calendario ABRE el proceso de reparto; no lo corre (ADR 0008). Este
// binario deja el trabajo en la cola y se aparta; quien lo ejecuta es el
// worker, y a partir de ahi el proceso avanza por accion humana en las
// compuertas de doble firma.
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
	defer store.CerrarPool()

	// El mismo *Store satisface Calendario y ColaTrabajos. Que sean el mismo
	// tipo es asunto del adaptador: el nucleo sigue viendo dos puertos.
	//
	// La hora entra por el puerto y no por time.Now(): probar que una ventana
	// se abre cuando toca no puede depender de esperar a que llegue la fecha.
	planificador := aplicacion.Planificador{
		Calendario: store,
		Cola:       store,
		Reloj:      reloj.Sistema{},
	}

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
			revisarCalendario(ctx, planificador, log)
		}
	}
}

// revisarCalendario encola los periodos vencidos y los marca disparados.
//
// Se registran las claves encoladas ANTES del error: Disparar devuelve lo que
// alcanzo a hacer, y perder ese dato dejaria un trabajo en la cola del que el
// log no dice nada. Un fallo no tumba el scheduler: la operacion es
// idempotente y la pasada siguiente reconcilia.
func revisarCalendario(ctx context.Context, p aplicacion.Planificador, log *slog.Logger) {
	claves, err := p.Disparar(ctx)
	for _, c := range claves {
		log.Info("corrida encolada", slog.String("trabajo", c.String()))
	}
	if err != nil {
		log.Error("revision de calendario fallida", slog.Any("error", err))
	}
}
