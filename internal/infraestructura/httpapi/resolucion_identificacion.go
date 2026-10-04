package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// ResolucionIdentificacion es la escritura de la cola manual (#175): asignar un
// caso ONI a una obra o descartarlo.
type ResolucionIdentificacion interface {
	Resolver(ctx context.Context, s aplicacion.SolicitudResolucion,
		actorID, actorNombre string) (aplicacion.CasoIdentificacion, error)
}

var _ ResolucionIdentificacion = aplicacion.ResolucionIdentificacion{}

// maxCuerpoResolucion acota el cuerpo, igual que los POST de /alertas: es la
// decision, la obra, la nota y el sello de la propuesta mostrada, asi que
// 64 KiB sobra.
const maxCuerpoResolucion = 64 << 10

// resolucionDeCasoJSON es el cuerpo: la decision, la obra cuando la hay, la
// nota y, si la bandeja lo dio, el sello de la propuesta mostrada. NO lleva
// actor: quien resolvio sale de la sesion (ADR 0006).
type resolucionDeCasoJSON struct {
	Decision string `json:"decision"`
	ObraID   string `json:"obra_id"`
	Nota     string `json:"nota"`
	Sello    string `json:"sello"`
}

// resolverCasoIdentificacion resuelve un caso de la cola manual a nombre de
// quien lo resuelve.
//
// El actor sale de la SESION y no del cuerpo, por lo mismo que en
// [API.resolverAlerta]: dejarlo llegar por JSON permitiria firmar la decision a
// nombre de otro, y el asiento del ADR 0006 tiene que nombrar a quien la tomo
// de verdad. Un campo `actor` colado en el JSON se ignora sin mas.
//
// Un POST sin cuerpo llega al caso de uso con nota vacia y sale 400 por
// identificacion.ErrNotaVacia.
func (a *API) resolverCasoIdentificacion(w http.ResponseWriter, r *http.Request) {
	if a.resolucion == nil {
		escribirError(w, http.StatusServiceUnavailable, "la resolucion de casos no esta disponible")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoResolucion)

	var cuerpo resolucionDeCasoJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil && !errors.Is(err, io.EOF) {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con decision, obra_id y nota")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	// Los ids de uso son TEXT, no UUID: no se valida forma aqui. Un id
	// desconocido lo dice el caso de uso con ErrNoEncontrado.
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	caso, err := a.resolucion.Resolver(r.Context(), aplicacion.SolicitudResolucion{
		UsoID:    id,
		Decision: cuerpo.Decision,
		ObraID:   cuerpo.ObraID,
		Nota:     cuerpo.Nota,
		Sello:    cuerpo.Sello,
	}, usuario.ID, usuario.Nombre)

	switch {
	case err == nil:
	case errors.Is(err, identificacion.ErrNotaVacia),
		errors.Is(err, identificacion.ErrNotaDemasiadoLarga),
		errors.Is(err, identificacion.ErrDecisionInvalida),
		errors.Is(err, aplicacion.ErrObraInexistente),
		errors.Is(err, aplicacion.ErrPropuestaInvalida):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "ese caso no existe")
		return
	case errors.Is(err, identificacion.ErrCasoYaResuelto),
		errors.Is(err, identificacion.ErrCasoNoPendiente),
		errors.Is(err, aplicacion.ErrAliasEnConflicto):
		// 409 y no 200: quien pulso el boton el segundo tiene que saber que la
		// firma que quedo escrita no es la suya, y las tres causas se
		// distinguen por el mensaje (el front de #39 pinta "Recargar caso").
		escribirError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, aplicacion.ErrActorAusente):
		// Defensa: el middleware ya exigio sesion, asi que llegar aqui es que
		// el actor se perdio por el camino.
		noAutenticado(w, "sesion invalida o expirada")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al resolver un caso de identificacion", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo resolver el caso")
		return
	}

	escribirJSON(w, http.StatusOK, aCasoJSON(caso))
}
