package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.BitacoraAuditoria = (*Store)(nil)

// id::text: la columna es UUID nativo, y el cast a texto es lo que deja
// escanear el resultado directo a un string de Go sin arrastrar un tipo de
// pgx hasta aplicacion, que no sabe que existe un UUID.
const columnasAsiento = `id::text, hecho, ref_tipo, ref_id, COALESCE(actor_id, ''), payload, cuando`

// Asentar escribe en `asientos`. La tabla es append-only por trigger
// (`asientos_inmutables`, migracion 00001): este adaptador no tiene -ni
// necesita- un Actualizar ni un Borrar, la base los rechaza por si sola.
//
// El id lo genera la base (DEFAULT gen_random_uuid()) y no el asiento que
// llega: derivarlo del hecho convertiria un INSERT idempotente en perdida
// silenciosa de asientos, que es justo lo que el comentario de la migracion
// advierte.
func (s *Store) Asentar(ctx context.Context, a aplicacion.Asiento) error {
	return asentar(ctx, s.pool, a)
}

// asentar es el INSERT que comparten [Store.Asentar] -suelto, contra el
// pool- y cualquier otro puerto que necesite el mismo asiento DENTRO de su
// propia transaccion -ver [Store.Guardar] en declaraciones.go, que lo corre
// contra una pgx.Tx para que la version y el asiento sean una sola operacion
// (ADR 0006)-. ejecutor es la parte de *pgxpool.Pool y pgx.Tx que este INSERT
// necesita; cual de los dos llega lo decide quien llama.
func asentar(ctx context.Context, ex ejecutor, a aplicacion.Asiento) error {
	_, err := ex.Exec(ctx,
		`INSERT INTO asientos (hecho, ref_tipo, ref_id, actor_id, payload, cuando)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)`,
		a.Hecho, a.RefTipo, a.RefID, a.ActorID, a.Payload, a.Cuando)
	if err != nil {
		return traducirError(err, "asentar %q sobre %s %q", a.Hecho, a.RefTipo, a.RefID)
	}
	return nil
}

// De devuelve los asientos de una referencia, del mas antiguo al mas nuevo:
// es el orden en el que ocurrieron los hechos, y es el que espera
// ExplicarCifra para reconstruir una cadena.
func (s *Store) De(ctx context.Context, refTipo, refID string) ([]aplicacion.Asiento, error) {
	filas, err := s.pool.Query(ctx,
		`SELECT `+columnasAsiento+` FROM asientos
		  WHERE ref_tipo = $1 AND ref_id = $2 ORDER BY cuando`,
		refTipo, refID)
	if err != nil {
		return nil, traducirError(err, "asientos de %s %q", refTipo, refID)
	}
	defer filas.Close()

	var asientos []aplicacion.Asiento
	for filas.Next() {
		var a aplicacion.Asiento
		if err := filas.Scan(&a.ID, &a.Hecho, &a.RefTipo, &a.RefID, &a.ActorID, &a.Payload, &a.Cuando); err != nil {
			return nil, traducirError(err, "escanear asiento de %s %q", refTipo, refID)
		}
		asientos = append(asientos, a)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "asientos de %s %q", refTipo, refID)
	}
	return asientos, nil
}

func (s *Store) AsientoPorID(ctx context.Context, id string) (aplicacion.Asiento, error) {
	var a aplicacion.Asiento
	err := s.pool.QueryRow(ctx,
		`SELECT `+columnasAsiento+` FROM asientos WHERE id = $1`, id).
		Scan(&a.ID, &a.Hecho, &a.RefTipo, &a.RefID, &a.ActorID, &a.Payload, &a.Cuando)
	if err != nil {
		return aplicacion.Asiento{}, traducirError(err, "asiento %q", id)
	}
	return a, nil
}
