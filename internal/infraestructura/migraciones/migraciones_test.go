package migraciones_test

import (
	"context"
	"database/sql"
	"errors"
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
	"github.com/rosvend/intela/internal/infraestructura/migraciones/numeracion"
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

// TestAplicarSobreLaVersionDesplegada es la regresion de la numeracion.
//
// `Aplicar` llama a goose.RunContext SIN opciones, asi que corre con
// allowMissing = false. Con ese ajuste, una migracion numerada POR DEBAJO de la
// version que la base ya tiene aplicada no es una migracion que llegue tarde:
// es un error que para a goose en seco antes de aplicar NADA.
//
//	found N missing migrations before current version X
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
//
// Las desplegadas se derivan de MIGRACIONES_BASE_REF (main), no de una lista a
// mano: esa lista se quedo en 1/2/5 mientras produccion avanzaba (#110).
func TestAplicarSobreLaVersionDesplegada(t *testing.T) {
	ctx := t.Context()
	dsn := testhelp.DSN(t)

	desplegadas, err := numeracion.DesplegadasEn(numeracion.BaseRef())
	if err != nil {
		t.Fatalf("%v", err)
	}

	// testhelp entrega la base con TODAS las migraciones de esta rama
	// aplicadas, que es justo el estado en el que el hueco no se nota. Volver a
	// cero y subir solo lo que main tiene es lo que reproduce produccion.
	if err := migraciones.Aplicar(ctx, dsn, "reset", mudo()); err != nil {
		t.Fatalf("volver la base a cero: %v", err)
	}
	ponerEnLaVersionDesplegada(ctx, t, dsn, desplegadas)

	if err := migraciones.Aplicar(ctx, dsn, "up", mudo()); err != nil {
		t.Fatalf("una base en la version ya desplegada tiene que poder migrar, "+
			"y una migracion nueva por debajo de esa version se lo impide: %v", err)
	}
}

// ponerEnLaVersionDesplegada aplica SOLO las migraciones de desplegadas que
// esta rama ya tiene embebidas.
//
// MIGRACIONES_BASE_REF es la punta de main al disparar el evento, no el
// merge-base. Una rama que no ha rebasado no tiene los .sql que main gano
// mientras tanto: leerlos del FS embebido fallaria por un motivo que no es
// la numeracion de esta PR. La interseccion ignora esos nombres -son justo
// las migraciones sobre las que aun no se ha rebasado- y la pregunta de la
// prueba (¿una nueva por debajo rompe el up?) no las necesita.
//
// Con un FS recortado y no con `up-to`: `up-to N` aplicaria tambien cualquier
// version intermedia que anada esta rama, que es exactamente el hueco que hay
// que dejar sin tapar para que la prueba signifique algo.
//
// Con Provider y no con las funciones globales de goose porque [migraciones.Aplicar]
// usa esas globales: pisarlas aqui dejaria la prueba siguiente montada sobre el
// FS recortado.
func ponerEnLaVersionDesplegada(ctx context.Context, t *testing.T, dsn string, desplegadas []string) {
	t.Helper()

	soloMain := fstest.MapFS{}
	var omitidas []string
	for _, nombre := range desplegadas {
		sql, err := migrations.FS.ReadFile(nombre)
		if err != nil {
			omitidas = append(omitidas, nombre)
			continue
		}
		soloMain[nombre] = &fstest.MapFile{Data: sql}
	}
	if len(soloMain) == 0 {
		t.Skipf("la rama va por detras de main: ninguna de las %d migraciones "+
			"desplegadas esta embebida; rebasa antes de confiar en esta prueba",
			len(desplegadas))
	}
	if len(omitidas) > 0 {
		t.Logf("rama por detras de main: se omiten %d migraciones aun no rebaseadas (%s)",
			len(omitidas), strings.Join(omitidas, ", "))
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

// TestUpYDownRecorrenTodasLasMigraciones baja y vuelve a subir el esquema
// entero, que hasta ahora no lo hacia nadie: CI solo prueba el `up`, y un
// bloque `-- +goose Down` roto no se descubriria hasta necesitarlo, que es el
// peor momento posible.
//
// Con Provider y no con las globales de [migraciones.Aplicar]: esas son estado
// del proceso y esto corre dentro de un binario de pruebas con -race.
//
// # No empieza en cero
//
// testhelp.DSN entrega una base ya migrada a la version mas alta (es su
// plantilla, para que las pruebas de este paquete no repitan el costo de
// migrar). El primer Up de aqui abajo es por tanto un no-op de verificacion,
// no el ejercicio real. Lo que de verdad prueba esta funcion es el ciclo
// DownTo(0) -> Up: si algun bloque Down deja algo a medio revertir -- una
// tabla, un indice, un CHECK -- el Up que le sigue choca contra lo que quedo,
// y sin datos de por medio ese choque solo puede venir de un Down mal escrito.
func TestUpYDownRecorrenTodasLasMigraciones(t *testing.T) {
	ctx := t.Context()

	db, err := sql.Open("pgx", testhelp.DSN(t))
	if err != nil {
		t.Fatalf("abrir la base: %v", err)
	}
	defer func() { _ = db.Close() }()

	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("subir el esquema: %v", err)
	}
	version, err := p.GetDBVersion(ctx)
	if err != nil {
		t.Fatalf("leer la version: %v", err)
	}
	if version == 0 {
		t.Fatal("el esquema quedo en la version 0 despues de un up")
	}

	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("bajar el esquema entero: %v", err)
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != 0 {
		t.Fatalf("version tras el down = %d (err %v), se esperaba 0", v, err)
	}

	// Volver a subir sobre lo que dejo el down: si un Down olvida soltar algo,
	// el Up siguiente choca contra el resto.
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("volver a subir despues de un down limpio: %v", err)
	}
	if v, err := p.GetDBVersion(ctx); err != nil || v != version {
		t.Fatalf("version tras el segundo up = %d (err %v), se esperaba %d", v, err, version)
	}
}

// El Down de refiere_a restaura el cuerpo anterior de bitacora_solo_append y suelta la columna; el Up los repone.
func TestDownDeRefiereARestauraLaFuncionAnterior(t *testing.T) {
	ctx := t.Context()
	db, err := sql.Open("pgx", testhelp.DSN(t))
	if err != nil {
		t.Fatalf("abrir la base: %v", err)
	}
	defer func() { _ = db.Close() }()

	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	var version int64
	for _, s := range p.ListSources() {
		if strings.HasSuffix(s.Path, "_bitacora_refiere_a.sql") {
			version = s.Version
		}
	}
	if version == 0 {
		t.Fatal("no se encontro la migracion *_bitacora_refiere_a.sql")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("subir: %v", err)
	}

	estado := func() (codigo, columna bool) {
		var def string
		if err := db.QueryRowContext(ctx, `SELECT pg_get_functiondef('bitacora_solo_append'::regproc)`).Scan(&def); err != nil {
			t.Fatalf("leer la funcion: %v", err)
		}
		if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_name = 'asientos' AND column_name = 'refiere_a')`).Scan(&columna); err != nil {
			t.Fatalf("leer la columna: %v", err)
		}
		return strings.Contains(def, "IN006"), columna
	}

	if codigo, columna := estado(); !codigo || !columna {
		t.Fatalf("tras el up: IN006=%v refiere_a=%v, se esperaban los dos", codigo, columna)
	}
	if _, err := p.DownTo(ctx, version-1); err != nil {
		t.Fatalf("bajar a %d: %v", version-1, err)
	}
	if codigo, columna := estado(); codigo || columna {
		t.Fatalf("tras el down: IN006=%v refiere_a=%v, se esperaba el cuerpo anterior sin columna", codigo, columna)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("volver a subir: %v", err)
	}
	if codigo, columna := estado(); !codigo || !columna {
		t.Fatalf("tras el segundo up: IN006=%v refiere_a=%v", codigo, columna)
	}
}

// proveedorEnLaVersionAnteriorA baja la base de testhelp a la version justo
// anterior a la migracion cuyo nombre termina en sufijo, y devuelve el
// proveedor, la conexion y esa version.
func proveedorEnLaVersionAnteriorA(t *testing.T, sufijo string) (*goose.Provider, *sql.DB, int64) {
	t.Helper()
	db, err := sql.Open("pgx", testhelp.DSN(t))
	if err != nil {
		t.Fatalf("abrir la base: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	var version int64
	for _, s := range p.ListSources() {
		if strings.HasSuffix(s.Path, sufijo) {
			version = s.Version
		}
	}
	if version == 0 {
		t.Fatalf("no se encontro la migracion *%s", sufijo)
	}
	if _, err := p.Up(t.Context()); err != nil {
		t.Fatalf("subir: %v", err)
	}
	if _, err := p.DownTo(t.Context(), version-1); err != nil {
		t.Fatalf("bajar a %d: %v", version-1, err)
	}
	return p, db, version
}

// La migracion de parametros textuales (#194) le da `cine_teatro.base` a una
// base ya sembrada con el dataset sintetico -la de la demo, que no se puede
// resembrar porque tiene asientos-, y a ninguna otra: una instalacion con
// parametros reales no recibe un valor inventado (ADR 0004).
func TestParametrosTextualesSiembraLaBaseDeCineSoloEnUnaBaseSintetica(t *testing.T) {
	casos := []struct {
		nombre     string
		filas      string
		quiereFila bool
	}{
		{"base sembrada con el dataset sintetico",
			`('ott.wa', 0.5, DATE '2024-01-01', 'sintetico', 'RD-IX-seed-sintetico'),
			 ('ponderacion.serie', 1.3, DATE '2023-01-01', 'Consejo Directivo', 'RD 9.1.1')`, true},
		{"base con parametros reales", `('ponderacion.serie', 1.3, DATE '2023-01-01', 'Consejo Directivo', 'RD 9.1.1')`, false},
		{"base vacia", "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ctx := t.Context()
			p, db, version := proveedorEnLaVersionAnteriorA(t, "_parametros_textuales.sql")
			if c.filas != "" {
				if _, err := db.ExecContext(ctx,
					`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento) VALUES `+c.filas); err != nil {
					t.Fatalf("sembrar parametros previos: %v", err)
				}
			}
			if _, err := p.UpTo(ctx, version); err != nil {
				t.Fatalf("subir a %d: %v", version, err)
			}

			var (
				valorTexto, organo string
				desde              string
				valorNulo          bool
			)
			err := db.QueryRowContext(ctx,
				`SELECT valor_texto, valor IS NULL, organo, vigente_desde::text
				   FROM parametros WHERE clave = 'cine_teatro.base'`).Scan(&valorTexto, &valorNulo, &organo, &desde)
			if !c.quiereFila {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("no tenia que sembrarse cine_teatro.base: err=%v valor=%q", err, valorTexto)
				}
				return
			}
			if err != nil {
				t.Fatalf("leer cine_teatro.base: %v", err)
			}
			// La vigencia es la del dataset sintetico, para que cubra sus periodos.
			if valorTexto != "taquilla" || !valorNulo || organo != "sintetico" || desde != "2024-01-01" {
				t.Fatalf("cine_teatro.base = %q (valor NULL=%v, organo %q, desde %s), se esperaba taquilla sintetica desde 2024-01-01",
					valorTexto, valorNulo, organo, desde)
			}
		})
	}
}

// El down de 00024 no puede dejar un snapshot congelado sin su valor: se
// niega mientras haya uno textual (ADR 0005). Sin snapshots textuales baja y
// vuelve a subir limpio: ver TestDownDeParametrosTextualesSinSnapshotsBorraLaFilaTextual.
func TestDownDeParametrosTextualesNoDejaSnapshotsSinValor(t *testing.T) {
	ctx := t.Context()
	p, db, version := proveedorEnLaVersionAnteriorA(t, "_parametros_textuales.sql")
	if _, err := p.UpTo(ctx, version); err != nil {
		t.Fatalf("subir: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO snapshots_parametros (snapshot_id, clave, valor_texto, organo, reglamento, vigente_desde)
		 VALUES ('snp2-`+strings.Repeat("a", 64)+`', 'cine_teatro.base', 'taquilla', 'sintetico', 'RD-IX-seed-sintetico', DATE '2024-01-01')`); err != nil {
		t.Fatalf("congelar una fila textual: %v", err)
	}
	if _, err := p.DownTo(ctx, version-1); err == nil || !strings.Contains(err.Error(), "ADR 0005") {
		t.Fatalf("el down tenia que negarse citando la ADR 0005, dio: %v", err)
	}
}

