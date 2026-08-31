// Command seed carga el dataset sintetico de desarrollo.
//
// No corre en produccion ni al arrancar la API. Se invoca a mano, contra una
// base ya migrada, para demos, pruebas de integracion del pipeline y las
// presentaciones al PO. Los valores que el cliente no ha entregado van
// etiquetados como sinteticos (ADR 0004, docs/ARRANQUE.md).
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/semilla"
)

func main() {
	log := config.Logger("seed")
	if err := ejecutar(log); err != nil {
		log.Error("semilla fallida", slog.Any("error", err))
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	dsn := config.Cadena("DATABASE_URL", "")
	if dsn == "" {
		return errors.New("falta DATABASE_URL")
	}

	ctx, cancelar := context.WithTimeout(context.Background(),
		config.Duracion("SEED_TIMEOUT", 2*time.Minute))
	defer cancelar()

	store, err := postgres.Abrir(ctx, dsn)
	if err != nil {
		return err
	}
	defer store.Cerrar()

	almacen := objetos.Disco{Dir: config.Cadena("OBJECT_DIR", "/data/objetos")}
	return semilla.Cargar(ctx, store, almacen, cripto.Bcrypt{}, claves(),
		config.Bool("SEED_RESET", false), log)
}

func claves() semilla.Claves {
	return semilla.Claves{
		Admin:        config.Cadena("SEED_CLAVE_ADMIN", "admin-local"),
		Distribucion: config.Cadena("SEED_CLAVE_DISTRIBUCION", "distribucion-local"),
		Contabilidad: config.Cadena("SEED_CLAVE_CONTABILIDAD", "contabilidad-local"),
		Auditor:      config.Cadena("SEED_CLAVE_AUDITOR", "auditor-local"),
		Titular:      config.Cadena("SEED_CLAVE_TITULAR", "ana-local"),
	}
}
