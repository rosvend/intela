package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// CasosIdentificacion es la lectura de la cola manual que consume la bandeja de #39.
type CasosIdentificacion interface {
	Listar(ctx context.Context, f aplicacion.FiltroCasos) (aplicacion.PaginaCasos, error)
}

var _ CasosIdentificacion = aplicacion.CasosIdentificacion{}

// paginaCasosJSON cuadra con el schema PaginaCasosIdentificacion de api/openapi.yaml.
type paginaCasosJSON struct {
	Casos      []casoJSON `json:"casos"`
	Pendientes int        `json:"pendientes"`
}

type casoJSON struct {
	ID                  string          `json:"id"`
	Titulo              string          `json:"titulo"`
	TituloOriginal      string          `json:"titulo_original"`
	Fuente              string          `json:"fuente"`
	Modalidad           string          `json:"modalidad"`
	ReporteID           string          `json:"reporte_id"`
	Periodo             string          `json:"periodo"`
	IDsFuente           string          `json:"ids_fuente"`
	Evidencia           string          `json:"evidencia"`
	Estado              string          `json:"estado"`
	Candidatos          []candidatoJSON `json:"candidatos"`
	ObraAsignada        *obraRefJSON    `json:"obra_asignada"`
	ResueltoPor         *resolutorJSON  `json:"resuelto_por"`
	ResueltoEn          *string         `json:"resuelto_en"`
	UltimaActualizacion string          `json:"ultima_actualizacion"`
	// Nota es null en un caso pendiente y el texto de la decision en uno
	// resuelto: la justificacion es obligatoria al resolver (#175, D5).
	Nota *string `json:"nota"`

	// Sugerencia siempre viaja: "ninguna" es la ausencia de propuesta, no
	// la falta del campo. Aceptada es null mientras nadie ha resuelto.
	Sugerencia sugerenciaJSON `json:"sugerencia"`
}

type candidatoJSON struct {
	ObraID           string                `json:"obra_id"`
	Titulo           string                `json:"titulo"`
	Anio             int                   `json:"anio"`
	Genero           string                `json:"genero"`
	Puntaje          decimalComoNumeroJSON `json:"puntaje"`
	TituloConsultado string                `json:"titulo_consultado"`
}

type obraRefJSON struct {
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
}

type resolutorJSON struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

type sugerenciaJSON struct {
	Decision  string                `json:"decision"`
	ObraID    *string               `json:"obra_id"`
	Titulo    *string               `json:"titulo"`
	Confianza decimalComoNumeroJSON `json:"confianza"`
	Motivo    string                `json:"motivo"`
	Orden     []string              `json:"orden"`
	Aceptada  *bool                 `json:"aceptada"`
	// Sello es null cuando no hay propuesta verificable que devolver al
	// confirmar: un caso ya resuelto, o una bandeja sin sello.
	Sello *string `json:"sello"`
}

func (a *API) listarCasosIdentificacion(w http.ResponseWriter, r *http.Request) {
	if a.identificacion == nil {
		escribirError(w, http.StatusServiceUnavailable, "la cola de identificacion no esta disponible")
		return
	}
	q := r.URL.Query()
	pag, ok := leerPaginacion(w, q)
	if !ok {
		return
	}
	res, err := a.identificacion.Listar(r.Context(), aplicacion.FiltroCasos{
		Estado: q.Get("estado"), Fuente: q.Get("fuente"), Periodo: q.Get("periodo"), Paginacion: pag,
	})
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrFiltroCasosInvalido):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al listar los casos de identificacion", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo leer la cola de identificacion")
		return
	}
	out := paginaCasosJSON{Casos: make([]casoJSON, 0, len(res.Casos)), Pendientes: res.Pendientes}
	for _, c := range res.Casos {
		out.Casos = append(out.Casos, aCasoJSON(c))
	}
	escribirJSON(w, http.StatusOK, out)
}

func aCasoJSON(c aplicacion.CasoIdentificacion) casoJSON {
	out := casoJSON{
		ID: c.UsoID, Titulo: c.Titulo, TituloOriginal: c.TituloOriginal, Fuente: c.Fuente,
		Modalidad: c.Modalidad, ReporteID: c.ReporteID, Periodo: c.Periodo, IDsFuente: c.IDsFuente,
		Evidencia: c.Evidencia, Estado: c.Estado,
		Candidatos:          make([]candidatoJSON, 0, len(c.Candidatos)),
		UltimaActualizacion: c.UltimaActualizacion.UTC().Format(time.RFC3339),
	}
	for _, cand := range c.Candidatos {
		out.Candidatos = append(out.Candidatos, candidatoJSON{
			ObraID: cand.ObraID, Titulo: cand.Titulo, Anio: cand.Anio, Genero: cand.Genero,
			Puntaje: decimalComoNumeroJSON(cand.Puntaje), TituloConsultado: cand.TituloConsultado,
		})
	}
	if c.ObraAsignada != nil {
		out.ObraAsignada = &obraRefJSON{ID: c.ObraAsignada.ID, Titulo: c.ObraAsignada.Titulo}
	}
	if c.ResueltoPor != nil {
		out.ResueltoPor = &resolutorJSON{ID: c.ResueltoPor.ID, Nombre: c.ResueltoPor.Nombre}
	}
	if c.ResueltoEn != nil {
		s := c.ResueltoEn.UTC().Format(time.RFC3339)
		out.ResueltoEn = &s
	}
	// null y no "": un caso pendiente no tiene nota, y la cadena vacia se lee
	// como "la nota esta en blanco", que el servidor rechaza al resolver.
	if c.Nota != "" {
		nota := c.Nota
		out.Nota = &nota
	}
	out.Sugerencia = aSugerenciaJSON(c.Sugerencia)
	return out
}

func aSugerenciaJSON(s *aplicacion.SugerenciaCaso) sugerenciaJSON {
	if s == nil {
		return sugerenciaJSON{Decision: "ninguna", Orden: []string{}}
	}
	out := sugerenciaJSON{
		Decision:  s.Decision,
		Confianza: decimalComoNumeroJSON(s.Confianza),
		Motivo:    s.Motivo,
		Orden:     s.Orden,
		Aceptada:  s.Aceptada,
	}
	if out.Decision == "" {
		out.Decision = "ninguna"
	}
	if out.Orden == nil {
		out.Orden = []string{}
	}
	if s.ObraID != "" {
		obra := s.ObraID
		out.ObraID = &obra
	}
	if s.Titulo != "" {
		titulo := s.Titulo
		out.Titulo = &titulo
	}
	if s.Sello != "" {
		sello := s.Sello
		out.Sello = &sello
	}
	return out
}