// Sin snapshots textuales el down de 00024 si baja: borra la fila textual de
// `parametros` -no cabe en el esquema de 00023- y devuelve `valor` a NOT NULL.
// Es el camino del DELETE y del SET NOT NULL, que la prueba de la negativa no
// recorre. Despues vuelve a subir.
func TestDownDeParametrosTextualesSinSnapshotsBorraLaFilaTextual(t *testing.T) {
	ctx := t.Context()
	p, db, version := proveedorEnLaVersionAnteriorA(t, "_parametros_textuales.sql")
	if _, err := p.UpTo(ctx, version); err != nil {
		t.Fatalf("subir: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO parametros (clave, valor, valor_texto, vigente_desde, organo, reglamento)
		 VALUES ('cine_teatro.base', NULL, 'taquilla', DATE '2024-01-01', 'sintetico', 'RD-IX-seed-sintetico'),
		        ('ott.wa', 0.5, NULL, DATE '2024-01-01', 'sintetico', 'RD-IX-seed-sintetico')`); err != nil {
		t.Fatalf("sembrar una fila textual y una numerica: %v", err)
	}

	if _, err := p.DownTo(ctx, version-1); err != nil {
		t.Fatalf("bajar sin snapshots textuales tenia que funcionar: %v", err)
	}
	var textuales, numericas int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE clave = 'cine_teatro.base'), COUNT(*) FILTER (WHERE clave = 'ott.wa')
		   FROM parametros`).Scan(&textuales, &numericas); err != nil {
		t.Fatalf("contar parametros tras el down: %v", err)
	}
	if textuales != 0 || numericas != 1 {
		t.Fatalf("tras el down quedan %d filas textuales y %d numericas, se esperaban 0 y 1", textuales, numericas)
	}
	for _, tabla := range []string{"parametros", "snapshots_parametros"} {
		var nulable string
		if err := db.QueryRowContext(ctx,
			`SELECT is_nullable FROM information_schema.columns
			  WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'valor'`,
			tabla).Scan(&nulable); err != nil {
			t.Fatalf("leer la nulabilidad de %s.valor: %v", tabla, err)
		}
		if nulable != "NO" {
			t.Errorf("%s.valor quedo nulable tras el down", tabla)
		}
	}

	if _, err := p.UpTo(ctx, version); err != nil {
		t.Fatalf("volver a subir: %v", err)
	}
}
