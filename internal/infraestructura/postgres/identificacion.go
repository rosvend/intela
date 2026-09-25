package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var _ aplicacion.RepositorioIdentificacion = (*Store)(nil)

// Alias busca el par (fuente, tipo, valor) en alias_obra. La PK de la tabla
// garantiza como mucho una fila.
func (s *Store) Alias(ctx context.Context, fuente, tipo, valor string) (string, error) {
	var obraID string
	err := s.ejecutorDe(ctx).QueryRow(ctx,
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
	_, err := s.ejecutorDe(ctx).Exec(ctx,
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
	err := s.ejecutorDe(ctx).QueryRow(ctx,
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
// reinterpretar nada (D6 del diseno): oni es la negacion de si hay obra, salvo
// para escalon='excluido', que no tiene obra y tampoco es ONI (criterio 4 de
// #28: una fila fuera de repertorio no puede aparecer en oni_publico). El CHECK
// uso_resuelto_tiene_obra, desde 00007, admite ese caso y solo ese.
//
// oni = ($2 = vacio), no ($2 <> vacio): el CHECK uso_resuelto_tiene_obra
// exige (oni AND obra_id IS NULL) OR (NOT oni AND obra_id IS NOT NULL) -oni
// significa "sin identificar", no "identificada"-. El SQL de 01-design.md
// §5.5 trae la comparacion invertida, que viola ese mismo CHECK en cuanto se
// guarda un match con obra; se corrige aqui y se anota en la PR.
//
// resuelto_por y resuelto_en no se tocan: el CHECK manual_tiene_autor los
// reserva a escalon='manual', que este puerto no escribe en este issue.
//
// AND escalon = escalonPrevio: la escritura es condicional al estado que el
// caso de uso leyo. Una fila que otro proceso cambio entre la lectura y este
// UPDATE -una resolucion manual, otra corrida- no se pisa; el caso de uso ve
// ErrNoEncontrado y la salta.
//
// # tipo_obra NO se rellena aqui, y es una decision medida
//
// `obras.tipo` tiene el dato -TEXT NOT NULL con CHECK sobre las cinco
// categorias de `RD 9.1.1` (00001)- y `usos.tipo_obra` se queda vacio para toda
// fuente cuyo mapa no traiga la columna, que hoy es la parrilla entera de
// Caracol. Copiarlo aqui parece el arreglo obvio y NO lo es: medido contra
// Postgres real, con el backfill puesto la corrida de TV de Caracol deja de
// abortar y pasa a devolver `noDistribuido` = la bolsa ENTERA, con error nil.
// `MapaCaracol` tampoco mapea `rating`, que queda en 0, y `puntosTV` multiplica
// por el.
//
// O sea que el backfill cambia un fallo RUIDOSO -ErrRepartoInvalido, que para
// la corrida y se ve- por uno SILENCIOSO -cero puntos, cero pagos, sin una sola
// senal-. Son tres huecos independientes (`tipo_obra`, `canal_id` y `rating`) y
// cerrar uno solo empeora el conjunto; van juntos, en su propia issue, con el
// mapa de ingesta y la pregunta P-05 delante. Ver el cuerpo de la PR de #37.
//
// Hoy la cadena ni siquiera llega al aborto: `MapaCaracol` no mapea `canal_id`
// -`CampoCanalID` existe en mapa.go y no lo usa ningun Mapa-, asi que
// `UsosDeCanal` devuelve cero filas y el motor no corre. El hueco es real y
// esta LATENTE.
func (s *Store) GuardarMatch(ctx context.Context, usoID, escalonPrevio string, r identificacion.Resultado) error {
	etiqueta, err := s.ejecutorDe(ctx).Exec(ctx,
		`UPDATE usos
		    SET obra_id = NULLIF($2, ''), escalon = $3, evidencia = $4, puntaje = $5,
		        oni = ($2 = '' AND $3 <> 'excluido')
		  WHERE id = $1 AND escalon = $6`,
		usoID, r.ObraID, r.Escalon, r.Evidencia, r.Puntaje, escalonPrevio)
	if err != nil {
		return traducirError(err, "guardar match del uso %q", usoID)
	}
	if etiqueta.RowsAffected() == 0 {
		return fmt.Errorf("guardar match del uso %q: no existe o ya no esta en escalon %q: %w",
			usoID, escalonPrevio, aplicacion.ErrNoEncontrado)
	}
	return nil
}

// GuardarCandidatos reemplaza la bandeja de revision de un uso (D9).
//
// En transaccion porque borrar y escribir son la misma operacion. Dentro de una
// unidad de trabajo ([Store.EnUnidad]) participa en ella y no confirma por su
// cuenta: asi el caso de uso puede atar la bandeja al match de la misma fila.
// `orden` va explicito y no se deduce del puntaje: el desempate es regla del
// dominio.
func (s *Store) GuardarCandidatos(ctx context.Context, usoID string, cs []identificacion.Candidato) error {
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM candidatos_match WHERE uso_id = $1`, usoID); err != nil {
			return traducirError(err, "limpiar candidatos del uso %q", usoID)
		}
		for i, c := range cs {
			_, err := tx.Exec(ctx,
				`INSERT INTO candidatos_match (uso_id, obra_id, puntaje, orden, titulo_consultado)
				 VALUES ($1, $2, $3, $4, $5)`,
				usoID, c.ObraID, c.Puntaje, i, c.TituloConsultado)
			if err != nil {
				return traducirError(err, "guardar candidato %q del uso %q", c.ObraID, usoID)
			}
		}
		return nil
	})
}

// CandidatosDeUso lee la bandeja de un uso, en su orden. La consume #39.
func (s *Store) CandidatosDeUso(ctx context.Context, usoID string) ([]identificacion.Candidato, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT obra_id, puntaje, titulo_consultado FROM candidatos_match WHERE uso_id = $1 ORDER BY orden`, usoID)
	if err != nil {
		return nil, traducirError(err, "leer candidatos del uso %q", usoID)
	}
	defer filas.Close()

	var cs []identificacion.Candidato
	for filas.Next() {
		var c identificacion.Candidato
		if err := filas.Scan(&c.ObraID, &c.Puntaje, &c.TituloConsultado); err != nil {
			return nil, traducirError(err, "leer candidatos del uso %q", usoID)
		}
		cs = append(cs, c)
	}
	// No es opcional: sin esto una lista truncada pasa por completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer candidatos del uso %q", usoID)
	}
	return cs, nil
}
