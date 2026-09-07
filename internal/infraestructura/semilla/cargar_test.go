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
	"github.com/rosvend/intela/internal/dominio/repertorio"
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

// TestCargarDejaElCatalogoLegible cruza las dos mitades que nadie cruzaba: el
// seed ESCRIBE el catalogo y el caso de uso lo LEE.
//
// Sin el cruce, el seed pasaba verde escribiendo `obras` sin `obra_coautores`
// y GET /obras devolvia 500 en las cuatro obras -"una obra del catalogo
// necesita al menos un coautor con IPI"-, porque la lectura reconstruye la
// entidad por el mismo constructor que la crea. Las pruebas de este paquete
// solo miraban que el dataset cargara, y las de postgres/catalogo_test.go solo
// el adaptador con sus propios fixtures.
func TestCargarDejaElCatalogoLegible(t *testing.T) {
	store, _ := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	catalogo := aplicacion.Catalogo{Obras: store}

	obras, err := catalogo.BuscarObras(ctx, aplicacion.FiltroObras{})
	if err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	esperadas := []string{ObraCine, ObraSerie, ObraSketch, ObraUnitario} // ORDER BY id
	if len(obras) != len(esperadas) {
		t.Fatalf("BuscarObras devolvio %d obras, se esperaban %d", len(obras), len(esperadas))
	}
	for i, id := range esperadas {
		if obras[i].ID() != id {
			t.Fatalf("obra %d: %q, se esperaba %q", i, obras[i].ID(), id)
		}
	}

	// Cada obra tambien por su propia ruta, que es el GET /obras/{id}.
	for _, id := range esperadas {
		obra, err := catalogo.ObraPorID(ctx, id)
		if err != nil {
			t.Fatalf("ObraPorID(%q): %v", id, err)
		}
		coautores := obra.Coautores()
		if len(coautores) == 0 {
			t.Fatalf("obra %q sin coautores: el dominio la rechaza al leerla", id)
		}
		for _, c := range coautores {
			if c.IPI == "" {
				t.Fatalf("obra %q: coautor %q sin IPI", id, c.Nombre)
			}
		}
	}

	// El filtro por IPI resuelve contra obra_coautores: es la comprobacion de
	// que las filas estan ahi y no solo de que la lectura no revienta.
	deCarla, err := catalogo.BuscarObras(ctx, aplicacion.FiltroObras{IPI: "IPI-00000003"})
	if err != nil {
		t.Fatalf("BuscarObras por IPI: %v", err)
	}
	if len(deCarla) != 1 || deCarla[0].ID() != ObraUnitario {
		t.Fatalf("obras de IPI-00000003 = %v, se esperaba solo %s", ids(deCarla), ObraUnitario)
	}
}

func ids(obras []repertorio.Obra) []string {
	out := make([]string, len(obras))
	for i, o := range obras {
		out[i] = o.ID()
	}
	return out
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
