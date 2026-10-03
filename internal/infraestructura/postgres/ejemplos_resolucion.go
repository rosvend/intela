package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var _ aplicacion.RepositorioEjemplosResolucion = (*Store)(nil)

func (s *Store) HistoriaPorClaves(ctx context.Context, claves []string) ([]identificacion.EjemploEtiquetado, error) {
	if len(claves) == 0 {
		return nil, nil
	}
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT clave, decision, COALESCE(obra_elegida, '')
		   FROM ejemplos_resolucion
		  WHERE clave = ANY($1::text[])`, claves)
	if err != nil {
		return nil, traducirError(err, "leer el historial de resoluciones")
	}
	defer filas.Close()

	var out []identificacion.EjemploEtiquetado
	for filas.Next() {
		var e identificacion.EjemploEtiquetado
		var decision string
		if err := filas.Scan(&e.Clave, &decision, &e.ObraID); err != nil {
			return nil, traducirError(err, "leer el historial de resoluciones")
		}
		e.Decision = identificacion.Decision(decision)
		out = append(out, e)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer el historial de resoluciones")
	}
	return out, nil
}

func (s *Store) GuardarEjemplo(ctx context.Context, e aplicacion.EjemploResolucion) error {
	candidatos, err := json.Marshal(rasgosDe(e.Candidatos))
	if err != nil {
		return fmt.Errorf("serializar las candidatas del ejemplo %q: %w", e.UsoID, err)
	}
	orden, err := json.Marshal(ordenDe(e.Orden))
	if err != nil {
		return fmt.Errorf("serializar el orden del ejemplo %q: %w", e.UsoID, err)
	}
	_, err = s.ejecutorDe(ctx).Exec(ctx,
		`INSERT INTO ejemplos_resolucion (
		    uso_id, clave, fuente, candidatos, decision, obra_elegida,
		    sugerencia_decision, sugerencia_obra_id, confianza, motivo, orden,
		    aceptada, actor_id
		  ) VALUES (
		    $1, $2, $3, $4, $5, NULLIF($6, ''),
		    $7, NULLIF($8, ''), $9, $10, $11,
		    $12, $13
		  )`,
		e.UsoID, e.Clave, e.Fuente, candidatos, string(e.Decision), e.ObraElegida,
		e.SugerenciaDecision, e.SugerenciaObraID, e.Confianza, e.Motivo, orden,
		e.Aceptada, e.ActorID)
	if err != nil {
		return traducirError(err, "guardar el ejemplo de la resolucion %q", e.UsoID)
	}
	return nil
}

func (s *Store) EjemplosDe(ctx context.Context, usoIDs []string) (map[string]aplicacion.EjemploGuardado, error) {
	out := map[string]aplicacion.EjemploGuardado{}
	if len(usoIDs) == 0 {
		return out, nil
	}
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT e.uso_id, e.sugerencia_decision, COALESCE(e.sugerencia_obra_id, ''),
		        COALESCE(o.titulo, ''), e.confianza, e.motivo, e.orden, e.aceptada
		   FROM ejemplos_resolucion e
		   LEFT JOIN obras o ON o.id = e.sugerencia_obra_id
		  WHERE e.uso_id = ANY($1::text[])`, usoIDs)
	if err != nil {
		return nil, traducirError(err, "leer ejemplos de resolucion")
	}
	defer filas.Close()

	for filas.Next() {
		var (
			id       string
			g        aplicacion.EjemploGuardado
			ordenRaw []byte
		)
		if err := filas.Scan(&id, &g.SugerenciaDecision, &g.SugerenciaObraID,
			&g.Titulo, &g.Confianza, &g.Motivo, &ordenRaw, &g.Aceptada); err != nil {
			return nil, traducirError(err, "leer ejemplos de resolucion")
		}
		if err := json.Unmarshal(ordenRaw, &g.Orden); err != nil {
			return nil, fmt.Errorf("orden del ejemplo %q: %w", id, err)
		}
		if g.Orden == nil {
			g.Orden = []string{}
		}
		out[id] = g
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "leer ejemplos de resolucion")
	}
	return out, nil
}

type rasgoJSON struct {
	ObraID  string `json:"obra_id"`
	Puntaje string `json:"puntaje"`
}

func rasgosDe(cs []identificacion.Candidato) []rasgoJSON {
	out := make([]rasgoJSON, 0, len(cs))
	for _, c := range cs {
		out = append(out, rasgoJSON{ObraID: c.ObraID, Puntaje: c.Puntaje.String()})
	}
	return out
}

func ordenDe(orden []string) []string {
	if orden == nil {
		return []string{}
	}
	return orden
}
