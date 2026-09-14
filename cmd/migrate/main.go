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
//
// La mecanica vive en internal/infraestructura/migraciones, porque la comparte
// con cmd/lambda-migrate: en AWS la base esta en subred privada y las
// migraciones tienen que correr desde dentro de la VPC.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/rosvend/intela/internal/infraestructura/config"
	"github.com/rosvend/intela/internal/infraestructura/migraciones"
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

	ctx, cancelar := context.WithTimeout(context.Background(),
		config.Duracion("MIGRATE_TIMEOUT", 2*time.Minute))
	defer cancelar()

	return migraciones.Aplicar(ctx, config.Cadena("DATABASE_URL", ""), flag.Arg(0), log)
}
