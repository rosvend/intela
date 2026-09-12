package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/oni"
)

var _ aplicacion.RepositorioPublicacionONI = (*Store)(nil)

const columnasPublicacion = `id::text, periodo, fecha_proceso, direccion_fisica, direccion_electronica`

const columnasItemPublico = `uso_id, titulo, fuente, ids_fuente, modalidad`

// PendientesDePeriodo lee la cola viva, no el listado publicado. Resolver un
// ONI despues no tiene que cambiar lo que ya se congelo.
func (s *Store) PendientesDePeriodo(ctx context.Context, periodo string) ([]oni.DatosIdentificatorios, error) {
	filas, err := s.q(ctx).Query(ctx, `
		SELECT u.id, u.titulo, u.fuente, u.ids_fuente, u.modalidad, r.periodo
		  FROM usos u
		  JOIN reportes r ON r.id = u.reporte_id
		 WHERE u.oni AND r.periodo = $1
		 ORDER BY u.titulo, u.id`, periodo)
	if err != nil {
		return nil, traducirError(err, "ONI pendientes del periodo %q", periodo)
	}
	defer filas.Close()

	var out []oni.DatosIdentificatorios
	for filas.Next() {
		var d oni.DatosIdentificatorios
		if err := filas.Scan(&d.ID, &d.Titulo, &d.Fuente, &d.IDsFuente, &d.Modalidad, &d.Periodo); err != nil {
			return nil, traducirError(err, "escanear ONI pendiente")
		}
		out = append(out, d)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "ONI pendientes del periodo %q", periodo)
	}
	return out, nil
}

func (s *Store) GuardarPublicacion(ctx context.Context, p aplicacion.PublicacionONI) (aplicacion.PublicacionONI, error) {
	err := s.q(ctx).QueryRow(ctx, `
		INSERT INTO oni_publicaciones (periodo, fecha_proceso, direccion_fisica, direccion_electronica)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`,
		p.Periodo, p.FechaProceso, p.DireccionFisica, p.DireccionElectronica,
	).Scan(&p.ID)
	if err != nil {
		if esClaveDuplicada(err) {
			return aplicacion.PublicacionONI{}, fmt.Errorf(
				"publicar periodo %q: %w", p.Periodo, aplicacion.ErrYaPublicado)
		}
		return aplicacion.PublicacionONI{}, traducirError(err, "guardar publicacion ONI del periodo %q", p.Periodo)
	}

	for _, o := range p.Obras {
		if _, err := s.q(ctx).Exec(ctx, `
			INSERT INTO oni_publicacion_items
			  (publicacion_id, uso_id, titulo, fuente, ids_fuente, modalidad)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			p.ID, o.ID, o.Titulo, o.Fuente, o.IDsFuente, o.Modalidad); err != nil {
			return aplicacion.PublicacionONI{}, traducirError(err, "guardar item ONI %q", o.ID)
		}
	}
	if p.Obras == nil {
		p.Obras = []oni.ProyeccionPublica{}
	}
	return p, nil
}

func (s *Store) AnclarPrescripcion(ctx context.Context, usoIDs []string, cuando time.Time) error {
	if len(usoIDs) == 0 {
		return nil
	}
	_, err := s.q(ctx).Exec(ctx, `
		UPDATE usos
		   SET publicado_en = $1
		 WHERE id = ANY($2) AND publicado_en IS NULL`, cuando, usoIDs)
	return traducirError(err, "anclar prescripcion ONI")
}

func (s *Store) PublicacionVigente(ctx context.Context) (aplicacion.PublicacionONI, error) {
	p, err := s.escanearPublicacion(ctx, `
		SELECT `+columnasPublicacion+`
		  FROM oni_publicaciones
		 ORDER BY fecha_proceso DESC, periodo DESC
		 LIMIT 1`, "publicacion ONI vigente")
	if err != nil {
		return aplicacion.PublicacionONI{}, err
	}
	return s.conItems(ctx, p)
}

func (s *Store) PublicacionDePeriodo(ctx context.Context, periodo string) (aplicacion.PublicacionONI, error) {
	p, err := s.escanearPublicacion(ctx, `
		SELECT `+columnasPublicacion+`
		  FROM oni_publicaciones
		 WHERE periodo = $1`, "publicacion ONI del periodo "+periodo, periodo)
	if err != nil {
		return aplicacion.PublicacionONI{}, err
	}
	return s.conItems(ctx, p)
}

func (s *Store) escanearPublicacion(ctx context.Context, sql, que string, args ...any) (aplicacion.PublicacionONI, error) {
	var p aplicacion.PublicacionONI
	err := s.q(ctx).QueryRow(ctx, sql, args...).
		Scan(&p.ID, &p.Periodo, &p.FechaProceso, &p.DireccionFisica, &p.DireccionElectronica)
	if err != nil {
		return aplicacion.PublicacionONI{}, traducirError(err, "%s", que)
	}
	return p, nil
}

func (s *Store) conItems(ctx context.Context, p aplicacion.PublicacionONI) (aplicacion.PublicacionONI, error) {
	filas, err := s.q(ctx).Query(ctx, `
		SELECT `+columnasItemPublico+`
		  FROM oni_publicacion_items
		 WHERE publicacion_id = $1
		 ORDER BY titulo, uso_id`, p.ID)
	if err != nil {
		return aplicacion.PublicacionONI{}, traducirError(err, "items de publicacion %q", p.ID)
	}
	defer filas.Close()

	p.Obras = []oni.ProyeccionPublica{}
	for filas.Next() {
		var o oni.ProyeccionPublica
		if err := filas.Scan(&o.ID, &o.Titulo, &o.Fuente, &o.IDsFuente, &o.Modalidad); err != nil {
			return aplicacion.PublicacionONI{}, traducirError(err, "escanear item de publicacion %q", p.ID)
		}
		o.Periodo = p.Periodo
		p.Obras = append(p.Obras, o)
	}
	if err := filas.Err(); err != nil {
		return aplicacion.PublicacionONI{}, traducirError(err, "items de publicacion %q", p.ID)
	}
	return p, nil
}
