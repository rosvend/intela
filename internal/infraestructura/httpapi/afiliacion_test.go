package httpapi

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

type admisionFalsa struct {
	vista       aplicacion.AfiliacionVista
	err         error
	recibida    aplicacion.SolicitudAfiliacion
	actor       aplicacion.Usuario
	idAprobar   string
	idRechazar  string
	idIPI       string
	ipiRecibido string
}

func (a *admisionFalsa) Solicitar(_ context.Context, in aplicacion.SolicitudAfiliacion) (aplicacion.AfiliacionVista, error) {
	a.recibida = in
	return a.vista, a.err
}

func (a *admisionFalsa) CompletarIPI(_ context.Context, id, ipi string) (aplicacion.AfiliacionVista, error) {
	a.idIPI = id
	a.ipiRecibido = ipi
	return a.vista, a.err
}

func (a *admisionFalsa) Aprobar(_ context.Context, actor aplicacion.Usuario, id string) (aplicacion.AfiliacionVista, error) {
	a.actor = actor
	a.idAprobar = id
	return a.vista, a.err
}

func (a *admisionFalsa) Rechazar(_ context.Context, actor aplicacion.Usuario, id string) (aplicacion.AfiliacionVista, error) {
	a.actor = actor
	a.idRechazar = id
	return a.vista, a.err
}

func servidorAdmision(t *testing.T, auth Autenticacion, adm Admision) http.Handler {
	t.Helper()
	return Nueva(nil, auth, adm, nil, Opciones{}).Router()
}

