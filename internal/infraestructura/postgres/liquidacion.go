package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.RepositorioLiquidacion = (*Store)(nil)

// DeTitular lee las lineas de corrida del titular, con los totales del
// proceso para prorratear deducciones aguas arriba.
//
// periodo vacio es todos. Cero filas no es ErrNoEncontrado.
func (s *Store) DeTitular(ctx context.Context, titularID, periodo string) ([]aplicacion.FilaLiquidacion, error) {
	const q = `
		SELECT p.periodo, rt.obra_id, o.titulo, rt.importe,
		       rp.bruto, rp.admin, rp.social, rp.reserva, rp.neto
		FROM resultados_titular rt
		JOIN procesos p ON p.id = rt.proceso_id
		JOIN resultados_proceso rp ON rp.proceso_id = rt.proceso_id
		JOIN obras o ON o.id = rt.obra_id
		WHERE rt.titular_id = $1
		  AND ($2 = '' OR p.periodo = $2)
		ORDER BY p.periodo, rt.obra_id`

	filas, err := s.pool.Query(ctx, q, titularID, periodo)
	if err != nil {
		return nil, traducirError(err, "liquidacion de titular %q", titularID)
	}
	defer filas.Close()

	var out []aplicacion.FilaLiquidacion
	for filas.Next() {
		var f aplicacion.FilaLiquidacion
		if err := filas.Scan(
			&f.Periodo, &f.ObraID, &f.Titulo, &f.Neto,
			&f.ProcesoBruto, &f.ProcesoAdmin, &f.ProcesoSocial, &f.ProcesoReserva, &f.ProcesoNeto,
		); err != nil {
			return nil, traducirError(err, "escanear liquidacion de titular %q", titularID)
		}
		out = append(out, f)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "liquidacion de titular %q", titularID)
	}
	return out, nil
}
