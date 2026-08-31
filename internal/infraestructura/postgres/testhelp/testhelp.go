// Package testhelp levanta un PostgreSQL real para las pruebas del adaptador.
//
// El ADR 0010 pide que los adaptadores se prueben contra Postgres de verdad y
// no contra un doble: el esquema lleva invariantes de negocio -CHECKs,
// triggers, la EXCLUDE de vigencias- que un mock no tiene, y una consulta que
// pasa contra un mock puede violar cualquiera de ellos en produccion.
//
// # Un contenedor por binario de pruebas
//
// Arrancar Postgres cuesta segundos y hay una prueba por metodo. El contenedor
// se levanta una vez, con sync.Once, y el aislamiento entre pruebas lo da
// Restore.
//
// # Restore y no TRUNCATE
//
// `asientos` y `notificaciones` llevan triggers BEFORE TRUNCATE que RECHAZAN
// la operacion: la bitacora es append-only (ADR 0006). Un harness que limpiara
// con TRUNCATE no funciona hoy, y uno que limpiara con DELETE dejaria de
// funcionar en cuanto una prueba escribiera un asiento -que es exactamente lo
// que van a hacer los issues que copien este harness.
//
// Restore tira la base y la recrea desde una plantilla tomada justo despues de
// las migraciones. No borra filas, asi que no dispara ningun trigger.
//
// # Las pruebas que usen Pool NO pueden llamar a t.Parallel()
//
// Restore es global a la base: dos pruebas en paralelo se borran los datos la
// una a la otra, y el fallo aparece como una consulta que devuelve de menos,
// no como un error del harness.
package testhelp

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	// Registra el driver "pgx" de database/sql. Lo necesitan goose, que solo
	// habla database/sql, y el WithSQLDriver("pgx") de mas abajo.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rosvend/intela/migrations"
)

const (
	// La misma familia que docker-compose.yml (postgres:16.6-alpine) y que la
	// que nombra el issue: probar contra otra version mayor no prueba nada.
	imagen = "postgres:16-alpine"

	// No puede ser "postgres": Restore se niega a tirar la base del sistema.
	base    = "intela_test"
	usuario = "intela"
	clave   = "intela"
)

var (
	unaVez      sync.Once
	contenedor  *tcpostgres.PostgresContainer
	dsn         string
	errArranque error
)

// Pool devuelve un pool contra una base recien migrada y vacia.
//
// Se salta con -short para que `go test -short ./...` no necesite Docker.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("prueba de integracion: necesita Docker")
	}

	unaVez.Do(arrancar)
	if errArranque != nil {
		t.Fatalf("levantar postgres: %v", errArranque)
	}

	ctx := t.Context()
	if err := contenedor.Restore(ctx); err != nil {
		t.Fatalf("restaurar la plantilla migrada: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	// Cerrar antes del Restore de la prueba siguiente: DROP DATABASE no
	// convive con conexiones vivas.
	t.Cleanup(pool.Close)
	return pool
}

// arrancar levanta el contenedor, migra y toma la plantilla. Corre una sola
// vez, dentro del sync.Once, que es lo que da la arista de happens-before que
// -race necesita para las variables de paquete de arriba.
func arrancar() {
	// Contexto propio y no el de una prueba: el contenedor sobrevive a todas.
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelar()

	ctr, err := tcpostgres.Run(ctx, imagen,
		tcpostgres.WithDatabase(base),
		tcpostgres.WithUsername(usuario),
		tcpostgres.WithPassword(clave),
		// El driver por defecto de Snapshot/Restore es "postgres" (lib/pq),
		// que no importamos. Sin esto no falla: degrada en silencio a un
		// `docker exec psql` mas lento que se traga el error real.
		tcpostgres.WithSQLDriver("pgx"),
		// Run no pone estrategia de espera. Postgres se reinicia despues de
		// initdb, asi que la primera conexion corre contra el servidor que se
		// esta apagando.
		tcpostgres.BasicWaitStrategies(),
	)
	contenedor = ctr
	if err != nil {
		errArranque = fmt.Errorf("arrancar contenedor: %w", err)
		return
	}

	if dsn, err = ctr.ConnectionString(ctx, "sslmode=disable"); err != nil {
		errArranque = fmt.Errorf("dsn: %w", err)
		return
	}
	if err := migrar(ctx, dsn); err != nil {
		errArranque = err
		return
	}
	// La plantilla se toma con el esquema migrado y sin una sola fila.
	if err := ctr.Snapshot(ctx); err != nil {
		errArranque = fmt.Errorf("tomar plantilla: %w", err)
	}
}

// migrar aplica las mismas migraciones embebidas que cmd/migrate.
//
// Con Provider y no con las funciones globales (goose.SetBaseFS, SetDialect,
// SetLogger) que usa cmd/migrate: esas son estado global del proceso y esto
// corre dentro de un binario de pruebas con -race. Mismo migrations.FS y mismo
// driver, asi que el esquema es el que se despliega, no una copia.
func migrar(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("abrir conexion para migrar: %w", err)
	}
	defer func() { _ = db.Close() }()

	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("goose: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrar: %w", err)
	}
	return nil
}

// Terminar apaga el contenedor. Lo llama el TestMain del paquete que use este
// harness: sin eso solo lo recoge Ryuk, que no esta disponible en todas partes.
func Terminar() {
	if contenedor == nil {
		return
	}
	_ = testcontainers.TerminateContainer(contenedor)
}
