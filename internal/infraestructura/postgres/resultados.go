package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.RepositorioResultados = (*Store)(nil)

// GuardarResultado persiste una corrida completa en una transaccion.
// bruto se reconstruye para el CHECK deducciones_cuadran.
//
// NoDistribuido es columna de resultados_proceso. PartesNoDistribuidas y
// PorGrupo son listas (ParteNoDistribuida, LineaGrupo), asi que viven en
// resultados_parte_no_distribuida y resultados_grupo (migracion 00018).
// El indice del slice se guarda como orden: ResultadoPorProceso lo relee
// en ese orden, no en el alfabetico del grupo (ADR 0005).
func (s *Store) GuardarResultado(ctx context.Context, procesoID string, r reparto.Resultado) error {
	bruto := r.Neto.Add(r.Admin).Add(r.Social).Add(r.Reserva)
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO resultados_proceso
			   (proceso_id, bruto, admin, social, reserva, neto, retenido, residuo,
			    no_distribuido, valor_punto, snapshot_id, reglamento)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			procesoID, bruto, r.Admin, r.Social, r.Reserva, r.Neto, r.Retenido, r.Residuo,
			r.NoDistribuido, r.ValorPunto, r.SnapshotID, r.Reglamento,
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

		for i, p := range r.PartesNoDistribuidas {
			// NULLIF: el dominio usa "" cuando el tramo no trae grupo u obra
			// (peso_cero, grupo_sin_obras). La columna es NULL, no '', para
			// que la FK a obras y el CHECK del grupo sigan aplicando.
			if _, err := tx.Exec(ctx,
				`INSERT INTO resultados_parte_no_distribuida
				   (proceso_id, orden, motivo, grupo, obra_id, importe)
				 VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)`,
				procesoID, i, string(p.Motivo), string(p.Grupo), p.ObraID, p.Importe,
			); err != nil {
				if esClaveForanea(err) {
					return fmt.Errorf("guardar parte no distribuida %d en %q: %w", i, procesoID, aplicacion.ErrNoEncontrado)
				}
				return traducirError(err, "guardar parte no distribuida %d en %q", i, procesoID)
			}
		}

		for i, g := range r.PorGrupo {
			if _, err := tx.Exec(ctx,
				`INSERT INTO resultados_grupo
				   (proceso_id, orden, grupo, bolsa, total_puntos, valor_punto, residuo)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				procesoID, i, string(g.Grupo), g.Bolsa, g.TotalPuntos, g.ValorPunto, g.Residuo,
			); err != nil {
				if esClaveForanea(err) {
					return fmt.Errorf("guardar linea de grupo %q en %q: %w", g.Grupo, procesoID, aplicacion.ErrNoEncontrado)
				}
				return traducirError(err, "guardar linea de grupo %q en %q", g.Grupo, procesoID)
			}
		}
		return nil
	})
}

// ResultadoPorProceso relee una corrida, ordenada de forma estable (ADR 0005).
func (s *Store) ResultadoPorProceso(ctx context.Context, procesoID string) (reparto.Resultado, error) {
	var r reparto.Resultado
	ejecutor := s.ejecutorDe(ctx)
	err := ejecutor.QueryRow(ctx,
		`SELECT admin, social, reserva, neto, retenido, residuo, no_distribuido,
		        valor_punto, snapshot_id, reglamento
		   FROM resultados_proceso WHERE proceso_id = $1`,
		procesoID,
	).Scan(&r.Admin, &r.Social, &r.Reserva, &r.Neto, &r.Retenido, &r.Residuo, &r.NoDistribuido,
		&r.ValorPunto, &r.SnapshotID, &r.Reglamento)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_proceso de %q", procesoID)
	}

	obraFilas, err := ejecutor.Query(ctx,
		`SELECT obra_id, puntos, importe, retenida, motivo FROM resultados_obra
		  WHERE proceso_id = $1 ORDER BY obra_id`, procesoID)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_obra de %q", procesoID)
	}
	defer obraFilas.Close()
	for obraFilas.Next() {
		var o reparto.LineaObra
		if err := obraFilas.Scan(&o.ObraID, &o.Puntos, &o.Importe, &o.Retenida, &o.Motivo); err != nil {
			return reparto.Resultado{}, traducirError(err, "escanear resultados_obra de %q", procesoID)
		}
		r.Obras = append(r.Obras, o)
	}
	if err := obraFilas.Err(); err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_obra de %q", procesoID)
	}

	titularFilas, err := ejecutor.Query(ctx,
		`SELECT obra_id, titular_id, ipi, porcentaje, importe FROM resultados_titular
		  WHERE proceso_id = $1 ORDER BY obra_id, titular_id`, procesoID)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_titular de %q", procesoID)
	}
	defer titularFilas.Close()
	for titularFilas.Next() {
		var t reparto.LineaTitular
		if err := titularFilas.Scan(&t.ObraID, &t.TitularID, &t.IPI, &t.Porcentaje, &t.Importe); err != nil {
			return reparto.Resultado{}, traducirError(err, "escanear resultados_titular de %q", procesoID)
		}
		r.Titulares = append(r.Titulares, t)
	}
	if err := titularFilas.Err(); err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_titular de %q", procesoID)
	}

	// COALESCE: grupo y obra_id son NULL cuando el tramo no los trae, y el
	// dominio los representa con "". Mismo patron que actor_id en asientos.
	parteFilas, err := ejecutor.Query(ctx,
		`SELECT motivo, COALESCE(grupo, ''), COALESCE(obra_id, ''), importe
		   FROM resultados_parte_no_distribuida
		  WHERE proceso_id = $1 ORDER BY orden`, procesoID)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer partes no distribuidas de %q", procesoID)
	}
	defer parteFilas.Close()
	for parteFilas.Next() {
		var (
			p      reparto.ParteNoDistribuida
			motivo string
			grupo  string
		)
		if err := parteFilas.Scan(&motivo, &grupo, &p.ObraID, &p.Importe); err != nil {
			return reparto.Resultado{}, traducirError(err, "escanear parte no distribuida de %q", procesoID)
		}
		p.Motivo = reparto.MotivoNoDistribuido(motivo)
		p.Grupo = reparto.GrupoCanal(grupo)
		r.PartesNoDistribuidas = append(r.PartesNoDistribuidas, p)
	}
	if err := parteFilas.Err(); err != nil {
		return reparto.Resultado{}, traducirError(err, "leer partes no distribuidas de %q", procesoID)
	}

	grupoFilas, err := ejecutor.Query(ctx,
		`SELECT grupo, bolsa, total_puntos, valor_punto, residuo
		   FROM resultados_grupo
		  WHERE proceso_id = $1 ORDER BY orden`, procesoID)
	if err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_grupo de %q", procesoID)
	}
	defer grupoFilas.Close()
	for grupoFilas.Next() {
		var (
			g     reparto.LineaGrupo
			grupo string
		)
		if err := grupoFilas.Scan(&grupo, &g.Bolsa, &g.TotalPuntos, &g.ValorPunto, &g.Residuo); err != nil {
			return reparto.Resultado{}, traducirError(err, "escanear resultados_grupo de %q", procesoID)
		}
		g.Grupo = reparto.GrupoCanal(grupo)
		r.PorGrupo = append(r.PorGrupo, g)
	}
	if err := grupoFilas.Err(); err != nil {
		return reparto.Resultado{}, traducirError(err, "leer resultados_grupo de %q", procesoID)
	}

	return r, nil
}
