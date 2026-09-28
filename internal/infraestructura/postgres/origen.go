package postgres

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.RepositorioOrigenDeUsos = (*Store)(nil)

// OrigenDeUsos lee en un solo viaje el reporte exacto y la identificacion de cada uso; un id ausente no vuelve.
func (s *Store) OrigenDeUsos(ctx context.Context, usoIDs []string) (map[string]aplicacion.OrigenDeUso, error) {
	origen := make(map[string]aplicacion.OrigenDeUso, len(usoIDs))
	if len(usoIDs) == 0 {
		return origen, nil
	}
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT u.id, u.reporte_id, r.fuente, r.sha256, r.clave_objeto, u.escalon,
		        u.puntaje, u.evidencia, COALESCE(u.resuelto_por, ''), u.resuelto_en
		   FROM usos u JOIN reportes r ON r.id = u.reporte_id
		  WHERE u.id = ANY($1)`, usoIDs)
	if err != nil {
		return nil, traducirError(err, "origen de %d usos", len(usoIDs))
	}
	defer filas.Close()

	for filas.Next() {
		var (
			o       aplicacion.OrigenDeUso
			puntaje decimal.NullDecimal
		)
		if err := filas.Scan(&o.UsoID, &o.ReporteID, &o.Fuente, &o.SHA256, &o.ClaveObjeto, &o.Escalon,
			&puntaje, &o.Evidencia, &o.ResueltoPor, &o.ResueltoEn); err != nil {
			return nil, traducirError(err, "escanear origen de uso")
		}
		if puntaje.Valid {
			o.Puntaje = &puntaje.Decimal
		}
		origen[o.UsoID] = o
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "origen de %d usos", len(usoIDs))
	}
	return origen, nil
}