func multipartSolicitud(t *testing.T, campos map[string]string, archivos map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var cuerpo bytes.Buffer
	w := multipart.NewWriter(&cuerpo)
	for k, v := range campos {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("campo %s: %v", k, err)
		}
	}
	for campo, datos := range archivos {
		parte, err := w.CreateFormFile(campo, campo+".pdf")
		if err != nil {
			t.Fatalf("archivo %s: %v", campo, err)
		}
		if _, err := parte.Write(datos); err != nil {
			t.Fatalf("escribir %s: %v", campo, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("cerrar multipart: %v", err)
	}
	return &cuerpo, w.FormDataContentType()
}

func TestSolicitarAfiliacionDevuelvePendiente(t *testing.T) {
	adm := &admisionFalsa{vista: aplicacion.AfiliacionVista{
		ID:                "afil-1",
		Nombre:            "Ana",
		Email:             "ana@redes.co",
		Estado:            "pendiente",
		Subtipo:           "socio",
		TieneRUT:          true,
		TieneCertBancaria: true,
	}}
	h := servidorAdmision(t, &autenticacionFalsa{}, adm)

	campos := map[string]string{
		"nombre":              "Ana Escritora",
		"email":               "ana@redes.co",
		"documento_identidad": "123",
		"subtipo":             "socio",
		"ipi":                 "IPI-1",
		"clave":               "secret12",
	}
	archivos := map[string][]byte{
		"rut":                    []byte("%PDF-1.4\n"),
		"certificacion_bancaria": []byte("%PDF-1.4\n"),
	}
	cuerpo, ctype := multipartSolicitud(t, campos, archivos)

	req := httptest.NewRequest(http.MethodPost, "/afiliaciones", cuerpo)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	got := decodificar(t, rec)
	if got["estado"] != "pendiente" {
		t.Fatalf("estado = %v", got["estado"])
	}
	if got["id"] != "afil-1" {
		t.Fatalf("id = %v", got["id"])
	}
	if !bytes.HasPrefix(adm.recibida.RUT, []byte("%PDF")) {
		t.Fatal("el RUT no llego al caso de uso")
	}
	if !bytes.HasPrefix(adm.recibida.CertBancaria, []byte("%PDF")) {
		t.Fatal("la certificacion bancaria no llego al caso de uso")
	}
	if adm.recibida.Clave != "secret12" {
		t.Fatalf("clave = %q", adm.recibida.Clave)
	}
}

func TestSolicitarAfiliacionConflictoExclusividadEs409(t *testing.T) {
	adm := &admisionFalsa{err: afiliacion.ErrExclusividad}
	h := servidorAdmision(t, &autenticacionFalsa{}, adm)

	campos := map[string]string{
		"nombre":              "Ana",
		"email":               "ana@redes.co",
		"documento_identidad": "123",
		"subtipo":             "socio",
		"pertenece_otra_sgc":  "true",
	}
	archivos := map[string][]byte{
		"rut":                    []byte("%PDF-1.4\n"),
		"certificacion_bancaria": []byte("%PDF-1.4\n"),
	}
	cuerpo, ctype := multipartSolicitud(t, campos, archivos)
	req := httptest.NewRequest(http.MethodPost, "/afiliaciones", cuerpo)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
	msg, _ := decodificar(t, rec)["error"].(string)
	if !strings.Contains(strings.ToLower(msg), "r-28") {
		t.Fatalf("el 409 tiene que explicar R-28: %q", msg)
	}
}

func TestSolicitarAfiliacionSinDocumentosEs400(t *testing.T) {
	adm := &admisionFalsa{err: afiliacion.ErrDocumentosPago}
	h := servidorAdmision(t, &autenticacionFalsa{}, adm)

	campos := map[string]string{
		"nombre":              "Ana",
		"email":               "ana@redes.co",
		"documento_identidad": "123",
		"subtipo":             "socio",
	}
	cuerpo, ctype := multipartSolicitud(t, campos, nil)
	req := httptest.NewRequest(http.MethodPost, "/afiliaciones", cuerpo)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestAprobarAfiliacionExigeSesion(t *testing.T) {
	h := servidorAdmision(t, &autenticacionFalsa{}, &admisionFalsa{})
	rec := pedir(t, h, http.MethodPost, "/afiliaciones/afil-1/aprobar", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}

func TestAprobarAfiliacionDevuelveAdmitida(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-admin", Rol: aplicacion.RolAdministrador,
	}}
	adm := &admisionFalsa{vista: aplicacion.AfiliacionVista{
		ID:               "afil-1",
		Estado:           "admitido",
		Subtipo:          "socio",
		ElegibleAnticipo: true,
		TitularID:        "tit-id-fijo",
	}}
	h := servidorAdmision(t, auth, adm)

	rec := pedir(t, h, http.MethodPost, "/afiliaciones/afil-1/aprobar", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	got := decodificar(t, rec)
	if got["estado"] != "admitido" {
		t.Fatalf("estado = %v", got["estado"])
	}
	if got["elegible_anticipo"] != true {
		t.Fatalf("elegible_anticipo = %v", got["elegible_anticipo"])
	}
	if adm.idAprobar != "afil-1" {
		t.Fatalf("id = %q", adm.idAprobar)
	}
	if adm.actor.Rol != aplicacion.RolAdministrador {
		t.Fatalf("actor.Rol = %q", adm.actor.Rol)
	}
}

func TestAprobarAfiliacionSinPermisoEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-ana", Rol: aplicacion.RolTitular,
	}}
	adm := &admisionFalsa{err: aplicacion.ErrNoAutorizado}
	h := servidorAdmision(t, auth, adm)

	rec := pedir(t, h, http.MethodPost, "/afiliaciones/afil-1/aprobar", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
}

func TestElAltaNoPideSesion(t *testing.T) {
	adm := &admisionFalsa{vista: aplicacion.AfiliacionVista{ID: "afil-1", Estado: "pendiente"}}
	h := servidorAdmision(t, &autenticacionFalsa{}, adm)
	cuerpo, ctype := multipartSolicitud(t, map[string]string{
		"nombre": "A", "email": "a@redes.co", "documento_identidad": "1", "subtipo": "socio",
	}, map[string][]byte{"rut": []byte("%PDF-1.4\n"), "certificacion_bancaria": []byte("%PDF-1.4\n")})
	req := httptest.NewRequest(http.MethodPost, "/afiliaciones", cuerpo)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("el alta sin token dio %d: %s", rec.Code, rec.Body)
	}
}

