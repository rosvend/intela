package postgres

import (
	"context"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioReservas = (*Store)(nil)

// GuardarReserva es un upsert por proceso_id: [aplicacion.BolsasAccesorias]
// abre la fila en RegistrarReserva y la vuelve a guardar en
// LiberarReservaPrescrita con el saldo en cero. Las dos son la misma
// operacion de persistencia -- la reserva de una corrida es una fila, no un
// historial de eventos.
func (s *Store) GuardarReserva(ctx context.Context, r reparto.PoolReserva) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO reservas (proceso_id, circuito, monto_inicial, saldo, tasa_pct, organo_aprobador)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (proceso_id) DO UPDATE SET saldo = EXCLUDED.saldo`,
		r.ProcesoID, string(r.Circuito), r.MontoInicial, r.Saldo, r.TasaPct, r.OrganoAprobador,
	)
	if esClaveForanea(err) {
		return fmt.Errorf("guardar reserva de %q: %w", r.ProcesoID, aplicacion.ErrNoEncontrado)
	}
	return traducirError(err, "guardar reserva de %q", r.ProcesoID)
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
