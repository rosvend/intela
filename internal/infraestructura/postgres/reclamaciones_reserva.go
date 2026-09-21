package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioReclamacionesReserva = (*Store)(nil)

// GuardarReclamacion crea la fila si es la primera vez, inserta los avales
// que todavia no estuvieran guardados, y recalcula `estado` a partir de los
// avales que de verdad quedaron en la base -- no de r.Avales, que puede
// venir de una lectura hecha antes de que otra llamada concurrente
// avalara. El SELECT ... FOR UPDATE al principio serializa dos llamadas
// concurrentes sobre la MISMA reclamacion: la segunda espera a que la
// primera confirme, y entonces su recalculo ve los avales de las dos
// (B4). 'resuelta' es "lista para el siguiente proceso de Distribucion"
// (RD 14.5.9); el dominio no modela un estado de rechazo todavia, asi que
// no hay 'rechazada' que escribir desde aqui.
//
// Al transicionar a 'resuelta' -y solo entonces, una vez- descuenta
// monto_solicitado de reservas.saldo: sin esto, LiberarReservaPrescrita
// repartiria mas tarde el saldo completo como si el reclamo resuelto no
// comprometiera nada, pagando el mismo dinero dos veces (B3). El CHECK
// saldo >= 0 rechaza comprometer mas de lo que queda.
func (s *Store) GuardarReclamacion(ctx context.Context, r reparto.ReclamacionReserva) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO reclamaciones (id, titular_id, proceso_id, detalle, estado, monto_solicitado)
			 VALUES ($1,$2,$3,$4,'abierta',$5)
			 ON CONFLICT (id) DO NOTHING`,
			r.ID, r.TitularID, r.ProcesoOrigenID, r.Detalle, r.MontoSolicitado,
		); err != nil {
			return traducirError(err, "crear reclamacion %q", r.ID)
		}

		var estadoAnterior string
		if err := tx.QueryRow(ctx, `SELECT estado FROM reclamaciones WHERE id = $1 FOR UPDATE`, r.ID).
			Scan(&estadoAnterior); err != nil {
			return traducirError(err, "bloquear reclamacion %q", r.ID)
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

		avales, err := avalesDe(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		nuevoEstado := estadoDeAvales(avales)
		if _, err := tx.Exec(ctx, `UPDATE reclamaciones SET estado = $2 WHERE id = $1`,
			r.ID, nuevoEstado); err != nil {
			return traducirError(err, "actualizar estado de %q", r.ID)
		}

		if estadoAnterior != "resuelta" && nuevoEstado == "resuelta" {
			ct, err := tx.Exec(ctx,
				`UPDATE reservas SET saldo = saldo - $2 WHERE proceso_id = $1`,
				r.ProcesoOrigenID, r.MontoSolicitado)
			if err != nil {
				return traducirError(err, "comprometer reclamacion %q contra la reserva de %q", r.ID, r.ProcesoOrigenID)
			}
			if ct.RowsAffected() == 0 {
				return fmt.Errorf("comprometer reclamacion %q: no hay reserva registrada para %q: %w",
					r.ID, r.ProcesoOrigenID, aplicacion.ErrNoEncontrado)
			}
		}
		return nil
	})
}

// consultor es la parte comun entre *pgxpool.Pool y pgx.Tx que necesita
// avalesDe: a veces lee suelto (ReclamacionPorID) y a veces DENTRO de la
// transaccion de GuardarReclamacion.
type consultor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// avalesDe lee los avales que de verdad estan persistidos para una
// reclamacion. Llamado dentro de una transaccion, ve tambien lo que esa
// misma transaccion acaba de insertar.
func avalesDe(ctx context.Context, q consultor, reclamacionID string) ([]reparto.AvalReclamacion, error) {
	filas, err := q.Query(ctx,
		`SELECT rol, actor_id FROM reclamaciones_avales WHERE reclamacion_id = $1 ORDER BY rol`, reclamacionID)
	if err != nil {
		return nil, traducirError(err, "leer avales de %q", reclamacionID)
	}
	defer filas.Close()

	var avales []reparto.AvalReclamacion
	for filas.Next() {
		var rol, actorID string
		if err := filas.Scan(&rol, &actorID); err != nil {
			return nil, traducirError(err, "escanear avales de %q", reclamacionID)
		}
		avales = append(avales, reparto.AvalReclamacion{Rol: reparto.RolAvalReclamacion(rol), ActorID: actorID})
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer avales de %q", reclamacionID)
	}
	return avales, nil
}

// estadoDeAvales mapea el conjunto de avales persistidos al vocabulario de
// la tabla `reclamaciones` (migracion 00001), compartida con reclamaciones
// que no son de reserva.
func estadoDeAvales(avales []reparto.AvalReclamacion) string {
	var tieneRevisoria, tieneDistribucion bool
	for _, a := range avales {
		switch a.Rol {
		case reparto.RolRevisoriaFiscalOAuditoriaInterna:
			tieneRevisoria = true
		case reparto.RolDistribucionYContabilidad:
			tieneDistribucion = true
		}
	}
	if tieneRevisoria && tieneDistribucion {
		return "resuelta"
	}
	return "abierta"
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

	avales, err := avalesDe(ctx, s.pool, id)
	if err != nil {
		return reparto.ReclamacionReserva{}, err
	}
	r.Avales = avales
	return r, nil
}
