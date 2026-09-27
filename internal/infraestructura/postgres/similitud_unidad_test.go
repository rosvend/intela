package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// Candidatos abre su transaccion con enTransaccionDe (doc.go, "Limites de
// transaccion"): dentro de una unidad de trabajo corre en la de la unidad, ve lo
// que la unidad escribio sin confirmar, y no confirma por su cuenta.
func TestCandidatosParticipaEnLaUnidad(t *testing.T) {
	// Dos conexiones: EnUnidad ocupa una con la transaccion y el conteo de
	// fuera (pool directo) pide otra. Con el MaxConns=1 de testhelp.Pool es el
	// mismo interbloqueo que TestParametroVigenteParticipaEnLaUnidad: la
	// lectura de fuera espera al pool mientras la unidad espera a la lectura.
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
	s := sembrarEn(t, pool)

	err = s.EnUnidad(t.Context(), func(ctx context.Context) error {
		if _, err := s.ejecutorDe(ctx).Exec(ctx,
			`INSERT INTO obras (id, titulo, genero, anio, tipo)
			 VALUES ('obra-de-la-unidad', 'Quimera Abisal del Otro Lado', 'Drama', 2000, 'serie')`); err != nil {
			return err
		}

		dentro, err := s.Candidatos(ctx, "Quimera Abisal del Otro Lado", pisoDePrueba)
		if err != nil {
			t.Errorf("Candidatos dentro de la unidad: %v", err)
			return nil
		}
		if ids := idsCandidatos(dentro); len(ids) != 1 || ids[0] != "obra-de-la-unidad" {
			t.Errorf("dentro de la unidad tenia que ver la obra recien escrita: %v", ids)
		}

		// Desde otra conexion, la obra no existe todavia: la unidad no confirmo.
		var n int
		if err := pool.QueryRow(t.Context(),
			`SELECT count(*) FROM obras WHERE id = 'obra-de-la-unidad'`).Scan(&n); err != nil {
			t.Errorf("contar desde fuera: %v", err)
			return nil
		}
		if n != 0 {
			t.Errorf("Candidatos confirmo la transaccion de la unidad por su cuenta")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EnUnidad: %v", err)
	}

	// Al terminar la unidad la obra queda confirmada, y el corte LOCAL se fue con
	// ella: la conexion vuelve al pool con el de fabrica.
	fuera, err := s.Candidatos(t.Context(), "Quimera Abisal del Otro Lado", pisoDePrueba)
	if err != nil {
		t.Fatalf("Candidatos fuera de la unidad: %v", err)
	}
	if ids := idsCandidatos(fuera); len(ids) != 1 || ids[0] != "obra-de-la-unidad" {
		t.Fatalf("la unidad confirmo y la obra tenia que verse: %v", ids)
	}
}
