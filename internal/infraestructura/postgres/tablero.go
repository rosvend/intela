package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

var _ aplicacion.RepositorioTablero = (*Store)(nil)

// ObrasDeclaradasDe trae las obras con parte vigente del titular y su declaracion entera; el estado lo decide el dominio.
func (s *Store) ObrasDeclaradasDe(ctx context.Context, titularID string) ([]aplicacion.ObraDeclarada, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT o.id, o.titulo FROM declaraciones d`+clausulaVigente+`
		   JOIN obras o ON o.id = d.obra_id
		  WHERE d.titular_id = $1
		  ORDER BY o.titulo, o.id`, titularID)
	if err != nil {
		return nil, traducirError(err, "obras declaradas de %q", titularID)
	}
	defer filas.Close()

	var obras []aplicacion.ObraDeclarada
	var ids []string
	for filas.Next() {
		var o aplicacion.ObraDeclarada
		if err := filas.Scan(&o.ID, &o.Titulo); err != nil {
			return nil, traducirError(err, "escanear obra declarada de %q", titularID)
		}
		obras = append(obras, o)
		ids = append(ids, o.ID)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "obras declaradas de %q", titularID)
	}

	partes, err := s.partesDeObras(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range obras {
		obras[i].Declaracion = repertorio.Declaracion{ObraID: obras[i].ID, Partes: partes[obras[i].ID]}
	}
	return obras, nil
}

// UltimaLiquidacionDe suma las lineas del titular en su ultimo periodo; sin lineas es ErrNoEncontrado.
// rt.importe es el neto, el mismo que sirve /mis-ingresos.
func (s *Store) UltimaLiquidacionDe(ctx context.Context, titularID string) (aplicacion.ResumenLiquidacion, error) {
	var r aplicacion.ResumenLiquidacion
	err := s.ejecutorDe(ctx).QueryRow(ctx, `
		WITH lineas AS (
		  SELECT p.periodo, rt.obra_id, rt.importe
		    FROM resultados_titular rt
		    JOIN procesos p ON p.id = rt.proceso_id
		   WHERE rt.titular_id = $1
		)
		SELECT periodo, SUM(importe), COUNT(DISTINCT obra_id)
		  FROM lineas
		 WHERE periodo = (SELECT MAX(periodo) FROM lineas)
		 GROUP BY periodo`, titularID).Scan(&r.Periodo, &r.Neto, &r.Obras)
	if err != nil {
		return aplicacion.ResumenLiquidacion{}, traducirError(err, "ultima liquidacion de %q", titularID)
	}
	return r, nil
}

// CargasPendientes cuenta los reportes con alguna fila que la cascada aun no proceso.
func (s *Store) CargasPendientes(ctx context.Context) (int, error) {
	var n int
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT COUNT(DISTINCT reporte_id) FROM usos WHERE escalon = $1`,
		identificacion.EscalonPendiente).Scan(&n)
	if err != nil {
		return 0, traducirError(err, "contar cargas pendientes")
	}
	return n, nil
}

// CasosONIPendientes cuenta los usos en escalon oni: el total de /identificacion/casos?estado=pendiente.
func (s *Store) CasosONIPendientes(ctx context.Context) (int, error) {
	var n int
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT COUNT(*) FROM usos WHERE escalon = $1`, identificacion.EscalonONI).Scan(&n)
	if err != nil {
		return 0, traducirError(err, "contar casos ONI pendientes")
	}
	return n, nil
}

// UltimaCorrida lee la corrida del periodo mas reciente; a igual periodo, la ultima abierta.
func (s *Store) UltimaCorrida(ctx context.Context) (aplicacion.ProcesoVista, error) {
	var (
		v               aplicacion.ProcesoVista
		circuito, etapa string
	)
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT id, circuito, etapa, periodo FROM procesos
		  ORDER BY periodo DESC, abierto DESC, id DESC LIMIT 1`).
		Scan(&v.ID, &circuito, &etapa, &v.Periodo)
	if err != nil {
		return aplicacion.ProcesoVista{}, traducirError(err, "ultima corrida")
	}
	v.Circuito = reparto.Circuito(circuito)
	v.Etapa = reparto.Etapa(etapa)
	return v, nil
}
