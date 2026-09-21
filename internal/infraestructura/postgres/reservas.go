package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioReservas = (*Store)(nil)

// CrearReserva inserta la reserva de una corrida. Es de una sola vez: un
// segundo alta para el mismo proceso_id no pisa tasa ni monto en silencio
// (N2), se rechaza con ErrReservaYaRegistrada.
func (s *Store) CrearReserva(ctx context.Context, r reparto.PoolReserva) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO reservas (proceso_id, circuito, monto_inicial, saldo, tasa_pct, organo_aprobador)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		r.ProcesoID, string(r.Circuito), r.MontoInicial, r.Saldo, r.TasaPct, r.OrganoAprobador,
	)
	if esClaveDuplicada(err) {
		return fmt.Errorf("crear reserva de %q: %w", r.ProcesoID, aplicacion.ErrReservaYaRegistrada)
	}
	if esClaveForanea(err) {
		return fmt.Errorf("crear reserva de %q: %w", r.ProcesoID, aplicacion.ErrNoEncontrado)
	}
	return traducirError(err, "crear reserva de %q", r.ProcesoID)
}

// ReservaPorProceso relee la reserva de una corrida.
func (s *Store) ReservaPorProceso(ctx context.Context, procesoID string) (reparto.PoolReserva, error) {
	var r reparto.PoolReserva
	var circuito string
	err := s.pool.QueryRow(ctx,
		`SELECT proceso_id, circuito, monto_inicial, saldo, tasa_pct, organo_aprobador
		   FROM reservas WHERE proceso_id = $1`,
		procesoID,
	).Scan(&r.ProcesoID, &circuito, &r.MontoInicial, &r.Saldo, &r.TasaPct, &r.OrganoAprobador)
	if err != nil {
		return reparto.PoolReserva{}, traducirError(err, "leer reserva de %q", procesoID)
	}
	r.Circuito = reparto.Circuito(circuito)
	return r, nil
}

// LiberarSaldoReserva bloquea reservas y rendimientos, entrega el saldo a fn, y persiste saldo/rendimiento/lineas en una transaccion (B1,B2,B4).
func (s *Store) LiberarSaldoReserva(
	ctx context.Context, procesoID, vigenciaRendimiento string, rendimientoAUsar decimal.Decimal,
	fn func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error),
) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		var saldoActual decimal.Decimal
		if err := tx.QueryRow(ctx, `SELECT saldo FROM reservas WHERE proceso_id = $1 FOR UPDATE`, procesoID).
			Scan(&saldoActual); err != nil {
			return traducirError(err, "bloquear reserva de %q", procesoID)
		}

		if rendimientoAUsar.IsPositive() {
			var disponible decimal.Decimal
			if err := tx.QueryRow(ctx,
				`SELECT monto FROM rendimientos WHERE circuito = 'nacional' AND vigencia = $1 FOR UPDATE`,
				vigenciaRendimiento).Scan(&disponible); err != nil {
				return traducirError(err, "bloquear rendimiento nacional/%s", vigenciaRendimiento)
			}
		}

		nuevoSaldo, lineas, err := fn(saldoActual)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `UPDATE reservas SET saldo = $2 WHERE proceso_id = $1`, procesoID, nuevoSaldo); err != nil {
			return traducirError(err, "actualizar saldo de reserva %q", procesoID)
		}

		if rendimientoAUsar.IsPositive() {
			if _, err := tx.Exec(ctx,
				`UPDATE rendimientos SET monto = monto - $2 WHERE circuito = 'nacional' AND vigencia = $1`,
				vigenciaRendimiento, rendimientoAUsar); err != nil {
				return traducirError(err, "descontar rendimiento nacional/%s", vigenciaRendimiento)
			}
		}

		for _, l := range lineas {
			if _, err := tx.Exec(ctx,
				`INSERT INTO reservas_liberaciones (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				procesoID, l.ObraID, l.TitularID, l.IPI, l.Porcentaje, l.Importe,
			); err != nil {
				return traducirError(err, "guardar linea liberada de %q", procesoID)
			}
		}
		return nil
	})
}
