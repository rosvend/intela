package httpapi

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

type admisionFalsa struct {
	vista     aplicacion.AfiliacionVista
	err       error
	recibida  aplicacion.SolicitudAfiliacion
	actor     aplicacion.Usuario
	idAprobar string
}

func (a *admisionFalsa) Solicitar(_ context.Context, in aplicacion.SolicitudAfiliacion) (aplicacion.AfiliacionVista, error) {
	a.recibida = in
	return a.vista, a.err
}

func (a *admisionFalsa) Aprobar(_ context.Context, actor aplicacion.Usuario, id string) (aplicacion.AfiliacionVista, error) {
	a.actor = actor
	a.idAprobar = id
	return a.vista, a.err
}

func servidorAdmision(t *testing.T, auth Autenticacion, adm Admision) http.Handler {
	t.Helper()
	return Nueva(nil, auth, adm, Opciones{}).Router()
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
