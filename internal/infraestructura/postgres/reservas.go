package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var (
	_ aplicacion.RepositorioReservas = (*Store)(nil)
	_ aplicacion.LecturaDeCorridas   = (*Store)(nil)
)

// CrearReserva inserta la reserva de una corrida. Es de una sola vez: un
// segundo alta para el mismo proceso_id no pisa tasa ni monto en silencio
// (N2), se rechaza con ErrReservaYaRegistrada.
func (s *Store) CrearReserva(ctx context.Context, r reparto.PoolReserva) error {
	_, err := s.ejecutorDe(ctx).Exec(ctx,
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
	err := s.ejecutorDe(ctx).QueryRow(ctx,
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
//
// enTransaccionDe y no EnTransaccion: si el caso de uso abrio una unidad, la
// liberacion entra en ELLA. Con EnTransaccion el saldo se confirmaria aqui y
// un asiento fallido (#177) dejaria dinero liberado sin rastro.
func (s *Store) LiberarSaldoReserva(
	ctx context.Context, procesoID, vigenciaRendimiento string, rendimientoAUsar decimal.Decimal,
	fn func(decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error),
) error {
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		var saldoActual decimal.Decimal
		if err := tx.QueryRow(ctx, `SELECT saldo FROM reservas WHERE proceso_id = $1 FOR UPDATE`, procesoID).
			Scan(&saldoActual); err != nil {
			return traducirError(err, "bloquear reserva de %q", procesoID)
		}

		var disponible decimal.Decimal
		if rendimientoAUsar.IsPositive() {
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

		saldoPersistir := nuevoSaldo
		if rendimientoAUsar.IsPositive() {
			// Las lineas de fn mezclan el saldo de la reserva y el rendimiento.
			// reservas_liberaciones guarda ese total. rendimientos_distribuciones
			// guarda solo la parte del rendimiento, con el mismo peso (el
			// Importe) que DistribuirSobreProporciones. El residuo de esa
			// particion se queda en el ledger: rendimientos.monto baja la suma
			// de las filas (rendimientoAUsar - residuo), no el importe pedido
			// entero. La primera particion ya metio rendimientoAUsar en el
			// saldo nuevo, asi que ese mismo residuo sale de la reserva: no
			// puede quedar en los dos lados. Sin lineas el residuo es el
			// importe completo y no sale del ledger.
			partes, residuo, err := reparto.DistribuirSobreProporciones(rendimientoAUsar, lineas)
			if err != nil {
				return err
			}
			consumido := rendimientoAUsar.Sub(residuo)
			// Un residuo negativo pide un centavo mas del que hay. Si el
			// saldo bloqueado cubre el pedido pero no ese centavo, no se
			// atribuye: las filas suman lo que el ledger si puede soltar.
			// Un pedido mayor que el saldo sigue cayendo en el CHECK
			// (monto >= 0), no en este recorte.
			if consumido.GreaterThan(disponible) && !rendimientoAUsar.GreaterThan(disponible) {
				exceso := consumido.Sub(disponible)
				if err := rebajarImporte(partes, exceso); err != nil {
					return err
				}
				consumido = disponible
				residuo = rendimientoAUsar.Sub(consumido)
			}
			saldoPersistir = nuevoSaldo.Sub(residuo)
			if saldoPersistir.IsNegative() {
				if err := rebajarImporte(lineas, saldoPersistir.Neg()); err != nil {
					return err
				}
				saldoPersistir = decimal.Zero
			}
			if _, err := tx.Exec(ctx,
				`UPDATE rendimientos SET monto = monto - $2 WHERE circuito = 'nacional' AND vigencia = $1`,
				vigenciaRendimiento, consumido); err != nil {
				return traducirError(err, "descontar rendimiento nacional/%s", vigenciaRendimiento)
			}
			for _, l := range partes {
				if _, err := tx.Exec(ctx,
					`INSERT INTO rendimientos_distribuciones
					   (circuito, vigencia, proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
					 VALUES ('nacional', $1, $2, $3, $4, $5, $6, $7)`,
					vigenciaRendimiento, procesoID, l.ObraID, l.TitularID, l.IPI, l.Porcentaje, l.Importe,
				); err != nil {
					return traducirError(err, "guardar distribucion del rendimiento nacional/%s", vigenciaRendimiento)
				}
			}
		}

		if _, err := tx.Exec(ctx, `UPDATE reservas SET saldo = $2 WHERE proceso_id = $1`, procesoID, saldoPersistir); err != nil {
			return traducirError(err, "actualizar saldo de reserva %q", procesoID)
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

// rebajarImporte resta exceso desde el final. Lo usa el residuo que no cabe
// en el ledger ni en el saldo de la reserva: sale de una linea ya calculada
// en vez de dejar el importe en dos sitios o en ninguno.
func rebajarImporte(lineas []reparto.LineaTitular, exceso decimal.Decimal) error {
	for i := len(lineas) - 1; i >= 0 && exceso.IsPositive(); i-- {
		if !lineas[i].Importe.IsPositive() {
			continue
		}
		if lineas[i].Importe.GreaterThanOrEqual(exceso) {
			lineas[i].Importe = lineas[i].Importe.Sub(exceso)
			return nil
		}
		exceso = exceso.Sub(lineas[i].Importe)
		lineas[i].Importe = decimal.Zero
	}
	if exceso.IsPositive() {
		return fmt.Errorf("%w: residuo de rendimiento sin linea de donde restarlo", reparto.ErrRepartoInvalido)
	}
	return nil
}
