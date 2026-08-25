// Command migrate aplica las migraciones con goose.
//
// Existe porque la API las aplicaba al arrancar: leia el .sql entero y lo
// ejecutaba, sin tabla de versiones y sin `down`. Eso funciona mientras haya
// UNA migracion. Con dos, la segunda vuelve a ejecutar la primera.
//
// Ahora es un paso propio del despliegue, con las migraciones embebidas en el
// binario: la imagen no depende de que alguien monte el directorio correcto.
//
//	migrate up        aplica lo pendiente (por defecto)
//	migrate down      revierte la ultima
//	migrate status    que hay aplicado
//	migrate version   version actual
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/migrations"
)

func main() {
	log := config.Logger("migrate")
	if err := ejecutar(log); err != nil {
		log.Error("migracion fallida", slog.Any("error", err))
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	flag.Parse()
	orden := flag.Arg(0)
	if orden == "" {
		orden = "up"
	}

	dsn := config.Cadena("DATABASE_URL", "")
	if dsn == "" {
		return errors.New("falta DATABASE_URL")
	}

	// goose habla database/sql; "pgx" es el driver registrado por stdlib.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abrir conexion: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancelar := context.WithTimeout(context.Background(),
		config.Duracion("MIGRATE_TIMEOUT", 2*time.Minute))
	defer cancelar()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("la base no responde: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(gooseLog{log: log})
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	log.Info("ejecutando", slog.String("orden", orden))
	if err := goose.RunContext(ctx, orden, db, "."); err != nil {
		return err
	}
	log.Info("migraciones al dia")
	return nil
}

// gooseLog adapta el logger de goose a slog, para que todo el proceso salga
// con la misma estructura.
type gooseLog struct{ log *slog.Logger }

func (g gooseLog) Printf(format string, v ...any) {
	g.log.Info("goose", slog.String("msg", fmt.Sprintf(format, v...)))
}

func (g gooseLog) Fatalf(format string, v ...any) {
	g.log.Error("goose", slog.String("msg", fmt.Sprintf(format, v...)))
	os.Exit(1)
}
