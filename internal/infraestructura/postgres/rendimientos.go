package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioRendimientos = (*Store)(nil)

// GuardarRendimiento es un upsert por (circuito, vigencia): RD 10.3 exige
// que cada ledger sea una sola fila que se acrece, nunca dos filas del mismo
// circuito que alguien tendria que sumar aguas abajo.
func (s *Store) GuardarRendimiento(ctx context.Context, p reparto.PoolRendimiento) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO rendimientos (circuito, vigencia, monto)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (circuito, vigencia) DO UPDATE SET monto = EXCLUDED.monto`,
		string(p.Circuito), p.Vigencia, p.Monto,
	)
	return traducirError(err, "guardar rendimiento %s/%s", p.Circuito, p.Vigencia)
}

// PorCircuitoYVigencia relee un ledger.
func (s *Store) PorCircuitoYVigencia(ctx context.Context, circuito reparto.Circuito, vigencia string) (reparto.PoolRendimiento, error) {
	var p reparto.PoolRendimiento
	var c string
	err := s.pool.QueryRow(ctx,
		`SELECT circuito, vigencia, monto FROM rendimientos WHERE circuito = $1 AND vigencia = $2`,
		string(circuito), vigencia,
	).Scan(&c, &p.Vigencia, &p.Monto)
	if err != nil {
		return reparto.PoolRendimiento{}, traducirError(err, "leer rendimiento %s/%s", circuito, vigencia)
	}
	p.Circuito = reparto.Circuito(c)
	return p, nil
}
