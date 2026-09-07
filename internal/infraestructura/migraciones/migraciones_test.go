package migraciones_test

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	// Registra el driver "pgx" de database/sql, que es el unico que habla
	// goose. Explicito y no heredado de testhelp: que otro paquete lo importe
	// hoy no es una dependencia que se pueda leer desde aqui.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/rosvend/intela/internal/infraestructura/migraciones"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/migrations"
)

func TestMain(m *testing.M) {
	codigo := m.Run()
	testhelp.Terminar()
	os.Exit(codigo)
}

func mudo() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestAplicarConParametrosDePool es la regresion del DSN que produce Terraform.
//
// infra/modules/database entrega UN solo DSN a los dos puntos de entrada, y ese
// DSN lleva `pool_max_conns` porque dimensiona el pool de la API. Antes de este
// caso, la Lambda de migraciones abria ese mismo DSN con sql.Open("pgx", ...),
// que lo mandaba al servidor como parametro de arranque:
//
//	FATAL: unrecognized configuration parameter "pool_max_conns" (SQLSTATE 42704)
//
// El primer `terraform apply` moria ahi, en aws_lambda_invocation.migrate, y la
// API no llegaba a crearse. No se veia en local porque el DSN de
// docker-compose no lleva parametros de pool.
func TestAplicarConParametrosDePool(t *testing.T) {
	dsn := testhelp.DSN(t) + "&pool_max_conns=4&pool_max_conn_lifetime=1h"

	if err := migraciones.Aplicar(t.Context(), dsn, "up", mudo()); err != nil {
		t.Fatalf("un DSN con parametros de pool tiene que migrar: %v", err)
	}
}

// El DSN de siempre tiene que seguir funcionando: el arreglo no puede depender
// de que haya parametros de pool que quitar.
func TestAplicarSinParametrosDePool(t *testing.T) {
	if err := migraciones.Aplicar(t.Context(), testhelp.DSN(t), "up", mudo()); err != nil {
		t.Fatalf("migrar: %v", err)
	}
}

func TestAplicarSinDSN(t *testing.T) {
	err := migraciones.Aplicar(t.Context(), "", "up", mudo())
	if err == nil {
		t.Fatal("un DSN vacio tiene que dar error")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("el error tiene que nombrar la variable que falta, dio: %v", err)
	}
}

// Un DSN ilegible tiene que fallar al parsearlo, no varias capas mas abajo
// cuando ya se esta hablando con un servidor.
func TestAplicarDSNInvalido(t *testing.T) {
	err := migraciones.Aplicar(t.Context(), "esto://no es un dsn", "up", mudo())
	if err == nil {
		t.Fatal("un DSN invalido tiene que dar error")
	}
	if !strings.Contains(err.Error(), "dsn invalido") {
		t.Errorf("el error tiene que decir que el DSN no se pudo leer, dio: %v", err)
	}
}

// ---------------------------------------------------------------------------
// La numeracion de las migraciones
// ---------------------------------------------------------------------------

// desplegadas son los ficheros de migracion que `main` ya tiene APLICADOS en
// la base de produccion.
//
// Se enumeran a mano, y eso es lo que hace util la prueba de abajo: son un
// hecho sobre el DESPLIEGUE, no sobre este arbol de trabajo. Derivarlas de
// migrations.FS -"las que no anade esta rama"- haria que la prueba pasara
// siempre, porque compararia el arbol consigo mismo.
//
// Hoy la lista es 1, 2 y 5: el PR #85 renumero su migracion a 00005 y su
// despliegue corrio `goose up` de verdad, asi que produccion esta en la
// version 5 con el hueco 3-4 libre PARA SIEMPRE. Cuando main avance, esta
// lista avanza con ella.
var desplegadas = []string{
	"00001_init.sql",
	"00002_catalogo_obras.sql",
	"00005_cola_clave_natural.sql",
}

// TestAplicarSobreLaVersionDesplegada es la regresion de la numeracion.
//
// `Aplicar` llama a goose.RunContext SIN opciones, asi que corre con
// allowMissing = false. Con ese ajuste, una migracion numerada POR DEBAJO de la
// version que la base ya tiene aplicada no es una migracion que llegue tarde:
// es un error que para a goose en seco antes de aplicar NADA.
//
//	found N missing migrations before current version 5
//
// Y no para solo el esquema. El despliegue corre las migraciones ANTES de sacar
// la API y condiciona el rollout a que terminen bien, asi que una version libre
// por debajo de la desplegada bloquea el despliegue entero: la version nueva
// del codigo no llega a salir.
//
// Es un fallo que NO se ve en local ni en CI de la forma habitual, y por eso
// hace falta escribirlo: contra una base recien creada -la que da testhelp- las
// migraciones se aplican de la 1 a la ultima en orden, no falta ninguna, y todo
// pasa. El hueco solo existe contra una base que YA vivio el despliegue de main.
func TestAplicarSobreLaVersionDesplegada(t *testing.T) {
	ctx := t.Context()
	dsn := testhelp.DSN(t)

	// testhelp entrega la base con TODAS las migraciones de esta rama
	// aplicadas, que es justo el estado en el que el hueco no se nota. Volver a
	// cero y subir solo lo que main tiene es lo que reproduce produccion.
	if err := migraciones.Aplicar(ctx, dsn, "reset", mudo()); err != nil {
		t.Fatalf("volver la base a cero: %v", err)
	}
	ponerEnLaVersionDesplegada(ctx, t, dsn)

	if err := migraciones.Aplicar(ctx, dsn, "up", mudo()); err != nil {
		t.Fatalf("una base en la version ya desplegada tiene que poder migrar, "+
			"y una migracion nueva por debajo de esa version se lo impide: %v", err)
	}
}

// ponerEnLaVersionDesplegada aplica SOLO las migraciones de [desplegadas].
//
// Con un FS recortado y no con `up-to`: `up-to 5` aplicaria tambien cualquier
// version intermedia que anada esta rama, que es exactamente el hueco que hay
// que dejar sin tapar para que la prueba signifique algo.
//
// Con Provider y no con las funciones globales de goose porque [migraciones.Aplicar]
// usa esas globales: pisarlas aqui dejaria la prueba siguiente montada sobre el
// FS recortado.
func ponerEnLaVersionDesplegada(ctx context.Context, t *testing.T, dsn string) {
	t.Helper()

	soloMain := fstest.MapFS{}
	for _, nombre := range desplegadas {
		sql, err := migrations.FS.ReadFile(nombre)
		if err != nil {
			t.Fatalf("leer %s de las migraciones embebidas: %v", nombre, err)
		}
		soloMain[nombre] = &fstest.MapFile{Data: sql}
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("abrir conexion: %v", err)
	}
	// Antes de que otra prueba pida base: el Restore de testhelp tira la base y
	// DROP DATABASE no convive con conexiones vivas.
	t.Cleanup(func() { _ = db.Close() })

	p, err := goose.NewProvider(goose.DialectPostgres, db, soloMain)
	if err != nil {
		t.Fatalf("goose: %v", err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("dejar la base en la version desplegada: %v", err)
	}
}
