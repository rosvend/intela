// Command api sirve la API HTTP.
//
// No aplica migraciones ni siembra datos al arrancar. Las migraciones son de
// goose y corren como paso propio del despliegue; el seed es cmd/seed y solo
// se invoca a mano en desarrollo.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/httpapi"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
)

func main() {
	log := config.Logger("api")
	if err := ejecutar(log); err != nil {
		log.Error("arranque fallido", slog.Any("error", err))
		os.Exit(1)
	}
}

// ejecutar devuelve error en vez de llamar a log.Fatal.
//
// log.Fatal llama a os.Exit(1), que NO corre los defer: con el patron
// anterior, el defer store.Cerrar() era codigo muerto y el pool nunca se
// cerraba limpiamente.
func ejecutar(log *slog.Logger) error {
	// NotifyContext cancela el contexto al recibir SIGINT o SIGTERM, que es
	// lo que manda `docker compose down`.
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

	api := httpapi.Nueva(store, httpapi.Opciones{
		OrigenesPermitidos: config.Lista("CORS_ORIGENES"),
		Log:                log,
	})

	srv := &http.Server{
		Addr:    config.Cadena("ADDR", ":8080"),
		Handler: api.Router(),

		// Los cuatro, no solo el de cabeceras. Con solo
		// ReadHeaderTimeout, una subida lenta a un endpoint de reportes
		// retiene la conexion indefinidamente.
		ReadHeaderTimeout: config.Duracion("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       config.Duracion("HTTP_READ_TIMEOUT", 60*time.Second),
		WriteTimeout:      config.Duracion("HTTP_WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:       config.Duracion("HTTP_IDLE_TIMEOUT", 120*time.Second),

		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errServidor := make(chan error, 1)
	go func() {
		log.Info("escuchando", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errServidor <- err
			return
		}
		errServidor <- nil
	}()

	select {
	case err := <-errServidor:
		return err
	case <-ctx.Done():
		log.Info("senal recibida, cerrando")
	}

	// Contexto nuevo: el de arriba ya esta cancelado, y con el no habria
	// margen para terminar las peticiones en vuelo.
	ctxApagado, cancelarApagado := context.WithTimeout(
		context.Background(), config.Duracion("SHUTDOWN_TIMEOUT", 15*time.Second))
	defer cancelarApagado()

	if err := srv.Shutdown(ctxApagado); err != nil {
		return err
	}
	log.Info("cerrado limpiamente")
	return nil
}
