package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ ColaRevision = aplicacion.Normalizacion{}

// itemRevisionJSON es la forma de red de la cola de revision. Cuadra con
// el schema ItemRevision de api/openapi.yaml. El nucleo no lleva etiquetas
// json: la forma que viaja la decide este fichero.
type itemRevisionJSON struct {
	ID        string `json:"id"`
	Tipo      string `json:"tipo"`
	Codigo    string `json:"codigo"`
	Motivo    string `json:"motivo"`
	Fuente    string `json:"fuente"`
	Titulo    string `json:"titulo"`
	ReporteID string `json:"reporte_id"`
}

func (a *API) listarColaRevision(w http.ResponseWriter, r *http.Request) {
	if a.cola == nil {
		escribirJSON(w, http.StatusOK, []itemRevisionJSON{})
		return
	}
	items, err := a.cola.ListarRevision(r.Context())
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al listar la cola de revision", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo leer la cola de revision")
		return
	}
	out := make([]itemRevisionJSON, 0, len(items))
	for _, it := range items {
		out = append(out, aItemRevisionJSON(it))
	}
	escribirJSON(w, http.StatusOK, out)
}

func aItemRevisionJSON(it aplicacion.ItemRevision) itemRevisionJSON {
	return itemRevisionJSON{
		ID:        it.ID,
		Tipo:      it.Tipo,
		Codigo:    it.Codigo,
		Motivo:    it.Motivo,
		Fuente:    it.Fuente,
		Titulo:    it.Titulo,
		ReporteID: it.ReporteID,
	}
}
