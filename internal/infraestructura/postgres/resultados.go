package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioResultados = (*Store)(nil)

// GuardarResultado persiste una corrida completa -- resultados_proceso,
// resultados_obra y resultados_titular -- en una sola transaccion: un
// resultado a medias es una cifra que alguien puede leer y pagar.
//
// bruto no es un campo de [reparto.Resultado] (el motor no lo necesita: solo
// deducciones.go lo consume, y ya viene de Bolsa.Bruto). Se reconstruye para
// satisfacer el CHECK deducciones_cuadran de la migracion 00001, que es la
// misma identidad que aplicarDeducciones ya garantiza en el dominio.
//
// # Lo que este round-trip NO reproduce
//
// resultados_proceso (migracion 00001) no tiene columnas para
// NoDistribuido, PartesNoDistribuidas ni PorGrupo: ese hueco de esquema es
// anterior a esta issue (#33/#34) y esta fuera de su alcance. #121 solo
// necesita Titulares y Reserva para el replay proporcional (RD 14.4,
// RD 10.1); ver [reparto.DistribuirSobreProporciones]. Persistir esos tres campos
// es trabajo de quien construya RepositorioProcesos completo.
func (s *Store) GuardarResultado(ctx context.Context, procesoID string, r reparto.Resultado) error {
	bruto := r.Neto.Add(r.Admin).Add(r.Social).Add(r.Reserva)
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO resultados_proceso
			   (proceso_id, bruto, admin, social, reserva, neto, retenido, residuo, valor_punto, snapshot_id, reglamento)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			procesoID, bruto, r.Admin, r.Social, r.Reserva, r.Neto, r.Retenido, r.Residuo, r.ValorPunto,
			r.SnapshotID, r.Reglamento,
		); err != nil {
			if esClaveForanea(err) {
				return fmt.Errorf("guardar resultados_proceso de %q: %w", procesoID, aplicacion.ErrNoEncontrado)
			}
			return traducirError(err, "guardar resultados_proceso de %q", procesoID)
		}

		for _, o := range r.Obras {
			if _, err := tx.Exec(ctx,
				`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida, motivo)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				procesoID, o.ObraID, o.Puntos, o.Importe, o.Retenida, o.Motivo,
			); err != nil {
				if esClaveForanea(err) {
					return fmt.Errorf("guardar linea de obra %q en %q: %w", o.ObraID, procesoID, aplicacion.ErrNoEncontrado)
				}
				return traducirError(err, "guardar linea de obra %q en %q", o.ObraID, procesoID)
			}
		}

		for _, t := range r.Titulares {
			if _, err := tx.Exec(ctx,
				`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				procesoID, t.ObraID, t.TitularID, t.IPI, t.Porcentaje, t.Importe,
			); err != nil {
				if esClaveForanea(err) {
					return fmt.Errorf("guardar linea de titular %q en %q: %w", t.TitularID, procesoID, aplicacion.ErrNoEncontrado)
				}
				return traducirError(err, "guardar linea de titular %q en %q", t.TitularID, procesoID)
			}
		}
		return nil
	})
}

// ResultadoPorProceso relee una corrida. El orden (obra_id, y obra_id +
// titular_id) es estable: el replay proporcional (RD 14.4, RD 10.1) itera
// sobre esta lista, y ADR 0005 exige orden reproducible.
func (s *Store) ResultadoPorProceso(ctx context.Context, procesoID string) (reparto.Resultado, error) {
	var r reparto.Resultado
	err := s.pool.QueryRow(ctx,
		`SELECT admin, social, reserva, neto, retenido, residuo, valor_punto, snapshot_id, reglamento
		   FROM resultados_proceso WHERE proceso_id = $1`,
		procesoID,
	).Scan(&r.Admin, &r.Social, &r.Reserva, &r.Neto, &r.Retenido, &r.Residuo, &r.ValorPunto, &r.SnapshotID, &r.Reglamento)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_proceso de %q", procesoID)
	}

	obraFilas, err := s.pool.Query(ctx,
		`SELECT obra_id, puntos, importe, retenida, motivo FROM resultados_obra
		  WHERE proceso_id = $1 ORDER BY obra_id`, procesoID)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_obra de %q", procesoID)
	}
	for obraFilas.Next() {
		var o reparto.LineaObra
		if err := obraFilas.Scan(&o.ObraID, &o.Puntos, &o.Importe, &o.Retenida, &o.Motivo); err != nil {
			obraFilas.Close()
			return reparto.Resultado{}, traducirError(err, "escanear resultados_obra de %q", procesoID)
		}
		r.Obras = append(r.Obras, o)
	}
	if err := obraFilas.Err(); err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_obra de %q", procesoID)
	}

	titularFilas, err := s.pool.Query(ctx,
		`SELECT obra_id, titular_id, ipi, porcentaje, importe FROM resultados_titular
		  WHERE proceso_id = $1 ORDER BY obra_id, titular_id`, procesoID)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_titular de %q", procesoID)
	}
	for titularFilas.Next() {
		var t reparto.LineaTitular
		if err := titularFilas.Scan(&t.ObraID, &t.TitularID, &t.IPI, &t.Porcentaje, &t.Importe); err != nil {
			titularFilas.Close()
			return reparto.Resultado{}, traducirError(err, "escanear resultados_titular de %q", procesoID)
		}
		r.Titulares = append(r.Titulares, t)
	}
	if err := titularFilas.Err(); err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_titular de %q", procesoID)
	}

	return r, nil
}
