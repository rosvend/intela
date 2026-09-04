package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// LecturaONI es lo que la capa HTTP necesita para servir el listado publico.
type LecturaONI interface {
	Ejecutar(ctx context.Context, periodo string) (aplicacion.PublicacionONI, error)
}

// EscrituraONI es lo que la capa HTTP necesita para publicar un periodo.
type EscrituraONI interface {
	Ejecutar(ctx context.Context, periodo, actorID string) (aplicacion.PublicacionONI, error)
}

type obraONIJSON struct {
	ID        string `json:"id"`
	Titulo    string `json:"titulo"`
	Fuente    string `json:"fuente"`
	IDsFuente string `json:"ids_fuente"`
	Modalidad string `json:"modalidad"`
}

type listadoONIJSON struct {
	Periodo              string        `json:"periodo"`
	FechaProceso         string        `json:"fecha_proceso"`
	DireccionFisica      string        `json:"direccion_fisica"`
	DireccionElectronica string        `json:"direccion_electronica"`
	Explicacion          string        `json:"explicacion"`
	Obras                []obraONIJSON `json:"obras"`
}

type publicarONIJSON struct {
	Periodo string `json:"periodo"`
}

func aListadoONIJSON(p aplicacion.PublicacionONI) listadoONIJSON {
	obras := make([]obraONIJSON, 0, len(p.Obras))
	for _, o := range p.Obras {
		obras = append(obras, obraONIJSON{
			ID:        o.ID,
			Titulo:    o.Titulo,
			Fuente:    o.Fuente,
			IDsFuente: o.IDsFuente,
			Modalidad: o.Modalidad,
		})
	}
	return listadoONIJSON{
		Periodo:              p.Periodo,
		FechaProceso:         p.FechaProceso.UTC().Format(time.RFC3339),
		DireccionFisica:      p.DireccionFisica,
		DireccionElectronica: p.DireccionElectronica,
		Explicacion:          aplicacion.ExplicacionListadoONI,
		Obras:                obras,
	}
}

// obtenerListadoONI sirve GET /publico/oni. Sin sesion: R-18 es una
// publicacion en la web, no un informe interno.
func (a *API) obtenerListadoONI(w http.ResponseWriter, r *http.Request) {
	if a.listadoONI == nil {
		escribirError(w, http.StatusInternalServerError, "listado ONI no disponible")
		return
	}
	pub, err := a.listadoONI.Ejecutar(r.Context(), r.URL.Query().Get("periodo"))
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "no hay listado ONI publicado")
		return
	case errors.Is(err, aplicacion.ErrPeriodoInvalido):
		escribirError(w, http.StatusBadRequest, "periodo invalido")
		return
	default:
		a.log.ErrorContext(r.Context(), "no se pudo leer el listado ONI", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo leer el listado ONI")
		return
	}
	escribirJSON(w, http.StatusOK, aListadoONIJSON(pub))
}

// crearPublicacionONI congela el listado de un periodo. Solo distribucion
// y administrador: publicar es el hecho juridico que arranca R-19.
func (a *API) crearPublicacionONI(w http.ResponseWriter, r *http.Request) {
	if a.publicarONI == nil {
		escribirError(w, http.StatusInternalServerError, "publicacion ONI no disponible")
		return
	}
	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	var cuerpo publicarONIJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con periodo")
		return
	}

	pub, err := a.publicarONI.Ejecutar(r.Context(), cuerpo.Periodo, usuario.ID)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrPeriodoInvalido):
		escribirError(w, http.StatusBadRequest, "periodo invalido")
		return
	case errors.Is(err, aplicacion.ErrYaPublicado):
		escribirError(w, http.StatusConflict, "el listado ONI de ese periodo ya fue publicado")
		return
	case errors.Is(err, aplicacion.ErrDireccionPublicacionAusente):
		a.log.ErrorContext(r.Context(), "faltan las direcciones de publicacion ONI")
		escribirError(w, http.StatusInternalServerError, "faltan las direcciones de publicacion ONI")
		return
	default:
		a.log.ErrorContext(r.Context(), "no se pudo publicar el listado ONI", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo publicar el listado ONI")
		return
	}
	escribirJSON(w, http.StatusCreated, aListadoONIJSON(pub))
}

// conRoles exige que el Usuario del contexto tenga uno de los roles.
//
// Va DETRAS de conSesion: sin Usuario, el cero tiene Rol vacio y se
// rechazaria igual, pero responder 401 (no autenticado) es mas honesto que
// 403 (autenticado y no basta).
func (a *API) conRoles(roles ...aplicacion.Rol) func(http.Handler) http.Handler {
	return func(siguiente http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			usuario, hay := UsuarioDe(r.Context())
			if !hay {
				noAutenticado(w, "sesion invalida o expirada")
				return
			}
			if !slices.Contains(roles, usuario.Rol) {
				escribirError(w, http.StatusForbidden, "no autorizado")
				return
			}
			siguiente.ServeHTTP(w, r)
		})
	}
}
