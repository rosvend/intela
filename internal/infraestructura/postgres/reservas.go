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

// ActualizarSaldoReserva bloquea la fila con SELECT ... FOR UPDATE, le pasa
// el saldo actual a fn, y persiste lo que fn devuelva -- todo en una
// transaccion. Una segunda llamada concurrente se bloquea en el FOR UPDATE
// hasta que la primera confirme, y entonces ve el saldo YA actualizado: es
// lo que impide que dos liberaciones lean el mismo saldo y repartan las dos
// (B1). Si fn devuelve error, la transaccion no confirma nada (B5: no se
// persiste un saldo a medias).
func (s *Store) ActualizarSaldoReserva(ctx context.Context, procesoID string, fn func(decimal.Decimal) (decimal.Decimal, error)) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		var saldoActual decimal.Decimal
		err := tx.QueryRow(ctx, `SELECT saldo FROM reservas WHERE proceso_id = $1 FOR UPDATE`, procesoID).
			Scan(&saldoActual)
		if err != nil {
			return traducirError(err, "bloquear reserva de %q", procesoID)
		}

		nuevoSaldo, err := fn(saldoActual)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `UPDATE reservas SET saldo = $2 WHERE proceso_id = $1`, procesoID, nuevoSaldo); err != nil {
			return traducirError(err, "actualizar saldo de reserva %q", procesoID)
		}
		return nil
	})
}
