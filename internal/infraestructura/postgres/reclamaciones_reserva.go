package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioReclamacionesReserva = (*Store)(nil)

// estadoDeReclamacion mapea [reparto.ReclamacionReserva.Pagable] al vocabulario
// de la tabla `reclamaciones` (migracion 00001), compartida con reclamaciones
// que no son de reserva. 'resuelta' es "lista para el siguiente proceso de
// Distribucion" (RD 14.5.9); el dominio no modela un estado de rechazo
// todavia, asi que no hay 'rechazada' que escribir desde aqui.
func estadoDeReclamacion(r reparto.ReclamacionReserva) string {
	if r.Pagable() {
		return "resuelta"
	}
	return "abierta"
}

// GuardarReclamacion es un upsert de la reclamacion y de cada aval que
// todavia no estuviera guardado, en una sola transaccion. FirmarReclamacion
// (aplicacion) llama a esto con la lista COMPLETA de avales cada vez -los ya
// guardados incluidos-, asi que los avales se insertan con
// ON CONFLICT DO NOTHING: reinsertar uno que ya estaba no es un error.
func (s *Store) GuardarReclamacion(ctx context.Context, r reparto.ReclamacionReserva) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO reclamaciones (id, titular_id, proceso_id, detalle, estado, monto_solicitado)
			 VALUES ($1,$2,$3,$4,$5,$6)
			 ON CONFLICT (id) DO UPDATE SET estado = EXCLUDED.estado`,
			r.ID, r.TitularID, r.ProcesoOrigenID, r.Detalle, estadoDeReclamacion(r), r.MontoSolicitado,
		); err != nil {
			return traducirError(err, "guardar reclamacion %q", r.ID)
		}

		for _, a := range r.Avales {
			if _, err := tx.Exec(ctx,
				`INSERT INTO reclamaciones_avales (reclamacion_id, rol, actor_id)
				 VALUES ($1,$2,$3)
				 ON CONFLICT (reclamacion_id, rol) DO NOTHING`,
				r.ID, string(a.Rol), a.ActorID,
			); err != nil {
				if esClaveForanea(err) {
					return fmt.Errorf("guardar aval de %q: %w", r.ID, aplicacion.ErrNoEncontrado)
				}
				return traducirError(err, "guardar aval de %q", r.ID)
			}
		}
		return nil
	})
}

// ReclamacionPorID relee una reclamacion con sus avales.
func (s *Store) ReclamacionPorID(ctx context.Context, id string) (reparto.ReclamacionReserva, error) {
	var r reparto.ReclamacionReserva
	err := s.pool.QueryRow(ctx,
		`SELECT id, titular_id, COALESCE(proceso_id, ''), detalle, monto_solicitado
		   FROM reclamaciones WHERE id = $1`,
		id,
	).Scan(&r.ID, &r.TitularID, &r.ProcesoOrigenID, &r.Detalle, &r.MontoSolicitado)
	if err != nil {
		return reparto.ReclamacionReserva{}, traducirError(err, "leer reclamacion %q", id)
	}

	filas, err := s.pool.Query(ctx,
		`SELECT rol, actor_id FROM reclamaciones_avales WHERE reclamacion_id = $1 ORDER BY rol`, id)
	if err != nil {
		return reparto.ReclamacionReserva{}, traducirError(err, "leer avales de %q", id)
	}
	for filas.Next() {
		var rol, actorID string
		if err := filas.Scan(&rol, &actorID); err != nil {
			filas.Close()
			return reparto.ReclamacionReserva{}, traducirError(err, "escanear avales de %q", id)
		}
		r.Avales = append(r.Avales, reparto.AvalReclamacion{Rol: reparto.RolAvalReclamacion(rol), ActorID: actorID})
	}
	if err := filas.Err(); err != nil {
		return reparto.ReclamacionReserva{}, traducirError(err, "leer avales de %q", id)
	}

	return r, nil
}
