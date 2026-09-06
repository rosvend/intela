package migraciones_test

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/infraestructura/migraciones"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
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
