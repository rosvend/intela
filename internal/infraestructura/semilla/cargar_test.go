package semilla

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/cripto"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
	"github.com/rosvend/intela/internal/infraestructura/postgres"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

func TestCargarSiembraElJuegoCompleto(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()

	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	decls, err := store.Declaraciones(ctx)
	if err != nil {
		t.Fatalf("Declaraciones: %v", err)
	}
	if got := decls[ObraCine].Estado(); got != "completa" {
		t.Fatalf("obra cine Estado() = %q, se esperaba completa", got)
	}
	if got := decls[ObraSerie].Estado(); got != "incompleta" {
		t.Fatalf("obra serie Estado() = %q, se esperaba incompleta", got)
	}
	if got := decls[ObraUnitario].Estado(); got != "completa" {
		t.Fatalf("obra unitario Estado() = %q, se esperaba completa", got)
	}
	if n := len(decls[ObraUnitario].Partes); n != 3 {
		t.Fatalf("obra unitario: %d coautores, se esperaban 3", n)
	}

	usos, err := store.UsosDePeriodo(ctx, Periodo)
	if err != nil {
		t.Fatalf("UsosDePeriodo: %v", err)
	}
	modalidad := map[reparto.Modalidad]int{}
	for _, u := range usos {
		modalidad[u.Modalidad]++
		if u.ONI {
			t.Fatalf("uso %s quedo en ONI", u.ID)
		}
	}
	for _, m := range []reparto.Modalidad{reparto.TV, reparto.Cine, reparto.OTT} {
		if modalidad[m] == 0 {
			t.Fatalf("no hay usos de %s en el periodo", m)
		}
	}

	var nSinteticos int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM parametros WHERE reglamento = $1`, ReglamentoSintetico,
	).Scan(&nSinteticos); err != nil {
		t.Fatalf("contar parametros sinteticos: %v", err)
	}
	if nSinteticos != 3 {
		t.Fatalf("parametros con %s: %d, se esperaban 3 (Wa/Wb/Wc)", ReglamentoSintetico, nSinteticos)
	}

	var nBolsas int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bolsas`).Scan(&nBolsas); err != nil {
		t.Fatalf("contar bolsas: %v", err)
	}
	if nBolsas != 4 {
		t.Fatalf("bolsas = %d, se esperaban 4", nBolsas)
	}
}

func TestCargarEsIdempotenteSinReset(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	cargar := func() error {
		return Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio())
	}
	if err := cargar(); err != nil {
		t.Fatalf("primera carga: %v", err)
	}
	if err := cargar(); err != nil {
		t.Fatalf("segunda carga sin reset: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM obras`).Scan(&n); err != nil {
		t.Fatalf("contar obras: %v", err)
	}
	if n != 4 {
		t.Fatalf("obras = %d despues de recargar, se esperaban 4 (no duplicar)", n)
	}
}

func TestCargarConResetReescribe(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("carga inicial: %v", err)
	}
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), true, silencio()); err != nil {
		t.Fatalf("carga con reset: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM obras`).Scan(&n); err != nil {
		t.Fatalf("contar obras: %v", err)
	}
	if n != 4 {
		t.Fatalf("obras = %d tras reset, se esperaban 4", n)
	}
}

func TestCargarResetRechazaSiHayAsientos(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("carga inicial: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO asientos (hecho, ref_tipo, ref_id) VALUES ('prueba', 'obra', $1)`,
		ObraCine); err != nil {
		t.Fatalf("insertar asiento: %v", err)
	}

	err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), true, silencio())
	if !errors.Is(err, ErrBitacoraNoVacia) {
		t.Fatalf("se esperaba ErrBitacoraNoVacia, se obtuvo %v", err)
	}
}

func abrir(t *testing.T) (*postgres.Store, *pgxpool.Pool) {
	t.Helper()
	pool := testhelp.Pool(t)
	return postgres.Nuevo(pool), pool
}

func disco(t *testing.T) aplicacion.AlmacenObjetos {
	t.Helper()
	return objetos.Disco{Dir: t.TempDir()}
}

func hasher() aplicacion.Hasher {
	return cripto.Bcrypt{Coste: bcrypt.MinCost}
}

func clavesPrueba() Claves {
	return Claves{
		Admin:        "admin-local",
		Distribucion: "distribucion-local",
		Contabilidad: "contabilidad-local",
		Auditor:      "auditor-local",
		Titular:      "ana-local",
	}
}

func silencio() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
