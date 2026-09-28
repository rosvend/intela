package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ConsultaIngresos es lo que la capa HTTP necesita del panel del titular.
type ConsultaIngresos interface {
	MisIngresos(ctx context.Context, actor aplicacion.Usuario, f aplicacion.FiltroIngresos) ([]aplicacion.Ingreso, error)
}

type ingresoJSON struct {
	Ref     string `json:"ref"`
	ObraID  string `json:"obra_id"`
	Obra    string `json:"obra"`
	Fuente  string `json:"fuente"`
	Periodo string `json:"periodo"`
	Neto    string `json:"neto"`
}

type listaIngresosJSON struct {
	Ingresos []ingresoJSON `json:"ingresos"`
}

func aIngresoJSON(i aplicacion.Ingreso) ingresoJSON {
	return ingresoJSON{
		Ref:     i.Ref,
		ObraID:  i.ObraID,
		Obra:    i.Obra,
		Fuente:  i.Fuente,
		Periodo: i.Periodo,
		Neto:    dinero(i.Neto),
	}
}

func dinero(d decimal.Decimal) string {
	return d.StringFixed(2)
}

func (a *API) misIngresos(w http.ResponseWriter, r *http.Request) {
	if a.ingresos == nil {
		escribirError(w, http.StatusServiceUnavailable,
			"el panel de ingresos no esta configurado en esta instalacion")
		return
	}
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	q := r.URL.Query()
	filas, err := a.ingresos.MisIngresos(r.Context(), actor, aplicacion.FiltroIngresos{
		ObraID:  q.Get("obra"),
		Fuente:  q.Get("fuente"),
		Periodo: q.Get("periodo"),
	})
	if err != nil {
		escribirFallo(w, err)
		return
	}
	cuerpo := make([]ingresoJSON, 0, len(filas))
	for _, f := range filas {
		cuerpo = append(cuerpo, aIngresoJSON(f))
	}
	escribirJSON(w, http.StatusOK, listaIngresosJSON{Ingresos: cuerpo})
}

func escribirFallo(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, aplicacion.ErrNoAutorizado.Error())
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, aplicacion.ErrNoEncontrado.Error())
	default:
		escribirError(w, http.StatusInternalServerError, "no se pudo completar la consulta")
	}
}
