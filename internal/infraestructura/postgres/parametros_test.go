package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

func fecha(aaaammdd string) time.Time {
	t, err := time.Parse(time.DateOnly, aaaammdd)
	if err != nil {
		panic(err)
	}
	return t
}

// parametro inserta una vigencia. hasta vacio significa abierta por arriba.
func parametro(t *testing.T, pool *pgxpool.Pool, clave, valor, desde, hasta string) error {
	t.Helper()
	var hastaArg any
	if hasta != "" {
		hastaArg = fecha(hasta)
	}
	_, err := pool.Exec(t.Context(),
		`INSERT INTO parametros (clave, valor, vigente_desde, vigente_hasta, organo, reglamento)
		 VALUES ($1, $2, $3, $4, 'Consejo Directivo', 'RD-IX-prueba')`,
		clave, decimal.RequireFromString(valor), fecha(desde), hastaArg)
	return err
}

func sembrarParametros(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool := testhelp.Pool(t)
	s := &Store{pool: pool}

	// Dos vigencias contiguas, la segunda abierta por arriba.
	if err := parametro(t, pool, "matching.umbral", "0.50", "2024-01-01", "2025-01-01"); err != nil {
		t.Fatalf("sembrar la primera vigencia: %v", err)
	}
	if err := parametro(t, pool, "matching.umbral", "0.60", "2025-01-01", ""); err != nil {
		t.Fatalf("sembrar la segunda vigencia: %v", err)
	}
	return s, pool
}

// Una respuesta por fecha, rango medio abierto: el dia del relevo ya es de la
// vigencia nueva.
func TestParametroVigenteEligeLaVigenciaQueCubreLaFecha(t *testing.T) {
	s, _ := sembrarParametros(t)

	casos := []struct {
		nombre string
		fecha  string
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
			v, err := s.ParametroVigente(t.Context(), "matching.umbral", fecha(c.fecha))
			if err != nil {
				t.Fatalf("ParametroVigente: %v", err)
			}
			if !v.Equal(decimal.RequireFromString(c.quiero)) {
				t.Fatalf("en %s el valor fue %s, se esperaba %s", c.fecha, v, c.quiero)
			}
		})
	}
}

// ADR 0004: ausente es ausente, nunca cero.
func TestParametroVigenteAntesDeLaPrimeraVigenciaEsError(t *testing.T) {
	s, _ := sembrarParametros(t)

	v, err := s.ParametroVigente(t.Context(), "matching.umbral", fecha("2023-12-31"))
	if !errors.Is(err, ErrParametroSinVigencia) {
		t.Fatalf("err = %v, se esperaba ErrParametroSinVigencia", err)
	}
	if !v.IsZero() {
		t.Fatalf("un error no puede traer valor util: %s", v)
	}
}

// El error nombra la clave y la fecha: hay veintitantas en la tabla.
func TestParametroVigenteNombraLaClaveQueFalta(t *testing.T) {
	s, _ := sembrarParametros(t)

	_, err := s.ParametroVigente(t.Context(), "matching.umbral_banda", fecha("2026-09-21"))
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

// El EXCLUDE es lo que hace que la pregunta tenga una respuesta, y hasta ahora
// nada lo ejercia.
func TestParametrosRechazaDosVigenciasSolapadas(t *testing.T) {
	_, pool := sembrarParametros(t)

	err := parametro(t, pool, "matching.umbral", "0.99", "2024-06-01", "2024-09-01")
	if err == nil {
		t.Fatal("la base acepto dos vigencias solapadas de la misma clave")
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "parametro_sin_solape" {
		t.Fatalf("se esperaba que fallara parametro_sin_solape, fallo: %v", err)
	}
}

// La restriccion es por clave, no global.
func TestParametrosAdmiteClavesDistintasEnLaMismaVigencia(t *testing.T) {
	s, pool := sembrarParametros(t)

	if err := parametro(t, pool, "matching.umbral_banda", "0.45", "2025-01-01", ""); err != nil {
		t.Fatalf("dos claves distintas tendrian que poder convivir: %v", err)
	}
	v, err := s.ParametroVigente(t.Context(), "matching.umbral_banda", fecha("2026-09-21"))
	if err != nil {
		t.Fatalf("ParametroVigente: %v", err)
	}
	if !v.Equal(decimal.RequireFromString("0.45")) {
		t.Fatalf("valor = %s, se esperaba 0.45", v)
	}
}
