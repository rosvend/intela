package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Auditoria es lo que la capa HTTP necesita del nucleo para el Portal de
// Auditoria (OE-7), declarado aqui igual que [Catalogo]: la direccion es
// httpapi -> aplicacion, nunca httpapi -> postgres.
type Auditoria interface {
	Asientos(ctx context.Context, pag aplicacion.Paginacion) ([]aplicacion.Asiento, error)
	HistorialDeObra(ctx context.Context, obraID string) ([]aplicacion.Asiento, error)
}

// ---------------------------------------------------------------------------
// Formas de red

// asientoJSON es un asiento de la bitacora tal como lo ve el auditor: quien
// hizo que, sobre que referencia, cuando, y el detalle del hecho.
//
// El payload viaja tal cual lo guardo el modulo que asento -es su evidencia,
// no una proyeccion-: la vista lo renderiza por familia de hecho
// (`declaracion.guardada` trae version/estado/partes, `recaudo.registrado`
// trae periodo/circuito/bruto/convenio/tarifa/factura) y muestra el resto como
// pares clave-valor. Tiene que cuadrar con el schema `Asiento` de
// api/openapi.yaml.
type asientoJSON struct {
	ID      string          `json:"id"`
	Hecho   string          `json:"hecho"`
	RefTipo string          `json:"ref_tipo"`
	RefID   string          `json:"ref_id"`
	Actor   string          `json:"actor"`
	Payload json.RawMessage `json:"payload"`
	Cuando  string          `json:"cuando"`
}

func aAsientoJSON(a aplicacion.Asiento) asientoJSON {
	payload := json.RawMessage(a.Payload)
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return asientoJSON{
		ID:      a.ID,
		Hecho:   a.Hecho,
		RefTipo: a.RefTipo,
		RefID:   a.RefID,
		Actor:   a.ActorID,
		Payload: payload,
		Cuando:  a.Cuando.Format(time.RFC3339Nano),
	}
}

func aAsientosJSON(asientos []aplicacion.Asiento) []asientoJSON {
	cuerpo := make([]asientoJSON, 0, len(asientos))
	for _, a := range asientos {
		cuerpo = append(cuerpo, aAsientoJSON(a))
	}
	return cuerpo
}

// ---------------------------------------------------------------------------
// Handlers

// listarAsientos sirve la pagina de la bitacora en orden de timeline. Solo
// lectura: no hay POST, PUT ni DELETE bajo /auditoria, y la tabla ademas los
// rechaza por trigger (ADR 0006).
func (a *API) listarAsientos(w http.ResponseWriter, r *http.Request) {
	if a.auditoria == nil {
		escribirError(w, http.StatusServiceUnavailable,
			"la lectura de la bitacora no esta configurada en esta instalacion")
		return
	}
	pag, ok := leerPaginacion(w, r.URL.Query())
	if !ok {
		return
	}
	asientos, err := a.auditoria.Asientos(r.Context(), pag)
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al leer la bitacora", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar la bitacora")
		return
	}
	escribirJSON(w, http.StatusOK, aAsientosJSON(asientos))
}

// historialDeObra sirve los asientos que referencian a una obra, en orden de
// cadena. Una obra sin asientos devuelve una lista vacia, no un 404: mismo
// criterio que el historial de declaraciones.
func (a *API) historialDeObra(w http.ResponseWriter, r *http.Request) {
	if a.auditoria == nil {
		escribirError(w, http.StatusServiceUnavailable,
			"la lectura de la bitacora no esta configurada en esta instalacion")
		return
	}
	asientos, err := a.auditoria.HistorialDeObra(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al leer el historial de una obra", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar la bitacora")
		return
	}
	escribirJSON(w, http.StatusOK, aAsientosJSON(asientos))
}
