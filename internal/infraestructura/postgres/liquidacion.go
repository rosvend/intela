package postgres

import (
	"context"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
)

var (
	_ aplicacion.RepositorioIngresos    = (*Store)(nil)
	_ aplicacion.RepositorioExplicacion = (*Store)(nil)
)

// IngresosDe lista las lineas netas de un titular, recortadas por el filtro.
//
// El titularID lo pone el caso de uso desde la sesion. Aqui no hay forma de
// pedir "los de otro": la consulta lleva WHERE titular_id = $1.
func (s *Store) IngresosDe(ctx context.Context, titularID string, f aplicacion.FiltroIngresos) ([]aplicacion.Ingreso, error) {
	filas, err := s.pool.Query(ctx, `
		SELECT
			rt.proceso_id,
			rt.obra_id,
			rt.titular_id,
			o.titulo,
			p.periodo,
			rt.importe,
			COALESCE((
				SELECT string_agg(DISTINCT r.fuente, ', ' ORDER BY r.fuente)
				FROM usos u
				JOIN reportes r ON r.id = u.reporte_id
				WHERE u.obra_id = rt.obra_id
				  AND r.periodo = p.periodo
				  AND NOT u.oni
			), '') AS fuente
		FROM resultados_titular rt
		JOIN procesos p ON p.id = rt.proceso_id
		JOIN obras o ON o.id = rt.obra_id
		WHERE rt.titular_id = $1
		  AND ($2 = '' OR rt.obra_id = $2)
		  AND ($3 = '' OR p.periodo = $3)
		  AND (
		        $4 = '' OR EXISTS (
		            SELECT 1
		            FROM usos u
		            JOIN reportes r ON r.id = u.reporte_id
		            WHERE u.obra_id = rt.obra_id
		              AND r.periodo = p.periodo
		              AND r.fuente = $4
		              AND NOT u.oni
		        )
		      )
		ORDER BY p.periodo, o.titulo, rt.obra_id`,
		titularID, f.ObraID, f.Periodo, f.Fuente,
	)
	if err != nil {
		return nil, traducirError(err, "ingresos de titular %q", titularID)
	}
	defer filas.Close()

	ingresos := []aplicacion.Ingreso{}
	for filas.Next() {
		var (
			procesoID, obraID, tit, titulo, periodo, fuente string
			neto                                            decimal.Decimal
		)
		if err := filas.Scan(&procesoID, &obraID, &tit, &titulo, &periodo, &neto, &fuente); err != nil {
			return nil, traducirError(err, "escanear ingreso de titular %q", titularID)
		}
		ingresos = append(ingresos, aplicacion.Ingreso{
			Ref:     aplicacion.FormarRef(procesoID, obraID, tit),
			ObraID:  obraID,
			Obra:    titulo,
			Fuente:  fuente,
			Periodo: periodo,
			Neto:    neto,
		})
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "ingresos de titular %q", titularID)
	}
	return ingresos, nil
}

// PorLinea reconstruye el linaje de una cifra a partir de lo persistido:
// la linea de titular, la corrida, el reporte que pondero, el match, el
// snapshot y las deducciones. No recalcula el motor (ADR 0005): lee.
//
// El prorrateo bruto/deducciones es trabajo del dominio
// ([liquidacion.Prorratear]): aqui solo se deshace la resta de la bolsa
// sobre la linea del titular para mostrarla.
func (s *Store) PorLinea(ctx context.Context, procesoID, obraID, titularID string) (aplicacion.Explicacion, error) {
	var (
		x          aplicacion.Explicacion
		bolsaBruto decimal.Decimal
		admin      decimal.Decimal
		social     decimal.Decimal
		reserva    decimal.Decimal
		bolsaNeto  decimal.Decimal
	)
	err := s.pool.QueryRow(ctx, `
		SELECT
			rt.titular_id, rt.ipi, rt.porcentaje, rt.importe,
			p.id, p.periodo, p.circuito, COALESCE(p.snapshot_id, ''), p.reglamento,
			o.id, o.titulo,
			rp.bruto, rp.admin, rp.social, rp.reserva, rp.neto,
			COALESCE(d.version, 1)
		FROM resultados_titular rt
		JOIN procesos p ON p.id = rt.proceso_id
		JOIN obras o ON o.id = rt.obra_id
		JOIN resultados_proceso rp ON rp.proceso_id = rt.proceso_id
		-- Desde 00008 hay varias filas por (obra_id, titular_id). Sin filtrar
		-- la version abierta, QueryRow devolveria mas de una y el Scan
		-- abortaria. Mismo criterio que clausulaVigente en repertorio.go.
		LEFT JOIN declaracion_versiones dv
		  ON dv.obra_id = rt.obra_id AND dv.vigente_hasta IS NULL
		LEFT JOIN declaraciones d
		  ON d.obra_id = dv.obra_id AND d.version = dv.version AND d.titular_id = rt.titular_id
		WHERE rt.proceso_id = $1 AND rt.obra_id = $2 AND rt.titular_id = $3`,
		procesoID, obraID, titularID,
	).Scan(
		&x.TitularID, &x.Split.IPI, &x.Split.Porcentaje, &x.Neto,
		&x.Corrida.ProcesoID, &x.Corrida.Periodo, &x.Corrida.Circuito,
		&x.Regla.SnapshotID, &x.Regla.Reglamento,
		&x.Obra.ID, &x.Obra.Titulo,
		&bolsaBruto, &admin, &social, &reserva, &bolsaNeto,
		&x.Split.Version,
	)
	if err != nil {
		return aplicacion.Explicacion{}, traducirError(err,
			"explicar %s", aplicacion.FormarRef(procesoID, obraID, titularID))
	}
	x.Ref = aplicacion.FormarRef(procesoID, obraID, titularID)
	x.Split.TitularID = x.TitularID

	linea := liquidacion.Prorratear(x.Neto, admin, social, reserva, bolsaNeto)
	x.Bruto = linea.Bruto
	if bolsaNeto.IsZero() {
		x.Deducciones = []aplicacion.Deduccion{}
	} else {
		x.Deducciones = deduccionesDe(linea, admin, social, reserva, bolsaBruto)
	}

	if err := s.origenDeObra(ctx, &x, obraID); err != nil {
		return aplicacion.Explicacion{}, err
	}
	return x, nil
}

