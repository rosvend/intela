package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var _ aplicacion.Similitud = (*Store)(nil)

// maxCandidatos: una bandeja de revision con veinte filas parecidas no se
// revisa, se cierra.
const maxCandidatos = 5

// Candidatos implementa el escalon 3 con pg_trgm sobre el titulo normalizado.
// Ver D1 de docs/planes/32-difuso/diseno.md y explain-trgm.md para el indice.
//
// Va en transaccion por el SET LOCAL: `%` es el operador que usa el indice y su
// corte es el GUC, no un argumento. LOCAL para que la conexion vuelva al pool
// sin el corte puesto. El desempate por id da orden total (ADR 0005).
func (s *Store) Candidatos(ctx context.Context, titulo string, piso decimal.Decimal) ([]identificacion.Candidato, error) {
	var cs []identificacion.Candidato

	err := s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		// set_config y no interpolacion: el valor viaja como parametro. El
		// tercer argumento en true es lo que lo hace LOCAL.
		if _, err := tx.Exec(ctx,
			`SELECT set_config('pg_trgm.similarity_threshold', $1, true)`, piso.String()); err != nil {
			return traducirError(err, "fijar el piso de similitud en %s", piso)
		}

		filas, err := tx.Query(ctx, `
			SELECT id, similarity(titulo_norm, titulo_normalizado($1))
			  FROM obras
			 WHERE titulo_norm % titulo_normalizado($1)
			 ORDER BY similarity(titulo_norm, titulo_normalizado($1)) DESC, id ASC
			 LIMIT $2`, titulo, maxCandidatos)
		if err != nil {
			return traducirError(err, "buscar candidatos para %q", titulo)
		}
		defer filas.Close()

		for filas.Next() {
			var c identificacion.Candidato
			if err := filas.Scan(&c.ObraID, &c.Puntaje); err != nil {
				return traducirError(err, "leer candidatos para %q", titulo)
			}
			cs = append(cs, c)
		}
		// No es opcional: sin esto una lista truncada pasaria por "no se parece
		// a nada", que manda la fila a ONI.
		if err := filas.Err(); err != nil {
			return traducirError(err, "buscar candidatos para %q", titulo)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cs, nil
}
