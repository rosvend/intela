package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
)

var _ aplicacion.CorreccionDeDatos = (*Store)(nil)

// ExcluirUsoDuplicado pasa la fila a escalon 'duplicado' (#164, migracion 00025).
//
// Sin obra y con oni = FALSE: es lo que el CHECK `uso_resuelto_tiene_obra`
// admite para 'duplicado', y lo que deja la fila fuera de UsosDeCanal
// (`obra_id IS NOT NULL`), del listado ONI y de la cola de identificacion sin
// tocar ninguna de esas consultas. La obra que tenia queda en `evidencia`.
//
// La escritura es CONDICIONAL al escalon y la obra que el caso de uso valido:
// la cascada no toma el cerrojo de periodo, y una fila que cambio entretanto
// ya no es la que se decidio excluir. Cero filas -> ErrAccionNoAplica, y la
// unidad revierte el cierre de la alerta con ella.
func (s *Store) ExcluirUsoDuplicado(ctx context.Context, e aplicacion.ExclusionDeUso) error {
	etiqueta, err := s.ejecutorDe(ctx).Exec(ctx,
		`UPDATE usos
		    SET escalon = 'duplicado', oni = FALSE, obra_id = NULL, puntaje = 0,
		        evidencia = $4, resuelto_por = $5, resuelto_en = $6, nota_resolucion = $7
		  WHERE id = $1 AND escalon = $2 AND COALESCE(obra_id, '') = $3`,
		e.UsoID, e.EscalonPrevio, e.ObraPrevia, e.Evidencia, e.ActorID, e.Cuando, e.Nota)
	if err != nil {
		return traducirError(err, "excluir el uso %q por duplicado", e.UsoID)
	}
	if etiqueta.RowsAffected() == 0 {
		return fmt.Errorf("excluir el uso %q: ya no esta en escalon %q con obra %q; reevalue el periodo: %w",
			e.UsoID, e.EscalonPrevio, e.ObraPrevia, anomalias.ErrAccionNoAplica)
	}
	return nil
}

// ExcluirEntregaDuplicada marca la entrega como excluida y pasa a 'duplicado'
// sus filas que sigan en juego (#164).
//
// La marca en `reportes` va primero y con `excluida_en IS NULL`: dos cierres
// concurrentes sobre la misma entrega -- las alertas de las dos patas del par
// -- no pueden excluirla dos veces, y el segundo recibe ErrAccionNoAplica.
//
// Las filas 'descartado' no se tocan: ya estan fuera por una decision humana
// que dice otra cosa (no es del repertorio). Las 'duplicado' tampoco: ya lo
// son. Cada fila guarda en `evidencia` el escalon y la obra que tenia, en el
// mismo UPDATE que la cambia (el SET lee los valores anteriores de la fila).
//
// Dentro de la unidad de trabajo si la hay (enTransaccionDe): la marca y las
// filas son un solo hecho.
func (s *Store) ExcluirEntregaDuplicada(
	ctx context.Context, e aplicacion.ExclusionDeEntrega,
) (aplicacion.EntregaExcluida, error) {
	out := aplicacion.EntregaExcluida{PorEscalon: map[string]int{}}
	err := s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		etiqueta, err := tx.Exec(ctx,
			`UPDATE reportes SET excluida_por = $2, excluida_en = $3
			  WHERE id = $1 AND excluida_en IS NULL`,
			e.ReporteID, e.ActorID, e.Cuando)
		if err != nil {
			return traducirError(err, "excluir la entrega %q", e.ReporteID)
		}
		if etiqueta.RowsAffected() == 0 {
			return fmt.Errorf("excluir la entrega %q: no existe o ya estaba excluida; reevalue el periodo: %w",
				e.ReporteID, anomalias.ErrAccionNoAplica)
		}

		filas, err := tx.Query(ctx,
			`WITH previas AS (
			   SELECT id, escalon, obra_id FROM usos
			    WHERE reporte_id = $1 AND escalon NOT IN ('descartado', 'duplicado')
			      FOR UPDATE
			 ), cambiadas AS (
			   UPDATE usos u
			      SET escalon = 'duplicado', oni = FALSE, obra_id = NULL, puntaje = 0,
			          evidencia = $2 || '; antes escalon ' || p.escalon
			                         || COALESCE(' con obra ' || p.obra_id, ' sin obra'),
			          resuelto_por = $3, resuelto_en = $4, nota_resolucion = $5
			     FROM previas p
			    WHERE u.id = p.id
			   RETURNING p.escalon, p.obra_id
			 )
			 SELECT escalon, COALESCE(obra_id, '') FROM cambiadas`,
			e.ReporteID, e.Evidencia, e.ActorID, e.Cuando, e.Nota)
		if err != nil {
			return traducirError(err, "excluir las filas de la entrega %q", e.ReporteID)
		}
		defer filas.Close()

		obras := map[string]bool{}
		for filas.Next() {
			var escalon, obra string
			if err := filas.Scan(&escalon, &obra); err != nil {
				return traducirError(err, "escanear fila excluida de la entrega %q", e.ReporteID)
			}
			out.Usos++
			out.PorEscalon[escalon]++
			if obra != "" && !obras[obra] {
				obras[obra] = true
				out.Obras = append(out.Obras, obra)
			}
		}
		if err := filas.Err(); err != nil {
			return traducirError(err, "excluir las filas de la entrega %q", e.ReporteID)
		}
		return nil
	})
	if err != nil {
		return aplicacion.EntregaExcluida{}, err
	}
	return out, nil
}

// AsignarTipoObraAUso pone el tipo de obra a una fila identificada que no lo
// trae (#164) y devuelve su obra.
//
// Condicional a un `tipo_obra` vacio y a `obra_id IS NOT NULL`: es exactamente la
// fila que levanta `tipo_obra_sin_mapear`. Si ya tiene tipo, o ya no tiene
// obra, la alerta describe un dato que ya no esta asi: ErrAccionNoAplica.
func (s *Store) AsignarTipoObraAUso(ctx context.Context, usoID, tipoObra string) (string, error) {
	var obraID string
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`UPDATE usos SET tipo_obra = $2
		  WHERE id = $1 AND tipo_obra = '' AND obra_id IS NOT NULL
		  RETURNING obra_id`,
		usoID, tipoObra).Scan(&obraID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("asignar tipo de obra al uso %q: ya tiene tipo o ya no tiene obra; reevalue el periodo: %w",
			usoID, anomalias.ErrAccionNoAplica)
	}
	if err != nil {
		return "", traducirError(err, "asignar tipo de obra al uso %q", usoID)
	}
	return obraID, nil
}
