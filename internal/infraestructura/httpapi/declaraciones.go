package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Declaraciones es lo que la capa HTTP necesita del nucleo para servir el
// editor de splits de la #30, declarado aqui igual que [Catalogo].
type Declaraciones interface {
	GuardarSplits(ctx context.Context, obraID string, partes []repertorio.Parte, actorID string) (aplicacion.VersionDeclaracion, error)
	Historial(ctx context.Context, obraID string) ([]aplicacion.VersionDeclaracion, error)
}

// ---------------------------------------------------------------------------
// Formas de red

type parteJSON struct {
	TitularID  string          `json:"titular_id"`
	IPI        string          `json:"ipi"`
	Porcentaje decimal.Decimal `json:"porcentaje"`
}

// versionDeclaracionJSON es una version de la declaracion tal como la ve el
// front: sus partes, su estado derivado y su ventana de vigencia.
type versionDeclaracionJSON struct {
	Version      int         `json:"version"`
	VigenteDesde string      `json:"vigente_desde"`
	VigenteHasta *string     `json:"vigente_hasta"`
	Estado       string      `json:"estado"`
	Partes       []parteJSON `json:"partes"`
}

func aPartesDominio(ps []parteJSON) []repertorio.Parte {
	partes := make([]repertorio.Parte, 0, len(ps))
	for _, p := range ps {
		partes = append(partes, repertorio.Parte{
			TitularID: p.TitularID, IPI: p.IPI, Porcentaje: p.Porcentaje,
		})
	}
	return partes
}

func aVersionJSON(vd aplicacion.VersionDeclaracion) versionDeclaracionJSON {
	partes := make([]parteJSON, 0, len(vd.Declaracion.Partes))
	for _, p := range vd.Declaracion.Partes {
		partes = append(partes, parteJSON{TitularID: p.TitularID, IPI: p.IPI, Porcentaje: p.Porcentaje})
	}
	var vigenteHasta *string
	if vd.VigenteHasta != nil {
		s := vd.VigenteHasta.Format(time.RFC3339)
		vigenteHasta = &s
	}
	return versionDeclaracionJSON{
		Version:      vd.Version,
		VigenteDesde: vd.VigenteDesde.Format(time.RFC3339),
		VigenteHasta: vigenteHasta,
		Estado:       vd.Declaracion.Estado(),
		Partes:       partes,
	}
}

// ---------------------------------------------------------------------------
// Handlers

// guardarDeclaracion sirve tanto el alta como la edicion: son la misma
// operacion en el nucleo (aplicacion.Declaraciones.GuardarSplits decide por
// si sola si cierra una version anterior o abre la primera), y lo unico que
// cambia entre POST y PUT es el codigo de exito.
func (a *API) guardarDeclaracion(w http.ResponseWriter, r *http.Request, codigoExito int) {
	var cuerpo []parteJSON
	if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser un JSON con la lista de partes")
		return
	}

	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		// Inalcanzable detras de conSesion, igual que en sesionActual.
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	obraID := chi.URLParam(r, "id")
	vd, err := a.declaraciones.GuardarSplits(r.Context(), obraID, aPartesDominio(cuerpo), usuario.ID)
	switch {
	case err == nil:
	case errors.Is(err, repertorio.ErrDeclaracionInvalida):
		// 400 y no 422: los datos llegaron, no forman una declaracion valida,
		// y el mensaje del dominio dice cual falta -mismo criterio que
		// registrarObra con ErrObraInvalida-.
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrTitularInexistente):
		// Mismo criterio 400 que ErrDeclaracionInvalida: el titular_id viene del
		// cuerpo, no del path -por eso no es 404, que aqui esta reservado al
		// recurso de la URL (la obra)-. Mensaje fijo y no err.Error() porque
		// este centinela sube envuelto por el adaptador y por el caso de uso, y
		// esas frases son internas.
		escribirError(w, http.StatusBadRequest, "uno de los titulares indicados no existe")
		return
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "esa obra no esta en el catalogo")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al guardar una declaracion", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo guardar la declaracion")
		return
	}

	escribirJSON(w, codigoExito, aVersionJSON(vd))
}

func (a *API) declararObra(w http.ResponseWriter, r *http.Request) {
	a.guardarDeclaracion(w, r, http.StatusCreated)
}

func (a *API) editarDeclaracion(w http.ResponseWriter, r *http.Request) {
	a.guardarDeclaracion(w, r, http.StatusOK)
}

// historialDeclaracion sirve todas las versiones de la declaracion de una
// obra. Una obra sin ninguna declaracion aun devuelve una lista vacia, no un
// 404: mismo criterio que buscarObras.
func (a *API) historialDeclaracion(w http.ResponseWriter, r *http.Request) {
	obraID := chi.URLParam(r, "id")
	historial, err := a.declaraciones.Historial(r.Context(), obraID)
	if err != nil {
		a.log.ErrorContext(r.Context(), "fallo al leer el historial de una declaracion", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar el historial")
		return
	}

	cuerpo := make([]versionDeclaracionJSON, 0, len(historial))
	for _, vd := range historial {
		cuerpo = append(cuerpo, aVersionJSON(vd))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}
