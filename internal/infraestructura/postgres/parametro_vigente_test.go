package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// sembrarUmbrales deja dos vigencias contiguas de `matching.umbral`, la segunda
// abierta por arriba: la misma forma que tendria el parametro tras un cambio del
// Consejo Directivo.
func sembrarUmbrales(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	store, pool := colaVacia(t)
	nuevaVigencia(t, pool, "matching.umbral", "0.50", "2024-01-01", "2025-01-01")
	nuevaVigencia(t, pool, "matching.umbral", "0.60", "2025-01-01", "")
	return store, pool
}

// Una respuesta por fecha, rango medio abierto: el dia del relevo ya es de la
// vigencia nueva.
func TestParametroVigenteEligeLaVigenciaQueCubreLaFecha(t *testing.T) {
	store, _ := sembrarUmbrales(t)

	casos := []struct {
		nombre string
		dia    string
		quiero string
	}{
		{"dentro de la primera", "2024-06-15", "0.5"},
		{"primer dia de la primera", "2024-01-01", "0.5"},
		{"ultimo dia de la primera", "2024-12-31", "0.5"},
		{"el dia del relevo cuenta para la nueva", "2025-01-01", "0.6"},
		{"dentro de la segunda", "2026-09-21", "0.6"},
		{"muy adelante, vigencia abierta", "2099-01-01", "0.6"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			v, err := store.ParametroVigente(t.Context(), "matching.umbral", fecha(t, c.dia))
			if err != nil {
				t.Fatalf("ParametroVigente: %v", err)
			}
			if !v.Equal(decimal.RequireFromString(c.quiero)) {
				t.Fatalf("en %s el valor fue %s, se esperaba %s", c.dia, v, c.quiero)
			}
		})
	}
}

// ADR 0004: ausente es ausente, nunca cero.
func TestParametroVigenteAntesDeLaPrimeraVigenciaEsError(t *testing.T) {
	store, _ := sembrarUmbrales(t)

	v, err := store.ParametroVigente(t.Context(), "matching.umbral", fecha(t, "2023-12-31"))
	if !errors.Is(err, ErrParametroSinVigencia) {
		t.Fatalf("err = %v, se esperaba ErrParametroSinVigencia", err)
	}
	if !v.IsZero() {
		t.Fatalf("un error no puede traer valor util: %s", v)
	}
}

// El error nombra la clave y la fecha: hay veintitantas en la tabla.
func TestParametroVigenteNombraLaClaveQueFalta(t *testing.T) {
	store, _ := sembrarUmbrales(t)

	_, err := store.ParametroVigente(t.Context(), "matching.umbral_banda", fecha(t, "2026-09-21"))
	if !errors.Is(err, ErrParametroSinVigencia) {
		t.Fatalf("err = %v, se esperaba ErrParametroSinVigencia", err)
	}
	if !strings.Contains(err.Error(), "matching.umbral_banda") {
		t.Fatalf("el error no nombra la clave: %v", err)
	}
	if !strings.Contains(err.Error(), "2026-09-21") {
		t.Fatalf("el error no dice para que fecha se pregunto: %v", err)
	}
}

// La restriccion es por clave, no global: dos claves distintas conviven en la
// misma vigencia.
func TestParametroVigenteLeeOtraClaveEnLaMismaVigencia(t *testing.T) {
	store, pool := sembrarUmbrales(t)
	nuevaVigencia(t, pool, "matching.umbral_banda", "0.45", "2025-01-01", "")

	v, err := store.ParametroVigente(t.Context(), "matching.umbral_banda", fecha(t, "2026-09-21"))
	if err != nil {
		t.Fatalf("ParametroVigente: %v", err)
	}
	if !v.Equal(decimal.RequireFromString("0.45")) {
		t.Fatalf("valor = %s, se esperaba 0.45", v)
	}
}

// Mismo criterio que SnapshotEnFecha: la vigencia se decide por el DIA en UTC,
// no por la zona del instante que llega. 23:00 del 31 de diciembre en Colombia
// (UTC-5) ya es 2025-01-01 en UTC, asi que le toca la vigencia nueva.
func TestParametroVigenteComparaPorDiaUTC(t *testing.T) {
	store, _ := sembrarUmbrales(t)

	cot := time.FixedZone("COT", -5*3600)
	instante := time.Date(2024, 12, 31, 23, 0, 0, 0, cot)

	v, err := store.ParametroVigente(t.Context(), "matching.umbral", instante)
	if err != nil {
		t.Fatalf("ParametroVigente: %v", err)
	}
	if !v.Equal(decimal.RequireFromString("0.6")) {
		t.Fatalf("valor = %s, se esperaba 0.6 (el dia UTC es 2025-01-01)", v)
	}
}

// Dentro de una unidad de trabajo el metodo lee con la transaccion del contexto
// (doc.go, "Limites de transaccion"): ve lo que la unidad escribio y todavia no
// esta confirmado, y desde fuera no se ve.
func TestParametroVigenteParticipaEnLaUnidad(t *testing.T) {
	// Dos conexiones: EnUnidad ocupa una con la transaccion y la lectura de
	// fuera (t.Context, sin tx) pide otra. Con el MaxConns=1 de testhelp.Pool
	// la segunda espera al pool mientras la primera espera a la segunda: un
	// interbloqueo que en CI se ve como el timeout de 10 minutos del paquete.
	// Mismo arreglo que TestActualizarMetadatosObraConcurrenteAsientaLaCadenaCompleta.
	dsn := testhelp.DSN(t)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("configurar el pool de dos conexiones: %v", err)
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("abrir el pool de dos conexiones: %v", err)
	}
	t.Cleanup(pool.Close)
	store := Nuevo(pool)

	err = store.EnUnidad(t.Context(), func(ctx context.Context) error {
		if _, err := store.ejecutorDe(ctx).Exec(ctx,
			`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
			 VALUES ('matching.umbral', 0.70, '2025-01-01'::date, $1, $2)`,
			organoSintetic, reglamentoSintetico); err != nil {
			return err
		}

		v, err := store.ParametroVigente(ctx, "matching.umbral", fecha(t, "2026-09-21"))
		if err != nil {
			t.Errorf("dentro de la unidad tenia que ver la vigencia recien escrita: %v", err)
			return nil
		}
		if !v.Equal(decimal.RequireFromString("0.7")) {
			t.Errorf("valor = %s, se esperaba 0.7", v)
		}

		// Sin el ctx de la unidad la lectura sale por el pool, otra conexion, y
		// no ve una escritura sin confirmar.
		if _, err := store.ParametroVigente(t.Context(), "matching.umbral", fecha(t, "2026-09-21")); !errors.Is(err, ErrParametroSinVigencia) {
			t.Errorf("fuera de la unidad la vigencia no debia verse todavia, err = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EnUnidad: %v", err)
	}
}
