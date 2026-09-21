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
// y persiste el monto nuevo y cada linea de fn en rendimientos_distribuciones
// -- misma forma que [Store.LiberarSaldoReserva]. Consumir el monto es lo
// que impide que una segunda distribucion reparta lo mismo otra vez (B3);
// persistir las lineas evita perder el rastro si algo falla despues del
// commit (B2).
func (s *Store) ActualizarMontoRendimiento(
	ctx context.Context, circuito reparto.Circuito, vigencia, procesoID string,
	fn func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error),
) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		var montoActual decimal.Decimal
		if err := tx.QueryRow(ctx,
			`SELECT monto FROM rendimientos WHERE circuito = $1 AND vigencia = $2 FOR UPDATE`,
			string(circuito), vigencia,
		).Scan(&montoActual); err != nil {
			return traducirError(err, "bloquear rendimiento %s/%s", circuito, vigencia)
		}

		nuevoMonto, lineas, err := fn(montoActual)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE rendimientos SET monto = $3 WHERE circuito = $1 AND vigencia = $2`,
			string(circuito), vigencia, nuevoMonto,
		); err != nil {
			return traducirError(err, "actualizar rendimiento %s/%s", circuito, vigencia)
		}

		for _, l := range lineas {
			if _, err := tx.Exec(ctx,
				`INSERT INTO rendimientos_distribuciones
				   (circuito, vigencia, proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
				string(circuito), vigencia, procesoID, l.ObraID, l.TitularID, l.IPI, l.Porcentaje, l.Importe,
			); err != nil {
				return traducirError(err, "guardar linea distribuida %s/%s", circuito, vigencia)
			}
		}
		return nil
	})
}
