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
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// Admision es lo que la capa HTTP necesita del alta, declarado aqui.
type Admision interface {
	Solicitar(ctx context.Context, in aplicacion.SolicitudAfiliacion) (aplicacion.AfiliacionVista, error)
	CompletarIPI(ctx context.Context, id, ipi string) (aplicacion.AfiliacionVista, error)
	Aprobar(ctx context.Context, actor aplicacion.Usuario, id string) (aplicacion.AfiliacionVista, error)
	Rechazar(ctx context.Context, actor aplicacion.Usuario, id string) (aplicacion.AfiliacionVista, error)
}

const (
	// 3 × 5 MiB de documentos (R-12 + R-28) mas el resto del formulario.
	maxCuerpoAfiliacion  = 18 << 20
	maxArchivoAfiliacion = 5 << 20
)

type afiliacionJSON struct {
	ID                 string `json:"id"`
	Nombre             string `json:"nombre"`
	Email              string `json:"email"`
	DocumentoIdentidad string `json:"documento_identidad"`
	IPI                string `json:"ipi"`
	Subtipo            string `json:"subtipo"`
	Estado             string `json:"estado"`
	ElegibleAnticipo   bool   `json:"elegible_anticipo"`
	TieneRUT           bool   `json:"tiene_rut"`
	TieneCertBancaria  bool   `json:"tiene_certificacion_bancaria"`
	TieneRenuncia      bool   `json:"tiene_renuncia"`
	TitularID          string `json:"titular_id"`
}

func aAfiliacionJSON(v aplicacion.AfiliacionVista) afiliacionJSON {
	return afiliacionJSON{
		ID:                 v.ID,
		Nombre:             v.Nombre,
		Email:              v.Email,
		DocumentoIdentidad: v.DocumentoIdentidad,
		IPI:                v.IPI,
		Subtipo:            v.Subtipo,
		Estado:             v.Estado,
		ElegibleAnticipo:   v.ElegibleAnticipo,
		TieneRUT:           v.TieneRUT,
		TieneCertBancaria:  v.TieneCertBancaria,
		TieneRenuncia:      v.TieneRenuncia,
		TitularID:          v.TitularID,
	}
}

func (a *API) solicitarAfiliacion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCuerpoAfiliacion)
	if err := r.ParseMultipartForm(maxCuerpoAfiliacion); err != nil {
		codigo, msg := errorCuerpoAfiliacion(err)
		escribirError(w, codigo, msg)
		return
	}

	rut, err := leerArchivo(r, "rut")
	if err != nil {
		escribirError(w, http.StatusBadRequest, "no se pudo leer el rut")
		return
	}
	banco, err := leerArchivo(r, "certificacion_bancaria")
	if err != nil {
		escribirError(w, http.StatusBadRequest, "no se pudo leer la certificacion bancaria")
		return
	}
	renuncia, err := leerArchivo(r, "renuncia")
	if err != nil {
		escribirError(w, http.StatusBadRequest, "no se pudo leer la renuncia")
		return
	}

	in := aplicacion.SolicitudAfiliacion{
		Nombre:             r.FormValue("nombre"),
		Email:              r.FormValue("email"),
		DocumentoIdentidad: r.FormValue("documento_identidad"),
		IPI:                r.FormValue("ipi"),
		Subtipo:            r.FormValue("subtipo"),
		Clave:              r.FormValue("clave"),
		PerteneceOtraSGC:   booleanoFormulario(r.FormValue("pertenece_otra_sgc")),
		RUT:                rut,
		CertBancaria:       banco,
		Renuncia:           renuncia,
	}

	vista, err := a.admision.Solicitar(r.Context(), in)
	if err != nil {
		if errors.Is(err, aplicacion.ErrConflicto) {
			// 409 sobre un alta publica enumeraba si un correo ya estaba
			// afiliado. El mensaje tampoco nombra el dato.
			escribirError(w, http.StatusBadRequest, "no se pudo registrar la solicitud")
			return
		}
		a.responderErrorAdmision(w, r, err, "solicitar afiliacion")
		return
	}
	escribirJSON(w, http.StatusCreated, aAfiliacionJSON(vista))
}

func (a *API) completarIPI(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		IPI string `json:"ipi"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&cuerpo); err != nil {
		escribirError(w, http.StatusBadRequest, "el cuerpo tiene que ser json con el ipi")
		return
	}

	id := chi.URLParam(r, "id")
	vista, err := a.admision.CompletarIPI(r.Context(), id, cuerpo.IPI)
	if err != nil {
		if errors.Is(err, aplicacion.ErrConflicto) {
			escribirError(w, http.StatusBadRequest, "no se pudo completar el ipi")
			return
		}
		a.responderErrorAdmision(w, r, err, "completar ipi")
		return
	}
	escribirJSON(w, http.StatusOK, aAfiliacionJSON(vista))
}

func (a *API) aprobarAfiliacion(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	id := chi.URLParam(r, "id")
	vista, err := a.admision.Aprobar(r.Context(), actor, id)
	if err != nil {
		a.responderErrorAdmision(w, r, err, "aprobar afiliacion")
		return
	}
	escribirJSON(w, http.StatusOK, aAfiliacionJSON(vista))
}

func (a *API) rechazarAfiliacion(w http.ResponseWriter, r *http.Request) {
	actor, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}

	id := chi.URLParam(r, "id")
	vista, err := a.admision.Rechazar(r.Context(), actor, id)
	if err != nil {
		a.responderErrorAdmision(w, r, err, "rechazar afiliacion")
		return
	}
	escribirJSON(w, http.StatusOK, aAfiliacionJSON(vista))
}

func (a *API) responderErrorAdmision(w http.ResponseWriter, r *http.Request, err error, que string) {
	switch {
	case errors.Is(err, afiliacion.ErrExclusividad),
		errors.Is(err, aplicacion.ErrConflicto),
		errors.Is(err, afiliacion.ErrEstadoInvalido):
		escribirError(w, http.StatusConflict, err.Error())
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, "no autorizado")
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "solicitud no encontrada")
	case errors.Is(err, afiliacion.ErrDocumentosPago),
		errors.Is(err, afiliacion.ErrNombreObligatorio),
		errors.Is(err, afiliacion.ErrEmailInvalido),
		errors.Is(err, afiliacion.ErrDocumentoObligatorio),
		errors.Is(err, afiliacion.ErrSubtipoInvalido),
		errors.Is(err, afiliacion.ErrIPIObligatorio),
		errors.Is(err, aplicacion.ErrDocumentoInvalido),
		errors.Is(err, aplicacion.ErrClaveInvalida):
		escribirError(w, http.StatusBadRequest, err.Error())
	default:
		a.log.ErrorContext(r.Context(), "fallo al "+que, slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo completar la operacion")
	}
}

func errorCuerpoAfiliacion(err error) (int, string) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return http.StatusRequestEntityTooLarge, "el cuerpo supera el tamano maximo permitido"
	}
	return http.StatusBadRequest, "el cuerpo tiene que ser multipart con los datos y los documentos"
}

func leerArchivo(r *http.Request, campo string) ([]byte, error) {
	f, _, err := r.FormFile(campo)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, maxArchivoAfiliacion+1))
}

func booleanoFormulario(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "on", "si", "sí":
		return true
	default:
		return false
	}
}
