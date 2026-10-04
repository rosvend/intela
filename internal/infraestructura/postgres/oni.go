package postgres

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/oni"
)

var _ aplicacion.RepositorioPublicacionONI = (*Store)(nil)

const columnasPublicacion = `id::text, periodo, fecha_proceso, direccion_fisica, direccion_electronica, secuencia`

const columnasItemPublico = `i.uso_id, i.titulo, i.fuente, i.ids_fuente, i.modalidad, pub.fecha_proceso`

// BloquearPeriodoONI serializa la publicacion ONI del periodo hasta el fin de la transaccion.
func (s *Store) BloquearPeriodoONI(ctx context.Context, periodo string) error {
	tx, hay := txDe(ctx)
	if !hay {
		return fmt.Errorf("bloquear periodo ONI %s: %w", periodo, errFueraDeUnidad)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte("oni_publicacion\x00" + periodo))
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(h.Sum64())); err != nil {
		return traducirError(err, "bloquear periodo ONI %s", periodo)
	}
	return nil
}

// PendientesDePeriodo lee la cola viva, no el listado publicado. Resolver un
// ONI despues no tiene que cambiar lo que ya se congelo.
//
// El corte es escalon = 'oni', no la bandera oni. La ingesta siembra
// escalon = 'pendiente' con oni = TRUE antes de que corra la cascada; esa
// fila todavia no es un ONI y no puede congelarse ni arrancar R-19.
// Solo se devuelven los usos no publicados (publicado_en IS NULL): los ya
// publicados ya anclaron su prescripcion y no deben volver a publicarse.
func (s *Store) PendientesDePeriodo(ctx context.Context, periodo string) ([]oni.DatosIdentificatorios, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		SELECT u.id, u.titulo, u.fuente, u.ids_fuente, u.modalidad, r.periodo
		  FROM usos u
		  JOIN reportes r ON r.id = u.reporte_id
		 WHERE u.escalon = 'oni' AND r.periodo = $1 AND u.publicado_en IS NULL
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
	err := s.ejecutorDe(ctx).QueryRow(ctx, `
		INSERT INTO oni_publicaciones (periodo, fecha_proceso, direccion_fisica, direccion_electronica, secuencia)
		VALUES (
			$1, $2, $3, $4,
			COALESCE((SELECT MAX(secuencia) FROM oni_publicaciones WHERE periodo = $1), 0) + 1
		)
		RETURNING id::text, secuencia`,
		p.Periodo, p.FechaProceso, p.DireccionFisica, p.DireccionElectronica,
	).Scan(&p.ID, &p.Secuencia)
	if err != nil {
		if esClaveDuplicada(err) {
			return aplicacion.PublicacionONI{}, fmt.Errorf(
				"publicar periodo %q: %w", p.Periodo, aplicacion.ErrYaPublicado)
		}
		return aplicacion.PublicacionONI{}, traducirError(err, "guardar publicacion ONI del periodo %q", p.Periodo)
	}

	for _, o := range p.Obras {
		if _, err := s.ejecutorDe(ctx).Exec(ctx, `
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
	_, err := s.ejecutorDe(ctx).Exec(ctx, `
		UPDATE usos
		   SET publicado_en = $1
		 WHERE id = ANY($2) AND publicado_en IS NULL`, cuando, usoIDs)
	return traducirError(err, "anclar prescripcion ONI")
}

func (s *Store) PublicacionVigente(ctx context.Context) (aplicacion.PublicacionONI, error) {
	p, err := s.escanearPublicacion(ctx, `
		SELECT `+columnasPublicacion+`
		  FROM oni_publicaciones
		 ORDER BY periodo DESC, secuencia DESC
		 LIMIT 1`, "publicacion ONI vigente")
	if err != nil {
		return aplicacion.PublicacionONI{}, err
	}
	return s.conItemsDePeriodo(ctx, p)
}

func (s *Store) PublicacionDePeriodo(ctx context.Context, periodo string) (aplicacion.PublicacionONI, error) {
	p, err := s.escanearPublicacion(ctx, `
		SELECT `+columnasPublicacion+`
		  FROM oni_publicaciones
		 WHERE periodo = $1
		 ORDER BY secuencia DESC
		 LIMIT 1`, "publicacion ONI del periodo "+periodo, periodo)
	if err != nil {
		return aplicacion.PublicacionONI{}, err
	}
	return s.conItemsDePeriodo(ctx, p)
}

func (s *Store) escanearPublicacion(ctx context.Context, sql, que string, args ...any) (aplicacion.PublicacionONI, error) {
	var p aplicacion.PublicacionONI
	err := s.ejecutorDe(ctx).QueryRow(ctx, sql, args...).
		Scan(&p.ID, &p.Periodo, &p.FechaProceso, &p.DireccionFisica, &p.DireccionElectronica, &p.Secuencia)
	if err != nil {
		return aplicacion.PublicacionONI{}, traducirError(err, "%s", que)
	}
	return p, nil
}

func (s *Store) conItemsDePeriodo(ctx context.Context, p aplicacion.PublicacionONI) (aplicacion.PublicacionONI, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		SELECT `+columnasItemPublico+`
		  FROM oni_publicacion_items i
		  JOIN oni_publicaciones pub ON pub.id = i.publicacion_id
		 WHERE pub.periodo = $1
		 ORDER BY i.titulo, i.uso_id`, p.Periodo)
	if err != nil {
		return aplicacion.PublicacionONI{}, traducirError(err, "items de publicacion del periodo %q", p.Periodo)
	}
	defer filas.Close()

	p.Obras = []oni.ProyeccionPublica{}
	for filas.Next() {
		var o oni.ProyeccionPublica
		var ancla time.Time
		if err := filas.Scan(&o.ID, &o.Titulo, &o.Fuente, &o.IDsFuente, &o.Modalidad, &ancla); err != nil {
			return aplicacion.PublicacionONI{}, traducirError(err, "escanear item de publicacion del periodo %q", p.Periodo)
		}
		o.Periodo = p.Periodo
		o.FechaProceso = ancla.UTC().Format(time.RFC3339)
		p.Obras = append(p.Obras, o)
	}
	if err := filas.Err(); err != nil {
		return aplicacion.PublicacionONI{}, traducirError(err, "items de publicacion del periodo %q", p.Periodo)
	}
	return p, nil
}
