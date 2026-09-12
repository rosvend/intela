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

func (s *Store) GuardarSolicitud(ctx context.Context, a afiliacion.Afiliado, claveHash string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO afiliaciones (
			id, nombre, email, documento_identidad, ipi, subtipo, estado,
			persona_natural, pertenece_otra_sgc, clave_rut, clave_cert_bancaria,
			clave_renuncia, clave_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		a.ID, a.Nombre, a.Email, a.DocumentoIdentidad, a.IPI,
		string(a.Subtipo), string(a.Estado), a.PersonaNatural, a.PerteneceOtraSGC,
		a.ClaveRUT, a.ClaveCertBancaria, a.ClaveRenuncia, claveHash,
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

// AdmitirSolicitud escribe el titular del padron, la cuenta con la que
// entra, y marca la solicitud admitida, las tres cosas en la misma
// transaccion: un padron sin credenciales deja al socio fuera, y una
// cuenta sin fila de cobro cobra a nadie.
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

		var claveHash string
		if err := tx.QueryRow(ctx,
			`SELECT clave_hash FROM afiliaciones WHERE id = $1`, a.ID,
		).Scan(&claveHash); err != nil {
			return traducirError(err, "clave de solicitud %q", a.ID)
		}

		usuarioID := "usr-" + a.TitularID
		_, err = tx.Exec(ctx, `
			INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash)
			VALUES ($1, $2, $3, 'titular', $4, $5)`,
			usuarioID, a.Email, a.Nombre, a.TitularID, claveHash,
		)
		if err != nil {
			if esConflictoUnico(err) {
				return fmt.Errorf("crear usuario de titular %q: %w", usuarioID, aplicacion.ErrConflicto)
			}
			return traducirError(err, "crear usuario de titular %q", usuarioID)
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

// ActualizarPendiente persiste IPI o rechazo sobre una fila que sigue
// pendiente. El WHERE cierra la carrera con AdmitirSolicitud.
func (s *Store) ActualizarPendiente(ctx context.Context, a afiliacion.Afiliado) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE afiliaciones
		   SET ipi = $2,
		       estado = $3,
		       resuelto = CASE WHEN $3 <> 'pendiente' THEN now() ELSE resuelto END
		 WHERE id = $1 AND estado = 'pendiente'`,
		a.ID, a.IPI, string(a.Estado),
	)
	if err != nil {
		if esConflictoUnico(err) {
			return fmt.Errorf("actualizar solicitud %q: %w", a.ID, aplicacion.ErrConflicto)
		}
		return traducirError(err, "actualizar solicitud %q", a.ID)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("actualizar solicitud %q: %w", a.ID, afiliacion.ErrEstadoInvalido)
	}
	return nil
}
