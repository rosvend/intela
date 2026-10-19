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
	"github.com/rosvend/intela/internal/dominio/anticipos"
)

// Anticipos es lo que la capa HTTP necesita del modulo de anticipos a socios
// (R-30..R-33), declarado aqui igual que [Procesos] y [Admision].
type Anticipos interface {
	Solicitar(ctx context.Context, actor aplicacion.Usuario, in aplicacion.SolicitudAnticipo) (aplicacion.AnticipoRegistrado, error)
	Aprobar(ctx context.Context, actor aplicacion.Usuario, id, soporte string) (aplicacion.AnticipoRegistrado, error)
	Rechazar(ctx context.Context, actor aplicacion.Usuario, id, motivo string) (aplicacion.AnticipoRegistrado, error)
	Girar(ctx context.Context, actor aplicacion.Usuario, id, soporte string) (aplicacion.AnticipoRegistrado, error)
	Listar(ctx context.Context, actor aplicacion.Usuario, estado string) ([]aplicacion.AnticipoRegistrado, error)
}

var _ Anticipos = aplicacion.Anticipos{}

// ---------------------------------------------------------------------------
// Formas de red

// anticipoJSON es un anticipo tal como viaja por la red. Monto y saldo van
// como cadena con dos decimales por lo mismo que los de una orden de pago: un
// JSON number es IEEE-754 y no representa dinero (ADR 0010).
type anticipoJSON struct {
	ID            string     `json:"id"`
	TitularID     string     `json:"titular_id"`
	Monto         string     `json:"monto"`
	Saldo         string     `json:"saldo"`
	Estado        string     `json:"estado"`
	Justificacion string     `json:"justificacion"`
	Soporte       string     `json:"soporte,omitempty"`
	MotivoRechazo string     `json:"motivo_rechazo,omitempty"`
	Solicitado    time.Time  `json:"solicitado"`
	Resuelto      *time.Time `json:"resuelto,omitempty"`
	ResueltoPor   string     `json:"resuelto_por,omitempty"`
	Girado        *time.Time `json:"girado,omitempty"`
	GiradoPor     string     `json:"girado_por,omitempty"`
	SoporteGiro   string     `json:"soporte_giro,omitempty"`
}

type listadoAnticiposJSON struct {
	Anticipos []anticipoJSON `json:"anticipos"`
}

func aAnticipoJSON(r aplicacion.AnticipoRegistrado) anticipoJSON {
	a := r.Anticipo
	out := anticipoJSON{
		ID:            a.ID,
		TitularID:     a.TitularID,
		Monto:         a.Monto.StringFixed(2),
		Saldo:         a.Saldo.StringFixed(2),
		Estado:        string(a.Estado),
		Justificacion: a.Justificacion,
		Soporte:       a.Soporte,
		MotivoRechazo: a.MotivoRechazo,
		Solicitado:    r.Solicitado,
		ResueltoPor:   r.ResueltoPor,
		GiradoPor:     r.GiradoPor,
		SoporteGiro:   a.SoporteGiro,
	}
	if !r.Resuelto.IsZero() {
		resuelto := r.Resuelto
		out.Resuelto = &resuelto
	}
	if !r.Girado.IsZero() {
		girado := r.Girado
		out.Girado = &girado
	}
	return out
}

// solicitudAnticipoJSON es el cuerpo de POST /anticipos (RA 3.2.1). El titular
// no viaja: sale de la sesion (RA 3.1.f, la solicitud la hace el propio socio).
//
// Monto es string y no decimal.Decimal: el contrato lo declara cadena, y
// decimal.Decimal tambien acepta un JSON number, que ya llego por coma
// flotante (ADR 0010). Con string, un number no desserializa y es un 400; la
// cadena se convierte despues con [decimal.NewFromString], sin pasar por float.
type solicitudAnticipoJSON struct {
	Monto         string `json:"monto"`
	Justificacion string `json:"justificacion"`
}

type aprobacionAnticipoJSON struct {
	Soporte string `json:"soporte"`
}

// giroAnticipoJSON es el cuerpo de POST /anticipos/{id}/girar: la ubicacion
// del registro de la transferencia (RA 3.3).
type giroAnticipoJSON struct {
	Soporte string `json:"soporte"`
}

type rechazoAnticipoJSON struct {
	Motivo string `json:"motivo"`
}

const maxCuerpoAnticipo = 16 << 10

// ---------------------------------------------------------------------------
// Handlers

// solicitarAnticipo es POST /anticipos: el socio autenticado pide su anticipo.
func (a *API) solicitarAnticipo(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	var cuerpo solicitudAnticipoJSON
	if !leerCuerpoAnticipo(w, r, &cuerpo, "el cuerpo tiene que ser un JSON con monto (cadena) y justificacion") {
		return
	}
	monto, err := decimal.NewFromString(cuerpo.Monto)
	if err != nil {
		escribirError(w, http.StatusBadRequest, "el monto tiene que ser una cadena con un importe decimal")
		return
	}
	registrado, err := a.anticipos.Solicitar(r.Context(), actor, aplicacion.SolicitudAnticipo{
		Monto:         monto,
		Justificacion: cuerpo.Justificacion,
	})
	if err != nil {
		a.responderErrorAnticipo(w, r, err, "solicitar el anticipo")
		return
	}
	// Sin Location: no hay GET /anticipos/{id} al que apuntar. El anticipo
	// creado viaja entero en el cuerpo.
	escribirJSON(w, http.StatusCreated, aAnticipoJSON(registrado))
}

