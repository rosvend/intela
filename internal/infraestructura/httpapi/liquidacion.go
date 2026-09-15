package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ ConsultaLiquidaciones = aplicacion.Liquidaciones{}

// ConsultaLiquidaciones es lo que la capa HTTP necesita del nucleo para
// servir liquidaciones. Se declara aqui, igual que [Autenticacion]: el
// adaptador depende de dos metodos, no del struct que los implementa.
type ConsultaLiquidaciones interface {
	Listar(ctx context.Context, actor aplicacion.Usuario) ([]aplicacion.OrdenVista, error)
	DeTitular(ctx context.Context, actor aplicacion.Usuario) ([]aplicacion.OrdenVista, error)
}

type deduccionJSON struct {
	Concepto string `json:"concepto"`
	Monto    string `json:"monto"`
}

type ordenJSON struct {
	ID          string          `json:"id"`
	ProcesoID   string          `json:"proceso_id"`
	TitularID   string          `json:"titular_id"`
	Periodo     string          `json:"periodo"`
	Bruto       string          `json:"bruto"`
	Deducciones []deduccionJSON `json:"deducciones"`
	Neto        string          `json:"neto"`
	Estado      string          `json:"estado"`
	Pagable     bool            `json:"pagable"`
	Enviada     string          `json:"enviada"`
}

type listadoJSON struct {
	Liquidaciones []ordenJSON `json:"liquidaciones"`
}

func aOrdenJSON(v aplicacion.OrdenVista) ordenJSON {
	o := v.Orden
	deducciones := make([]deduccionJSON, 0, len(o.Deducciones))
	for _, d := range o.Deducciones {
		deducciones = append(deducciones, deduccionJSON{
			Concepto: d.Concepto,
			Monto:    d.Monto.StringFixed(2),
		})
	}
	return ordenJSON{
		ID:          o.ID,
		ProcesoID:   o.ProcesoID,
		TitularID:   o.TitularID,
		Periodo:     o.Periodo,
		Bruto:       o.Bruto.StringFixed(2),
		Deducciones: deducciones,
		Neto:        o.Neto.StringFixed(2),
		Estado:      string(o.Estado),
		Pagable:     v.Pagable,
		Enviada:     o.EnviadaDia,
	}
}

func aListadoJSON(vistas []aplicacion.OrdenVista) listadoJSON {
	ordenes := make([]ordenJSON, 0, len(vistas))
	for _, v := range vistas {
		ordenes = append(ordenes, aOrdenJSON(v))
	}
	return listadoJSON{Liquidaciones: ordenes}
}

func (a *API) listarLiquidaciones(w http.ResponseWriter, r *http.Request) {
	a.servirLiquidaciones(w, r, a.liq.Listar)
}

func (a *API) misLiquidaciones(w http.ResponseWriter, r *http.Request) {
	a.servirLiquidaciones(w, r, a.liq.DeTitular)
}

func (a *API) servirLiquidaciones(w http.ResponseWriter, r *http.Request, fn func(context.Context, aplicacion.Usuario) ([]aplicacion.OrdenVista, error)) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	vistas, err := fn(r.Context(), actor)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, "no autorizado")
		return
	case errors.Is(err, aplicacion.ErrParametroAusente):
		a.log.ErrorContext(r.Context(), "parametro normativo ausente", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "parametro normativo ausente")
		return
	default:
		a.log.ErrorContext(r.Context(), "no se pudieron leer las liquidaciones", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudieron leer las liquidaciones")
		return
	}
	escribirJSON(w, http.StatusOK, aListadoJSON(vistas))
}