// deduccionesDe arma el desglose para ExplicarCifra. Los porcentajes son
// los de la bolsa (tasas normativas del proceso); los montos son los ya
// redondeados de [liquidacion.Prorratear], para que Bruto - Σ = Neto.
func deduccionesDe(
	l liquidacion.Linea,
	adminProc, socialProc, reservaProc, bolsaBruto decimal.Decimal,
) []aplicacion.Deduccion {
	cien := decimal.NewFromInt(100)
	pct := func(parte decimal.Decimal) decimal.Decimal {
		if bolsaBruto.IsZero() {
			return decimal.Zero
		}
		return parte.Div(bolsaBruto).Mul(cien).Round(2)
	}
	return []aplicacion.Deduccion{
		{Concepto: "gastos administrativos", Porcentaje: pct(adminProc), Monto: l.Admin},
		{Concepto: "bienestar social", Porcentaje: pct(socialProc), Monto: l.Social},
		{Concepto: "reserva", Porcentaje: pct(reservaProc), Monto: l.Reserva},
	}
}

// origenDeObra rellena reporte, escalon y puntaje. Sin uso la cifra sigue
// existiendo: el origen queda vacio, no se convierte un 200 en 404.
//
// Si la obra pondero por varias fuentes en el periodo, fuente lista todas
// (ordenadas, separadas por coma). id/sha256/escalon/puntaje quedan del
// uso de mayor puntaje — el reporte "principal" — sin ocultar las demas
// fuentes en silencio.
func (s *Store) origenDeObra(ctx context.Context, x *aplicacion.Explicacion, obraID string) error {
	filas, err := s.pool.Query(ctx, `
		SELECT r.id, r.fuente, r.sha256, u.escalon, u.puntaje
		FROM usos u
		JOIN reportes r ON r.id = u.reporte_id
		WHERE u.obra_id = $1 AND r.periodo = $2 AND NOT u.oni
		ORDER BY u.puntaje DESC, r.id`,
		obraID, x.Corrida.Periodo,
	)
	if err != nil {
		return traducirError(err, "uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
	}
	defer filas.Close()

	var (
		fuentes []string
		visto   = map[string]struct{}{}
		primero = true
	)
	for filas.Next() {
		var (
			id, fuente, sha, escalon string
			puntaje                  decimal.Decimal
		)
		if err := filas.Scan(&id, &fuente, &sha, &escalon, &puntaje); err != nil {
			return traducirError(err, "escanear uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
		}
		if primero {
			x.Reporte.ID = id
			x.Reporte.SHA256 = sha
			x.Obra.Escalon = escalon
			x.Obra.Puntaje = puntaje
			primero = false
		}
		if _, ok := visto[fuente]; !ok {
			visto[fuente] = struct{}{}
			fuentes = append(fuentes, fuente)
		}
	}
	if err := filas.Err(); err != nil {
		return traducirError(err, "uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
	}
	if len(fuentes) > 0 {
		// Mismo criterio que IngresosDe (string_agg ORDER BY fuente).
		slices.Sort(fuentes)
		x.Reporte.Fuente = strings.Join(fuentes, ", ")
	}
	return nil
}