// listarAnticipos es GET /anticipos. El staff ve todos; un titular, los suyos.
// El recorte lo hace el caso de uso con el TitularID de la sesion (OE-6).
func (a *API) listarAnticipos(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	lista, err := a.anticipos.Listar(r.Context(), actor, r.URL.Query().Get("estado"))
	if err != nil {
		a.responderErrorAnticipo(w, r, err, "listar los anticipos")
		return
	}
	cuerpo := listadoAnticiposJSON{Anticipos: make([]anticipoJSON, 0, len(lista))}
	for _, ant := range lista {
		cuerpo.Anticipos = append(cuerpo.Anticipos, aAnticipoJSON(ant))
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

// aprobarAnticipo es POST /anticipos/{id}/aprobar: la decision del Consejo
// Directivo (RA 3.2.3) con la ubicacion de su soporte (RA 3.2.5).
func (a *API) aprobarAnticipo(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	var cuerpo aprobacionAnticipoJSON
	if !leerCuerpoAnticipo(w, r, &cuerpo, "el cuerpo tiene que ser un JSON con el soporte de la aprobacion") {
		return
	}
	resuelto, err := a.anticipos.Aprobar(r.Context(), actor, chi.URLParam(r, "id"), cuerpo.Soporte)
	if err != nil {
		a.responderErrorAnticipo(w, r, err, "aprobar el anticipo")
		return
	}
	escribirJSON(w, http.StatusOK, aAnticipoJSON(resuelto))
}

// rechazarAnticipo es POST /anticipos/{id}/rechazar: la negacion del Consejo,
// con su justificacion literal (RA 3.2.3).
func (a *API) rechazarAnticipo(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	var cuerpo rechazoAnticipoJSON
	if !leerCuerpoAnticipo(w, r, &cuerpo, "el cuerpo tiene que ser un JSON con el motivo del rechazo") {
		return
	}
	resuelto, err := a.anticipos.Rechazar(r.Context(), actor, chi.URLParam(r, "id"), cuerpo.Motivo)
	if err != nil {
		a.responderErrorAnticipo(w, r, err, "rechazar el anticipo")
		return
	}
	escribirJSON(w, http.StatusOK, aAnticipoJSON(resuelto))
}

// girarAnticipo es POST /anticipos/{id}/girar: contabilidad registra la
// transferencia al socio (RA 3.2.4). Es la segunda firma de la salida de
// dinero; desde aqui R-33 descuenta (ADR 0027).
func (a *API) girarAnticipo(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	var cuerpo giroAnticipoJSON
	if !leerCuerpoAnticipo(w, r, &cuerpo, "el cuerpo tiene que ser un JSON con el soporte de la transferencia") {
		return
	}
	girado, err := a.anticipos.Girar(r.Context(), actor, chi.URLParam(r, "id"), cuerpo.Soporte)
	if err != nil {
		a.responderErrorAnticipo(w, r, err, "girar el anticipo")
		return
	}
	escribirJSON(w, http.StatusOK, aAnticipoJSON(girado))
}

func leerCuerpoAnticipo(w http.ResponseWriter, r *http.Request, destino any, mensaje string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoAnticipo)
	if err := json.NewDecoder(r.Body).Decode(destino); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			escribirError(w, http.StatusRequestEntityTooLarge, "el cuerpo supera el tamano maximo permitido")
			return false
		}
		escribirError(w, http.StatusBadRequest, mensaje)
		return false
	}
	return true
}

// responderErrorAnticipo traduce los centinelas del modulo.
//
//   - 400: a la solicitud o a la decision le falta lo que el reglamento pide
//     que conste (RA 3.2.1, RA 3.2.3, RA 3.2.5).
//   - 409: el estado no lo permite -ya hay un anticipo vigente o con saldo
//     (R-32), o la solicitud ya se resolvio-. Esperar o resolver lo otro lo
//     arregla.
//   - 422: la regla lo prohibe para este socio hoy: no es socio (R-30), se
//     pasa del tope (R-31) o tiene un embargo (R-32). El mensaje lleva el
//     motivo, que es lo que el socio o el Consejo necesitan leer.
//   - 500: falta el SMMLV vigente (ADR 0004): sin el no hay tope de RA 3.1.h.
//     Es un fallo de la instalacion, igual que en /liquidaciones.
func (a *API) responderErrorAnticipo(w http.ResponseWriter, r *http.Request, err error, que string) {
	switch {
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, "no autorizado")
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "anticipo o titular no encontrado")
	case errors.Is(err, anticipos.ErrSolicitudInvalida),
		errors.Is(err, anticipos.ErrResolucionIncompleta):
		escribirError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, anticipos.ErrAnticipoVigente),
		errors.Is(err, anticipos.ErrSaldoPendiente),
		errors.Is(err, anticipos.ErrEstadoInvalido):
		escribirError(w, http.StatusConflict, err.Error())
	case errors.Is(err, anticipos.ErrNoEsSocio),
		errors.Is(err, anticipos.ErrSuperaTope),
		errors.Is(err, anticipos.ErrEmbargoReportado):
		escribirError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, aplicacion.ErrParametroAusente):
		a.log.ErrorContext(r.Context(), "parametro normativo ausente al "+que, slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "parametro normativo ausente")
	default:
		a.log.ErrorContext(r.Context(), "fallo al "+que, slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo completar la operacion")
	}
}

// conAnticipos responde 503 si el binario no cableo el modulo, por lo mismo
// que [API.conIngesta]: un handler sobre una interfaz nil es un 500 sin cuerpo.
func (a *API) conAnticipos(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.anticipos == nil {
			escribirError(w, http.StatusServiceUnavailable,
				"el modulo de anticipos no esta configurado en esta instalacion")
			return
		}
		h(w, r)
	}
}
