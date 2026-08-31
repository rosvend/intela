package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

var _ aplicacion.RepositorioAdmision = (*Store)(nil)

const columnasAfiliacion = `id, nombre, email, documento_identidad, ipi, subtipo, estado,
	persona_natural, pertenece_otra_sgc, clave_rut, clave_cert_bancaria,
	clave_renuncia, COALESCE(titular_id, '')`

func escanearAfiliado(fila pgx.Row) (afiliacion.Afiliado, error) {
	var (
		a       afiliacion.Afiliado
		subtipo string
		estado  string
	)
	err := fila.Scan(
		&a.ID, &a.Nombre, &a.Email, &a.DocumentoIdentidad, &a.IPI,
		&subtipo, &estado, &a.PersonaNatural, &a.PerteneceOtraSGC,
		&a.ClaveRUT, &a.ClaveCertBancaria, &a.ClaveRenuncia, &a.TitularID,
	)
	a.Subtipo = afiliacion.Subtipo(subtipo)
	a.Estado = afiliacion.Estado(estado)
	return a, err
}

func (s *Store) GuardarSolicitud(ctx context.Context, a afiliacion.Afiliado) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO afiliaciones (
			id, nombre, email, documento_identidad, ipi, subtipo, estado,
			persona_natural, pertenece_otra_sgc, clave_rut, clave_cert_bancaria,
			clave_renuncia
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, a.Nombre, a.Email, a.DocumentoIdentidad, a.IPI,
		string(a.Subtipo), string(a.Estado), a.PersonaNatural, a.PerteneceOtraSGC,
		a.ClaveRUT, a.ClaveCertBancaria, a.ClaveRenuncia,
	)
	if err != nil {
		if esConflictoUnico(err) {
			return fmt.Errorf("guardar solicitud %q: %w", a.ID, aplicacion.ErrConflicto)
		}
		return traducirError(err, "guardar solicitud %q", a.ID)
	}
	return nil
}

func (s *Store) SolicitudPorID(ctx context.Context, id string) (afiliacion.Afiliado, error) {
	fila := s.pool.QueryRow(ctx,
		`SELECT `+columnasAfiliacion+` FROM afiliaciones WHERE id = $1`, id)
	a, err := escanearAfiliado(fila)
	if err != nil {
		return afiliacion.Afiliado{}, traducirError(err, "solicitud por id %q", id)
	}
	return a, nil
}

// AdmitirSolicitud escribe el titular del padron y marca la solicitud
// admitida en la misma transaccion: una de las dos a medias dejaria a
// alguien cobrando sin haber sido admitido, o admitido sin fila de cobro.
func (s *Store) AdmitirSolicitud(ctx context.Context, a afiliacion.Afiliado) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO titulares (id, nombre, ipi, persona_natural, clase, email)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			a.TitularID, a.Nombre, a.IPI, a.PersonaNatural, string(a.Subtipo), a.Email,
		)
		if err != nil {
			if esConflictoUnico(err) {
				return fmt.Errorf("crear titular %q: %w", a.TitularID, aplicacion.ErrConflicto)
			}
			return traducirError(err, "crear titular %q", a.TitularID)
		}

		tag, err := tx.Exec(ctx, `
			UPDATE afiliaciones
			   SET estado = $2, titular_id = $3, resuelto = now()
			 WHERE id = $1 AND estado = 'pendiente'`,
			a.ID, string(a.Estado), a.TitularID,
		)
		if err != nil {
			return traducirError(err, "admitir solicitud %q", a.ID)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("admitir solicitud %q: %w", a.ID, afiliacion.ErrEstadoInvalido)
		}
		return nil
	})
}
