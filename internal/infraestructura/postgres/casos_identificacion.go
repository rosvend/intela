package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var _ aplicacion.RepositorioCasosIdentificacion = (*Store)(nil)

// sqlCasosIdentificacion es una sola sentencia para que pagina y conteo salgan del mismo snapshot.
// El LEFT JOIN desde `uno` garantiza una fila con el conteo aunque la pagina venga vacia.
// No proyecta medidas de uso (rating, taquilla, vistas...): la identificacion no toca dinero (ADR 0007).
//
// $7 filtra por id de uso ($7 = ” no filtra): es lo que permite que la
// respuesta de una resolucion (#175) lea el caso con la MISMA sentencia que la
// lista, en vez de con una hermana que diverge en cuanto alguien anade una
// columna a una sola.
const sqlCasosIdentificacion = `
WITH base AS (
  SELECT u.id, u.titulo, u.titulo_original, u.fuente, COALESCE(u.modalidad, '') AS modalidad,
         u.reporte_id, r.periodo, u.ids_fuente, COALESCE(u.evidencia, '') AS evidencia,
         u.escalon, r.creado, u.resuelto_en, u.resuelto_por, u.obra_id, u.nota_resolucion
    FROM usos u
    JOIN reportes r ON r.id = u.reporte_id
   WHERE (u.escalon = ANY($1::text[]) OR u.escalon = $4)
     AND ($2 = '' OR u.fuente = $2)
     AND ($3 = '' OR r.periodo = $3)
     AND ($7 = '' OR u.id = $7)
),
pagina AS (
  SELECT * FROM base
   WHERE escalon = ANY($1::text[])
   ORDER BY creado, id
   LIMIT $5 OFFSET $6
)
SELECT (SELECT count(*) FROM base WHERE escalon = $4),
       p.id, p.titulo, p.titulo_original, p.fuente, p.modalidad, p.reporte_id, p.periodo,
       p.ids_fuente, p.evidencia, p.escalon, p.creado, p.resuelto_en,
       p.resuelto_por, us.nombre, p.obra_id, oa.titulo, p.nota_resolucion,
       COALESCE(cm.candidatos, '[]'::jsonb)
  FROM (SELECT 1) uno
  LEFT JOIN pagina p ON true
  LEFT JOIN usuarios us ON us.id = p.resuelto_por
  LEFT JOIN obras oa ON oa.id = p.obra_id
  LEFT JOIN LATERAL (
    SELECT jsonb_agg(
             jsonb_build_object('obra_id', c.obra_id, 'titulo', o.titulo, 'anio', o.anio,
                                'genero', o.genero, 'puntaje', c.puntaje,
                                'titulo_consultado', c.titulo_consultado)
             ORDER BY c.orden, c.obra_id
           ) AS candidatos
      FROM candidatos_match c
      JOIN obras o ON o.id = c.obra_id
     WHERE c.uso_id = p.id
  ) cm ON true
 ORDER BY p.creado, p.id`

// ListarCasosIdentificacion lee una pagina de la cola manual y el total de pendientes bajo los mismos filtros.
func (s *Store) ListarCasosIdentificacion(ctx context.Context, q aplicacion.ConsultaCasos) (aplicacion.PaginaCasos, error) {
	p := q.ConDefecto()
	var limite any = p.Limite
	if p.Limite == aplicacion.LimiteSinTope {
		limite = nil
	}
	filas, err := s.ejecutorDe(ctx).Query(ctx, sqlCasosIdentificacion,
		q.Escalones, q.Fuente, q.Periodo, identificacion.EscalonONI, limite, p.Desplazamiento, q.UsoID)
	if err != nil {
		return aplicacion.PaginaCasos{}, traducirError(err, "listar casos de identificacion")
	}
	defer filas.Close()

	pag := aplicacion.PaginaCasos{Casos: []aplicacion.CasoIdentificacion{}}
	for filas.Next() {
		var (
			id, titulo, tituloOrig, fuente, modalidad, reporteID, periodo *string
			idsFuente, evidencia, escalon, nota                           *string
			resueltoPor, nombre, obraID, obraTitulo                       *string
			creado, resueltoEn                                            *time.Time
			candidatosJSON                                                []byte
		)
		if err := filas.Scan(&pag.Pendientes, &id, &titulo, &tituloOrig, &fuente, &modalidad,
			&reporteID, &periodo, &idsFuente, &evidencia, &escalon, &creado, &resueltoEn,
			&resueltoPor, &nombre, &obraID, &obraTitulo, &nota, &candidatosJSON); err != nil {
			return aplicacion.PaginaCasos{}, traducirError(err, "escanear caso de identificacion")
		}
		if id == nil {
			continue
		}
		candidatos, err := decodificarCandidatos(candidatosJSON)
		if err != nil {
			return aplicacion.PaginaCasos{}, fmt.Errorf("caso %q: %w", *id, err)
		}
		caso := aplicacion.CasoIdentificacion{
			UsoID: *id, Titulo: *titulo, TituloOriginal: *tituloOrig, Fuente: *fuente,
			Modalidad: *modalidad, ReporteID: *reporteID, Periodo: *periodo,
			IDsFuente: *idsFuente, Evidencia: *evidencia, Escalon: *escalon,
			ReporteCreado: *creado, ResueltoEn: resueltoEn, Candidatos: candidatos,
			Nota: valorDe(nota),
		}
		if obraID != nil {
			caso.ObraAsignada = &aplicacion.ObraAsignada{ID: *obraID, Titulo: valorDe(obraTitulo)}
		}
		if resueltoPor != nil {
			caso.ResueltoPor = &aplicacion.Resolutor{ID: *resueltoPor, Nombre: valorDe(nombre)}
		}
		pag.Casos = append(pag.Casos, caso)
	}
	if err := filas.Err(); err != nil {
		return aplicacion.PaginaCasos{}, traducirError(err, "listar casos de identificacion")
	}
	return pag, nil
}

func valorDe(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// decodificarCandidatos traduce el jsonb_agg de la bandeja ambigua.
func decodificarCandidatos(bruto []byte) ([]aplicacion.CandidatoCaso, error) {
	var filas []struct {
		ObraID           string          `json:"obra_id"`
		Titulo           string          `json:"titulo"`
		Anio             int             `json:"anio"`
		Genero           string          `json:"genero"`
		Puntaje          decimal.Decimal `json:"puntaje"`
		TituloConsultado string          `json:"titulo_consultado"`
	}
	if err := json.Unmarshal(bruto, &filas); err != nil {
		return nil, fmt.Errorf("decodificar candidatos: %w", err)
	}
	candidatos := make([]aplicacion.CandidatoCaso, 0, len(filas))
	for _, f := range filas {
		candidatos = append(candidatos, aplicacion.CandidatoCaso{
			ObraID: f.ObraID, Titulo: f.Titulo, Anio: f.Anio, Genero: f.Genero,
			Puntaje: f.Puntaje, TituloConsultado: f.TituloConsultado,
		})
	}
	return candidatos, nil
}