func TestSolicitarAfiliacionConflictoNoEnumeraCorreo(t *testing.T) {
	adm := &admisionFalsa{err: aplicacion.ErrConflicto}
	h := servidorAdmision(t, &autenticacionFalsa{}, adm)
	cuerpo, ctype := multipartSolicitud(t, map[string]string{
		"nombre": "A", "email": "a@redes.co", "documento_identidad": "1", "subtipo": "socio",
	}, map[string][]byte{"rut": []byte("%PDF-1.4\n"), "certificacion_bancaria": []byte("%PDF-1.4\n")})
	req := httptest.NewRequest(http.MethodPost, "/afiliaciones", cuerpo)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400 (no 409). Cuerpo: %s", rec.Code, rec.Body)
	}
	msg, _ := decodificar(t, rec)["error"].(string)
	if strings.Contains(strings.ToLower(msg), "existe") || strings.Contains(strings.ToLower(msg), "conflicto") {
		t.Fatalf("el mensaje no puede decir que el correo ya existe: %q", msg)
	}
}

func TestCompletarIPIPublico(t *testing.T) {
	adm := &admisionFalsa{vista: aplicacion.AfiliacionVista{
		ID: "afil-1", IPI: "IPI-42", Estado: "pendiente",
	}}
	h := servidorAdmision(t, &autenticacionFalsa{}, adm)

	req := httptest.NewRequest(http.MethodPatch, "/afiliaciones/afil-1/ipi", strings.NewReader(`{"ipi":"IPI-42"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if adm.idIPI != "afil-1" || adm.ipiRecibido != "IPI-42" {
		t.Fatalf("id=%q ipi=%q", adm.idIPI, adm.ipiRecibido)
	}
}

func TestRechazarAfiliacionExigeSesion(t *testing.T) {
	h := servidorAdmision(t, &autenticacionFalsa{}, &admisionFalsa{})
	rec := pedir(t, h, http.MethodPost, "/afiliaciones/afil-1/rechazar", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}

func TestRechazarAfiliacionDevuelveRechazada(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-admin", Rol: aplicacion.RolAdministrador,
	}}
	adm := &admisionFalsa{vista: aplicacion.AfiliacionVista{
		ID: "afil-1", Estado: "rechazado", Subtipo: "socio",
	}}
	h := servidorAdmision(t, auth, adm)

	rec := pedir(t, h, http.MethodPost, "/afiliaciones/afil-1/rechazar", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	got := decodificar(t, rec)
	if got["estado"] != "rechazado" {
		t.Fatalf("estado = %v", got["estado"])
	}
	if adm.idRechazar != "afil-1" {
		t.Fatalf("id = %q", adm.idRechazar)
	}
}

func TestErrorCuerpoAfiliacionDistingueTamanoDeMultipart(t *testing.T) {
	codigo, msg := errorCuerpoAfiliacion(&http.MaxBytesError{Limit: maxCuerpoAfiliacion})
	if codigo != http.StatusRequestEntityTooLarge {
		t.Fatalf("codigo = %d, se esperaba 413", codigo)
	}
	if !strings.Contains(strings.ToLower(msg), "tamano") && !strings.Contains(strings.ToLower(msg), "tamaño") {
		t.Fatalf("el 413 tiene que hablar de tamano: %q", msg)
	}

	codigo, msg = errorCuerpoAfiliacion(errors.New("no es multipart"))
	if codigo != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400", codigo)
	}
	if !strings.Contains(msg, "multipart") {
		t.Fatalf("el 400 tiene que pedir multipart: %q", msg)
	}
}
