package httpapi

import (
	"context"
	"net/http"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Tablero es lo que la capa HTTP necesita de las tarjetas del panel de inicio.
type Tablero interface {
	MisObras(ctx context.Context, actor aplicacion.Usuario) ([]aplicacion.ObraResumen, error)
	UltimaLiquidacion(ctx context.Context, actor aplicacion.Usuario) (aplicacion.ResumenLiquidacion, error)
	CargasPendientes(ctx context.Context) (int, error)
	ObrasEnReserva(ctx context.Context) (int, error)
	ONIPendientes(ctx context.Context) (int, error)
	UltimaCorrida(ctx context.Context) (aplicacion.CorridaResumen, error)
}

type obraResumenJSON struct {
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
	Estado string `json:"estado"`
}

type misObrasJSON struct {
	Obras []obraResumenJSON `json:"obras"`
}

type ultimaLiquidacionJSON struct {
	Periodo string `json:"periodo"`
	Neto    string `json:"neto"`
	Obras   int    `json:"obras"`
}

type conteoJSON struct {
	Total int `json:"total"`
}

type ultimaCorridaJSON struct {
	Periodo string `json:"periodo"`
	Etapa   string `json:"etapa"`
	Estado  string `json:"estado"`
}

// conTablero responde 503 si el binario no cableo el caso de uso, igual que [API.conIngesta].
func (a *API) conTablero(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.tablero == nil {
			escribirError(w, http.StatusServiceUnavailable,
				"el tablero no esta configurado en esta instalacion")
			return
		}
		h(w, r)
	}
}

func (a *API) tableroMisObras(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	obras, err := a.tablero.MisObras(r.Context(), actor)
	if err != nil {
		escribirFallo(w, err)
		return
	}
	cuerpo := misObrasJSON{Obras: make([]obraResumenJSON, 0, len(obras))}
	for _, o := range obras {
		cuerpo.Obras = append(cuerpo.Obras, obraResumenJSON(o))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

func (a *API) tableroUltimaLiquidacion(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	l, err := a.tablero.UltimaLiquidacion(r.Context(), actor)
	if err != nil {
		escribirFallo(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, ultimaLiquidacionJSON{Periodo: l.Periodo, Neto: dinero(l.Neto), Obras: l.Obras})
}

func escribirConteo(w http.ResponseWriter, n int, err error) {
	if err != nil {
		escribirFallo(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, conteoJSON{Total: n})
}

func (a *API) tableroCargasPendientes(w http.ResponseWriter, r *http.Request) {
	n, err := a.tablero.CargasPendientes(r.Context())
	escribirConteo(w, n, err)
}

func (a *API) tableroObrasEnReserva(w http.ResponseWriter, r *http.Request) {
	n, err := a.tablero.ObrasEnReserva(r.Context())
	escribirConteo(w, n, err)
}

func (a *API) tableroONI(w http.ResponseWriter, r *http.Request) {
	n, err := a.tablero.ONIPendientes(r.Context())
	escribirConteo(w, n, err)
}

func (a *API) tableroUltimaCorrida(w http.ResponseWriter, r *http.Request) {
	c, err := a.tablero.UltimaCorrida(r.Context())
	if err != nil {
		escribirFallo(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, ultimaCorridaJSON(c))
}
