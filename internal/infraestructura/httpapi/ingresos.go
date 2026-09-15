package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ConsultaIngresos es lo que la capa HTTP necesita del panel del titular.
type ConsultaIngresos interface {
	MisIngresos(ctx context.Context, actor aplicacion.Usuario, f aplicacion.FiltroIngresos) ([]aplicacion.Ingreso, error)
}

// ExplicarCifra es lo que la capa HTTP necesita del linaje de una cifra.
type ExplicarCifra interface {
	Explicar(ctx context.Context, actor aplicacion.Usuario, ref string) (aplicacion.Explicacion, error)
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

type corridaJSON struct {
	ProcesoID string `json:"proceso_id"`
	Periodo   string `json:"periodo"`
	Circuito  string `json:"circuito"`
}

type reporteJSON struct {
	ID     string `json:"id"`
	Fuente string `json:"fuente"`
	SHA256 string `json:"sha256"`
}

type obraLinajeJSON struct {
	ID      string `json:"id"`
	Titulo  string `json:"titulo"`
	Escalon string `json:"escalon"`
	Puntaje string `json:"puntaje"`
}

type reglaJSON struct {
	SnapshotID string `json:"snapshot_id"`
	Reglamento string `json:"reglamento"`
}

type splitJSON struct {
	TitularID  string `json:"titular_id"`
	IPI        string `json:"ipi"`
	Porcentaje string `json:"porcentaje"`
	Version    int    `json:"version"`
}

type deduccionJSON struct {
	Concepto   string `json:"concepto"`
	Porcentaje string `json:"porcentaje"`
	Monto      string `json:"monto"`
}

type explicacionJSON struct {
	Ref         string          `json:"ref"`
	Neto        string          `json:"neto"`
	Bruto       string          `json:"bruto"`
	Corrida     corridaJSON     `json:"corrida"`
	Reporte     reporteJSON     `json:"reporte"`
	Obra        obraLinajeJSON  `json:"obra"`
	Regla       reglaJSON       `json:"regla"`
	Split       splitJSON       `json:"split"`
	Deducciones []deduccionJSON `json:"deducciones"`
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

func aExplicacionJSON(x aplicacion.Explicacion) explicacionJSON {
	ded := make([]deduccionJSON, 0, len(x.Deducciones))
	for _, d := range x.Deducciones {
		ded = append(ded, deduccionJSON{
			Concepto:   d.Concepto,
			Porcentaje: d.Porcentaje.StringFixed(2),
			Monto:      dinero(d.Monto),
		})
	}
	return explicacionJSON{
		Ref:   x.Ref,
		Neto:  dinero(x.Neto),
		Bruto: dinero(x.Bruto),
		Corrida: corridaJSON{
			ProcesoID: x.Corrida.ProcesoID,
			Periodo:   x.Corrida.Periodo,
			Circuito:  x.Corrida.Circuito,
		},
		Reporte: reporteJSON{
			ID:     x.Reporte.ID,
			Fuente: x.Reporte.Fuente,
			SHA256: x.Reporte.SHA256,
		},
		Obra: obraLinajeJSON{
			ID:      x.Obra.ID,
			Titulo:  x.Obra.Titulo,
			Escalon: x.Obra.Escalon,
			Puntaje: x.Obra.Puntaje.StringFixed(5),
		},
		Regla: reglaJSON{
			SnapshotID: x.Regla.SnapshotID,
			Reglamento: x.Regla.Reglamento,
		},
		Split: splitJSON{
			TitularID:  x.Split.TitularID,
			IPI:        x.Split.IPI,
			Porcentaje: x.Split.Porcentaje.StringFixed(4),
			Version:    x.Split.Version,
		},
		Deducciones: ded,
	}
}

func dinero(d decimal.Decimal) string {
	return d.StringFixed(2)
}

func (a *API) misIngresos(w http.ResponseWriter, r *http.Request) {
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

func (a *API) explicar(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	x, err := a.explicarCifra.Explicar(r.Context(), actor, chi.URLParam(r, "ref"))
	if err != nil {
		escribirFallo(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, aExplicacionJSON(x))
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
