package postgres

import (
	"context"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var _ aplicacion.RepositorioIdentificacion = (*Store)(nil)

// Alias busca el par (fuente, tipo, valor) en alias_obra. La PK de la tabla
// garantiza como mucho una fila.
func (s *Store) Alias(ctx context.Context, fuente, tipo, valor string) (string, error) {
	var obraID string
	err := s.pool.QueryRow(ctx,
		`SELECT obra_id FROM alias_obra WHERE fuente = $1 AND tipo_id = $2 AND valor = $3`,
		fuente, tipo, valor).Scan(&obraID)
	if err != nil {
		return "", traducirError(err, "alias de %q (%s=%s)", fuente, tipo, valor)
	}
	return obraID, nil
}

// GuardarAlias aprende un alias. Es idempotente por diseno (D5 del diseno de
// #28): ON CONFLICT DO NOTHING, para que una re-corrida o una carrera no
// dupliquen ni pisen un alias que ya apunta a otra obra -un match automatico
// no puede deshacer en silencio una decision anterior.
//
// quien vacio entra como NULL: la columna es nullable y "" no es un actor.
func (s *Store) GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO alias_obra (fuente, tipo_id, valor, obra_id, quien)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		 ON CONFLICT (fuente, tipo_id, valor) DO NOTHING`,
		fuente, tipo, valor, obraID, quien)
	if err != nil {
		return traducirError(err, "guardar alias %s %s=%s -> %q", fuente, tipo, valor, obraID)
	}
	return nil
}

// ObraPorIDGlobal busca una obra por IDA, EIDR o IMDB. El contrato del puerto
// (puertos.go) es que los tres vacios devuelven ErrNoEncontrado sin tocar la
// base: llamarla sin datos no puede inventar un match, y la cascada nunca la
// llama asi (siempre con exactamente uno poblado).
//
// ORDER BY id LIMIT 1: determinismo (D9) si el catalogo tiene mas de una obra
// con el mismo identificador global -no deberia, pero no hay UNIQUE en obras.
func (s *Store) ObraPorIDGlobal(ctx context.Context, ida, eidr, imdb string) (string, error) {
	if ida == "" && eidr == "" && imdb == "" {
		return "", fmt.Errorf("obra por id global sin ningun identificador poblado: %w", aplicacion.ErrNoEncontrado)
	}
	var obraID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM obras
		  WHERE ($1 <> '' AND ida = $1) OR ($2 <> '' AND eidr = $2) OR ($3 <> '' AND imdb = $3)
		  ORDER BY id LIMIT 1`,
		ida, eidr, imdb).Scan(&obraID)
	if err != nil {
		return "", traducirError(err, "obra por id global (ida=%q eidr=%q imdb=%q)", ida, eidr, imdb)
	}
	return obraID, nil
}

// GuardarMatch traduce un Resultado de la cascada al UPDATE de la fila, sin
// reinterpretar nada (D6 del diseno): oni es la negacion de si hay obra, y eso
// es consistente con el CHECK uso_resuelto_tiene_obra por construccion.
//
// oni = ($2 = vacio), no ($2 <> vacio): el CHECK uso_resuelto_tiene_obra
// exige (oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL) -oni
// significa "sin identificar", no "identificada"-. El SQL de 01-design.md
// §5.5 trae la comparacion invertida, que viola ese mismo CHECK en cuanto se
// guarda un match con obra; se corrige aqui y se anota en la PR.
//
// resuelto_por y resuelto_en no se tocan: el CHECK manual_tiene_autor los
// reserva a escalon='manual', que este puerto no escribe en este issue.
func (s *Store) GuardarMatch(ctx context.Context, usoID string, r identificacion.Resultado) error {
	etiqueta, err := s.pool.Exec(ctx,
		`UPDATE usos
		    SET obra_id = NULLIF($2, ''), escalon = $3, evidencia = $4, puntaje = $5,
		        oni = ($2 = '')
		  WHERE id = $1`,
		usoID, r.ObraID, r.Escalon, r.Evidencia, r.Puntaje)
	if err != nil {
		return traducirError(err, "guardar match del uso %q", usoID)
	}
	if etiqueta.RowsAffected() == 0 {
		return fmt.Errorf("guardar match del uso %q: %w", usoID, aplicacion.ErrNoEncontrado)
	}
	return nil
}
