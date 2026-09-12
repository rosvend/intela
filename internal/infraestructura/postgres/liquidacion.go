package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

var (
	_ aplicacion.RepositorioLiquidacion = (*Store)(nil)
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
		LEFT JOIN declaraciones d
		  ON d.obra_id = rt.obra_id AND d.titular_id = rt.titular_id
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
	x.Bruto, x.Deducciones = proporcional(x.Neto, bolsaBruto, admin, social, reserva, bolsaNeto)

	if err := s.origenDeObra(ctx, &x, obraID); err != nil {
		return aplicacion.Explicacion{}, err
	}
	return x, nil
}

// origenDeObra rellena reporte, escalon y puntaje. Sin uso la cifra sigue
// existiendo: el origen queda vacio, no se convierte un 200 en 404.
func (s *Store) origenDeObra(ctx context.Context, x *aplicacion.Explicacion, obraID string) error {
	var puntaje decimal.Decimal
	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.fuente, r.sha256, u.escalon, u.puntaje
		FROM usos u
		JOIN reportes r ON r.id = u.reporte_id
		WHERE u.obra_id = $1 AND r.periodo = $2 AND NOT u.oni
		ORDER BY u.puntaje DESC, r.id
		LIMIT 1`,
		obraID, x.Corrida.Periodo,
	).Scan(&x.Reporte.ID, &x.Reporte.Fuente, &x.Reporte.SHA256, &x.Obra.Escalon, &puntaje)
	if err == nil {
		x.Obra.Puntaje = puntaje
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return traducirError(err, "uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
}

// proporcional reparte bruto y deducciones de la bolsa en la misma
// proporcion que el neto de la linea. La linea ya es neta (cierra contra
// resultados_proceso.neto); el bruto del titular no se persiste y solo
// existe para la explicacion (OE-6).
func proporcional(netoLinea, bruto, admin, social, reserva, netoBolsa decimal.Decimal) (decimal.Decimal, []aplicacion.Deduccion) {
	if netoBolsa.IsZero() {
		return decimal.Zero, []aplicacion.Deduccion{}
	}
	factor := netoLinea.Div(netoBolsa)
	brutoTitular := bruto.Mul(factor).Round(2)
	cien := decimal.NewFromInt(100)
	pct := func(parte decimal.Decimal) decimal.Decimal {
		if bruto.IsZero() {
			return decimal.Zero
		}
		return parte.Div(bruto).Mul(cien).Round(2)
	}
	return brutoTitular, []aplicacion.Deduccion{
		{Concepto: "gastos administrativos", Porcentaje: pct(admin), Monto: admin.Mul(factor).Round(2)},
		{Concepto: "bienestar social", Porcentaje: pct(social), Monto: social.Mul(factor).Round(2)},
		{Concepto: "reserva", Porcentaje: pct(reserva), Monto: reserva.Mul(factor).Round(2)},
	}
}
