package postgres

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.RepositorioReporteLiquidacion = (*Store)(nil)

// FilasDeTitular lee las lineas de corrida del titular, con los totales del
// proceso y TODOS los netos del proceso (ordenados) para prorratear
// deducciones aguas arriba sin perder centavos entre titulares (RD 16).
//
// periodo vacio es todos. Cero filas no es ErrNoEncontrado.
func (s *Store) FilasDeTitular(ctx context.Context, titularID, periodo string) ([]aplicacion.FilaLiquidacion, error) {
	const q = `
		SELECT p.id, p.periodo, rt.obra_id, rt.titular_id, o.titulo, rt.importe,
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

	type filaCruda struct {
		aplicacion.FilaLiquidacion
		titularID string
	}
	var crudas []filaCruda
	procesos := map[string]struct{}{}
	for filas.Next() {
		var f filaCruda
		if err := filas.Scan(
			&f.ProcesoID, &f.Periodo, &f.ObraID, &f.titularID, &f.Titulo, &f.Neto,
			&f.ProcesoBruto, &f.ProcesoAdmin, &f.ProcesoSocial, &f.ProcesoReserva, &f.ProcesoNeto,
		); err != nil {
			return nil, traducirError(err, "escanear liquidacion de titular %q", titularID)
		}
		crudas = append(crudas, f)
		procesos[f.ProcesoID] = struct{}{}
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "liquidacion de titular %q", titularID)
	}
	if len(crudas) == 0 {
		return []aplicacion.FilaLiquidacion{}, nil
	}

	netosPorProceso, err := s.netosDeProcesos(ctx, keys(procesos))
	if err != nil {
		return nil, err
	}

	out := make([]aplicacion.FilaLiquidacion, 0, len(crudas))
	for _, c := range crudas {
		netos := netosPorProceso[c.ProcesoID]
		indice := indiceDeLinea(netos, c.ObraID, c.titularID)
		f := c.FilaLiquidacion
		f.NetosProceso = montos(netos)
		f.Indice = indice
		out = append(out, f)
	}
	return out, nil
}

type lineaProceso struct {
	obraID    string
	titularID string
	neto      decimal.Decimal
}

// netosDeProcesos carga todas las lineas de titular de cada proceso,
// ordenadas por (obra_id, titular_id) — el orden estable del mayor-resto.
func (s *Store) netosDeProcesos(ctx context.Context, procesoIDs []string) (map[string][]lineaProceso, error) {
	out := make(map[string][]lineaProceso, len(procesoIDs))
	if len(procesoIDs) == 0 {
		return out, nil
	}
	filas, err := s.pool.Query(ctx, `
		SELECT proceso_id, obra_id, titular_id, importe
		FROM resultados_titular
		WHERE proceso_id = ANY($1)
		ORDER BY proceso_id, obra_id, titular_id`,
		procesoIDs,
	)
	if err != nil {
		return nil, traducirError(err, "netos de procesos")
	}
	defer filas.Close()

	for filas.Next() {
		var (
			procesoID, obraID, titularID string
			neto                         decimal.Decimal
		)
		if err := filas.Scan(&procesoID, &obraID, &titularID, &neto); err != nil {
			return nil, traducirError(err, "escanear netos de proceso")
		}
		out[procesoID] = append(out[procesoID], lineaProceso{
			obraID: obraID, titularID: titularID, neto: neto,
		})
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "netos de procesos")
	}
	return out, nil
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func montos(lineas []lineaProceso) []decimal.Decimal {
	out := make([]decimal.Decimal, len(lineas))
	for i, l := range lineas {
		out[i] = l.neto
	}
	return out
}

func indiceDeLinea(lineas []lineaProceso, obraID, titularID string) int {
	for i, l := range lineas {
		if l.obraID == obraID && l.titularID == titularID {
			return i
		}
	}
	return -1
}
