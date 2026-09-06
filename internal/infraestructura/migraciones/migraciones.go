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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
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

	db, err := abrir(dsn)
	if err != nil {
		return err
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

// abrir conecta con el MISMO DSN que usa el resto del sistema, incluidos los
// parametros de dimensionado del pool.
//
// El sistema reparte un unico DSN, y ese DSN lleva `pool_max_conns` porque es
// lo que dimensiona el pool de la API. Ese parametro lo entiende
// [pgxpool.ParseConfig] y NADIE MAS: `sql.Open("pgx", dsn)` pasa por
// pgx.ParseConfig, que no lo reconoce, lo trata como parametro de arranque del
// servidor y lo manda en el paquete de conexion. Postgres responde
//
//	FATAL: unrecognized configuration parameter "pool_max_conns" (SQLSTATE 42704)
//
// y la migracion no llega ni a empezar. En local no se veia porque el DSN de
// docker-compose no lleva parametros de pool; el de Terraform si.
//
// Se parsea con pgxpool y se abre con ConnConfig -que es el resultado ya
// limpio, porque pgxpool retira los `pool_*` que consume- en vez de recortar la
// cadena a mano. Asi este runner interpreta el DSN exactamente igual que el
// adaptador de persistencia, incluida la forma `clave=valor`, y no hay una
// segunda lista de parametros que mantener al dia.
func abrir(dsn string) (*sql.DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("dsn invalido: %w", err)
	}
	return stdlib.OpenDB(*cfg.ConnConfig), nil
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
