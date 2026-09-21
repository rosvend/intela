package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioRendimientos = (*Store)(nil)

// AcrecerRendimiento suma `incremento` al monto de (circuito, vigencia) en
// una sola sentencia: la aritmetica corre en la base, no en Go, asi que dos
// acrecimientos concurrentes se suman y ninguno pisa al otro (B2). Crea la
// fila si es la primera vez.
func (s *Store) AcrecerRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia string, incremento decimal.Decimal) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO rendimientos (circuito, vigencia, monto)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (circuito, vigencia) DO UPDATE SET monto = rendimientos.monto + EXCLUDED.monto`,
		string(circuito), vigencia, incremento,
	)
	return traducirError(err, "acrecer rendimiento %s/%s", circuito, vigencia)
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

// ActualizarMontoRendimiento bloquea la fila, entrega el monto actual a fn,
// y persiste lo que fn devuelva -- misma forma que
// [Store.ActualizarSaldoReserva]. Consumir el monto (dejarlo en el residuo)
// es lo que impide que una segunda distribucion reparta lo mismo otra vez
// (B3).
func (s *Store) ActualizarMontoRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia string, fn func(decimal.Decimal) (decimal.Decimal, error)) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		var montoActual decimal.Decimal
		err := tx.QueryRow(ctx,
			`SELECT monto FROM rendimientos WHERE circuito = $1 AND vigencia = $2 FOR UPDATE`,
			string(circuito), vigencia,
		).Scan(&montoActual)
		if err != nil {
			return traducirError(err, "bloquear rendimiento %s/%s", circuito, vigencia)
		}

		nuevoMonto, err := fn(montoActual)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE rendimientos SET monto = $3 WHERE circuito = $1 AND vigencia = $2`,
			string(circuito), vigencia, nuevoMonto,
		); err != nil {
			return traducirError(err, "actualizar rendimiento %s/%s", circuito, vigencia)
		}
		return nil
	})
}
