// Package migraciones aplica el esquema con goose.
//
// Existe como paquete, y no dentro de `cmd/migrate`, porque hay dos puntos de
// entrada que necesitan lo mismo: el CLI de siempre y la Lambda que corre las
// migraciones dentro de la VPC. Un binario de Lambda es un unico ejecutable,
// asi que no se puede reutilizar el CLI pasandole argumentos; y la base no
// tiene endpoint publico, asi que el runner de GitHub tampoco la alcanza.
//
// Es infraestructura: sabe de goose, de database/sql y del driver de pgx. El
// nucleo no lo importa ni lo conoce.
package migraciones

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	// Registra el driver "pgx" en database/sql, que es con lo que habla goose.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/rosvend/intela/migrations"
)

// OrdenPorDefecto es lo que se aplica cuando no se pide nada concreto.
const OrdenPorDefecto = "up"

// Aplicar ejecuta una orden de goose contra dsn.
//
// El plazo entra por ctx en vez de leerse aqui de una variable de entorno: este
// paquete no lee configuracion, la reciben los `main` que lo llaman. Es la
// misma razon por la que el nucleo recibe el reloj en vez de llamar a time.Now.
func Aplicar(ctx context.Context, dsn, orden string, log *slog.Logger) error {
	if dsn == "" {
		return errors.New("falta DATABASE_URL")
	}
	if orden == "" {
		orden = OrdenPorDefecto
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abrir conexion: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("la base no responde: %w", err)
	}

	// goose guarda esto en estado global del paquete. No hay carrera: el CLI es
	// un proceso de un solo uso, y la Lambda de migracion se invoca una vez por
	// despliegue, nunca en paralelo consigo misma.
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(registro{log: log})
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("dialecto: %w", err)
	}

	log.Info("ejecutando", slog.String("orden", orden))
	if err := goose.RunContext(ctx, orden, db, "."); err != nil {
		return fmt.Errorf("goose %s: %w", orden, err)
	}
	log.Info("migraciones al dia")
	return nil
}

// registro adapta el logger de goose a slog, para que todo el proceso salga con
// la misma estructura.
type registro struct{ log *slog.Logger }

func (r registro) Printf(format string, v ...any) {
	r.log.Info("goose", slog.String("msg", fmt.Sprintf(format, v...)))
}

// Fatalf NO llama a os.Exit, a diferencia de la version que vivia en
// cmd/migrate. En una Lambda, os.Exit mata el runtime sin devolver error: la
// invocacion se reporta como fallo de plataforma en vez de como migracion
// fallida, y el mensaje de goose se pierde. Aqui se registra y se deja que el
// error de RunContext suba por su cuenta, que es quien lleva la causa.
func (r registro) Fatalf(format string, v ...any) {
	r.log.Error("goose", slog.String("msg", fmt.Sprintf(format, v...)))
}
