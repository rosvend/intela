package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.AlmacenVectorial = (*Store)(nil)

// IndexarSecciones borra el indice del modelo y lo vuelve a escribir en una transaccion: o el indice viejo o el nuevo, nunca una mezcla.
func (s *Store) IndexarSecciones(ctx context.Context, modelo string, secciones []aplicacion.SeccionIndexada) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM reglamento_secciones WHERE modelo = $1`, modelo); err != nil {
			return traducirError(err, "vaciar el indice del reglamento %q", modelo)
		}
		// El vector viaja como texto '[...]' y lo convierte el cast: sin dependencia pgvector-go.
		for _, sec := range secciones {
			if _, err := tx.Exec(ctx,
				`INSERT INTO reglamento_secciones (modelo, cita, reglamento, titulo, texto, embedding)
				 VALUES ($1, $2, $3, $4, $5, $6::vector)`,
				modelo, sec.Cita, sec.Reglamento, sec.Titulo, sec.Texto, literalVector(sec.Vector)); err != nil {
				return traducirError(err, "indexar %s", sec.Cita)
			}
		}
		return nil
	})
}

// BuscarSecciones ordena por distancia coseno (<=>) dentro del mismo modelo; similitud = 1 - distancia.
func (s *Store) BuscarSecciones(ctx context.Context, e aplicacion.Embedding, topK int) ([]aplicacion.CoincidenciaReglamento, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT cita, reglamento, titulo, texto, 1 - (embedding <=> $2::vector)
		   FROM reglamento_secciones
		  WHERE modelo = $1 AND vector_dims(embedding) = $3
		  ORDER BY embedding <=> $2::vector, cita
		  LIMIT $4`,
		e.Modelo, literalVector(e.Vector), len(e.Vector), topK)
	if err != nil {
		return nil, traducirError(err, "buscar en el reglamento")
	}
	defer filas.Close()

	var out []aplicacion.CoincidenciaReglamento
	for filas.Next() {
		var c aplicacion.CoincidenciaReglamento
		if err := filas.Scan(&c.Seccion.Cita, &c.Seccion.Reglamento, &c.Seccion.Titulo, &c.Seccion.Texto, &c.Similitud); err != nil {
			return nil, traducirError(err, "leer coincidencias del reglamento")
		}
		out = append(out, c)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer coincidencias del reglamento")
	}
	return out, nil
}

// literalVector escribe el formato de entrada de pgvector: [1,2.5,-3].
func literalVector(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
