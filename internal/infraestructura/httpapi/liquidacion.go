package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Liquidaciones es lo que la capa HTTP necesita del nucleo para el panel
// y el export. Se declara en el consumidor, igual que [Autenticacion].
type Liquidaciones interface {
	Consultar(ctx context.Context, actor aplicacion.Usuario, periodo string) (aplicacion.Liquidacion, error)
	Exportar(ctx context.Context, actor aplicacion.Usuario, periodo, formato string) (aplicacion.Archivo, error)
}

type montoJSON string

func aMonto(d decimal.Decimal) montoJSON {
	return montoJSON(d.StringFixed(2))
}

type lineaLiquidacionJSON struct {
	Periodo string    `json:"periodo"`
	ObraID  string    `json:"obra_id"`
	Titulo  string    `json:"titulo"`
	Bruto   montoJSON `json:"bruto"`
	Admin   montoJSON `json:"admin"`
	Social  montoJSON `json:"social"`
	Reserva montoJSON `json:"reserva"`
	Neto    montoJSON `json:"neto"`
}

type totalesJSON struct {
	Bruto   montoJSON `json:"bruto"`
	Admin   montoJSON `json:"admin"`
	Social  montoJSON `json:"social"`
	Reserva montoJSON `json:"reserva"`
	Neto    montoJSON `json:"neto"`
}

type liquidacionJSON struct {
	TitularID string                 `json:"titular_id"`
	Periodo   string                 `json:"periodo"`
	Lineas    []lineaLiquidacionJSON `json:"lineas"`
	Totales   totalesJSON            `json:"totales"`
}

func aLiquidacionJSON(l aplicacion.Liquidacion) liquidacionJSON {
	lineas := make([]lineaLiquidacionJSON, 0, len(l.Lineas))
	for _, ln := range l.Lineas {
		lineas = append(lineas, lineaLiquidacionJSON{
			Periodo: ln.Periodo,
			ObraID:  ln.ObraID,
			Titulo:  ln.Titulo,
			Bruto:   aMonto(ln.Bruto),
			Admin:   aMonto(ln.Admin),
			Social:  aMonto(ln.Social),
			Reserva: aMonto(ln.Reserva),
			Neto:    aMonto(ln.Neto),
		})
	}
	return liquidacionJSON{
		TitularID: l.TitularID,
		Periodo:   l.Periodo,
		Lineas:    lineas,
		Totales: totalesJSON{
			Bruto:   aMonto(l.Totales.Bruto),
			Admin:   aMonto(l.Totales.Admin),
			Social:  aMonto(l.Totales.Social),
			Reserva: aMonto(l.Totales.Reserva),
			Neto:    aMonto(l.Totales.Neto),
		},
	}
}

func (a *API) consultarLiquidaciones(w http.ResponseWriter, r *http.Request) {
	actor, ok := UsuarioDe(r.Context())
	if !ok {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	liq, err := a.liq.Consultar(r.Context(), actor, r.URL.Query().Get("periodo"))
	if !escribirErrorLiquidacion(w, err) {
		return
	}
	escribirJSON(w, http.StatusOK, aLiquidacionJSON(liq))
}

func (a *API) exportarLiquidaciones(w http.ResponseWriter, r *http.Request) {
	actor, ok := UsuarioDe(r.Context())
	if !ok {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	q := r.URL.Query()
	archivo, err := a.liq.Exportar(r.Context(), actor, q.Get("periodo"), q.Get("formato"))
	if !escribirErrorLiquidacion(w, err) {
		return
	}

	h := w.Header()
	h.Set("Content-Type", archivo.TipoMIME)
	h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, archivo.Nombre))
	h.Set("Content-Length", fmt.Sprintf("%d", len(archivo.Contenido)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archivo.Contenido)
}

func escribirErrorLiquidacion(w http.ResponseWriter, err error) bool {
	if err == nil {
		return true
	}
	switch {
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, aplicacion.ErrNoAutorizado.Error())
	case errors.Is(err, aplicacion.ErrFormatoInvalido):
		escribirError(w, http.StatusBadRequest, "formato tiene que ser pdf o xlsx")
	case errors.Is(err, aplicacion.ErrPeriodoInvalido):
		escribirError(w, http.StatusBadRequest, "periodo tiene que ser YYYY o YYYY-MM")
	default:
		escribirError(w, http.StatusInternalServerError, "no se pudo generar la liquidacion")
	}
	return false
}
