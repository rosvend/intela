package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.BitacoraAuditoria = (*Store)(nil)

func (s *Store) Asentar(ctx context.Context, a aplicacion.Asiento) error {
	payload := a.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, err := s.q(ctx).Exec(ctx, `
		INSERT INTO asientos (hecho, ref_tipo, ref_id, actor_id, payload, cuando)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
		a.Hecho, a.RefTipo, a.RefID, a.ActorID, payload, a.Cuando)
	return traducirError(err, "asentar %q", a.Hecho)
}

func (s *Store) De(ctx context.Context, refTipo, refID string) ([]aplicacion.Asiento, error) {
	filas, err := s.q(ctx).Query(ctx, `
		SELECT id::text, hecho, ref_tipo, ref_id, COALESCE(actor_id, ''), payload, cuando
		  FROM asientos
		 WHERE ref_tipo = $1 AND ref_id = $2
		 ORDER BY cuando, id`, refTipo, refID)
	if err != nil {
		return nil, traducirError(err, "asientos de %s/%s", refTipo, refID)
	}
	defer filas.Close()

	var out []aplicacion.Asiento
	for filas.Next() {
		var a aplicacion.Asiento
		if err := filas.Scan(&a.ID, &a.Hecho, &a.RefTipo, &a.RefID, &a.ActorID, &a.Payload, &a.Cuando); err != nil {
			return nil, traducirError(err, "escanear asiento")
		}
		out = append(out, a)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "asientos de %s/%s", refTipo, refID)
	}
	return out, nil
}

func (s *Store) PorID(ctx context.Context, id string) (aplicacion.Asiento, error) {
	var a aplicacion.Asiento
	err := s.q(ctx).QueryRow(ctx, `
		SELECT id::text, hecho, ref_tipo, ref_id, COALESCE(actor_id, ''), payload, cuando
		  FROM asientos WHERE id = $1`, id).
		Scan(&a.ID, &a.Hecho, &a.RefTipo, &a.RefID, &a.ActorID, &a.Payload, &a.Cuando)
	if err != nil {
		return aplicacion.Asiento{}, traducirError(err, "asiento %q", id)
	}
	return a, nil
}
