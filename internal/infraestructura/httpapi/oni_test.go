package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/oni"
)

type listadoONIFalso struct {
	pub     aplicacion.PublicacionONI
	err     error
	periodo string
}

func (l *listadoONIFalso) Ejecutar(_ context.Context, periodo string) (aplicacion.PublicacionONI, error) {
	l.periodo = periodo
	return l.pub, l.err
}

type publicarONIFalso struct {
	pub     aplicacion.PublicacionONI
	err     error
	periodo string
	actor   string
}

func (p *publicarONIFalso) Ejecutar(_ context.Context, periodo, actorID string) (aplicacion.PublicacionONI, error) {
	p.periodo = periodo
	p.actor = actorID
	return p.pub, p.err
}

func servidorONI(t *testing.T, auth Autenticacion, lectura LecturaONI, escritura EscrituraONI) http.Handler {
	t.Helper()
	return Nueva(nil, auth, Casos{ListadoONI: lectura, PublicarONI: escritura}, Opciones{}).Router()
}

func publicacionEjemplo() aplicacion.PublicacionONI {
	return aplicacion.PublicacionONI{
		ID:                   "pub-1",
		Periodo:              "2026-01",
		FechaProceso:         time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		DireccionFisica:      "Calle 74 #7-35, Bogota D.C.",
		DireccionElectronica: "oni@redescritores.com",
		Obras: []oni.ProyeccionPublica{
			{ID: "uso-1", Titulo: "Serie Desconocida", Fuente: "caracol", IDsFuente: "ID-99", Modalidad: "tv", Periodo: "2026-01"},
		},
	}
}

func TestListadoONIPublicoNoPideSesion(t *testing.T) {
	lectura := &listadoONIFalso{pub: publicacionEjemplo()}
	h := servidorONI(t, &autenticacionFalsa{}, lectura, nil)

	rec := pedir(t, h, http.MethodGet, "/publico/oni", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("una ruta publica no envia WWW-Authenticate: %q", rec.Header().Get("WWW-Authenticate"))
	}

	cuerpo := decodificar(t, rec)
	if cuerpo["periodo"] != "2026-01" {
		t.Fatalf("periodo = %v", cuerpo["periodo"])
	}
	if cuerpo["direccion_fisica"] == nil || cuerpo["direccion_electronica"] == nil {
		t.Fatalf("faltan las direcciones: %v", cuerpo)
	}
	if cuerpo["fecha_proceso"] != "2026-08-31T12:00:00Z" {
		t.Fatalf("fecha_proceso = %v", cuerpo["fecha_proceso"])
	}
	if cuerpo["explicacion"] == nil || cuerpo["explicacion"] == "" {
		t.Fatal("RD 13.8.4.4 exige una explicacion del proceso")
	}

	obras, ok := cuerpo["obras"].([]any)
	if !ok || len(obras) != 1 {
		t.Fatalf("obras = %v", cuerpo["obras"])
	}
	obra, _ := obras[0].(map[string]any)
	if obra["titulo"] != "Serie Desconocida" {
		t.Fatalf("titulo = %v", obra["titulo"])
	}
	if obra["ids_fuente"] != "ID-99" {
		t.Fatalf("ids_fuente = %v", obra["ids_fuente"])
	}
}

func TestListadoONIPublicoNuncaIncluyeMontos(t *testing.T) {
	lectura := &listadoONIFalso{pub: publicacionEjemplo()}
	rec := pedir(t, servidorONI(t, &autenticacionFalsa{}, lectura, nil),
		http.MethodGet, "/publico/oni", "", "")

	var generico map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &generico); err != nil {
		t.Fatalf("json: %v", err)
	}
	permitidos := map[string]bool{
		"periodo": true, "fecha_proceso": true, "direccion_fisica": true,
		"direccion_electronica": true, "explicacion": true, "obras": true,
	}
	for k := range generico {
		if !permitidos[k] {
			t.Fatalf("campo no publicable %q", k)
		}
	}
	obras, _ := generico["obras"].([]any)
	if len(obras) == 0 {
		t.Fatal("se esperaba al menos una obra")
	}
	obra0, _ := obras[0].(map[string]any)
	permitidosObra := map[string]bool{
		"id": true, "titulo": true, "fuente": true, "ids_fuente": true, "modalidad": true,
	}
	for k := range obra0 {
		if !permitidosObra[k] {
			t.Fatalf("campo de obra no publicable %q", k)
		}
	}
}

func TestListadoONIPublicoFiltraPorPeriodo(t *testing.T) {
	lectura := &listadoONIFalso{pub: publicacionEjemplo()}
	rec := pedir(t, servidorONI(t, &autenticacionFalsa{}, lectura, nil),
		http.MethodGet, "/publico/oni?periodo=2026-01", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if lectura.periodo != "2026-01" {
		t.Fatalf("periodo pasado al caso de uso = %q", lectura.periodo)
	}
}

func TestListadoONIPublicoSinPublicarEs404(t *testing.T) {
	lectura := &listadoONIFalso{err: aplicacion.ErrNoEncontrado}
	rec := pedir(t, servidorONI(t, &autenticacionFalsa{}, lectura, nil),
		http.MethodGet, "/publico/oni", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404", rec.Code)
	}
}

func TestPublicarONIExigeSesionYRol(t *testing.T) {
	escritura := &publicarONIFalso{pub: publicacionEjemplo()}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolTitular}}
	h := servidorONI(t, auth, nil, escritura)

	t.Run("sin token es 401", func(t *testing.T) {
		rec := pedir(t, h, http.MethodPost, "/oni/publicaciones", `{"periodo":"2026-01"}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
		}
		if escritura.periodo != "" {
			t.Fatal("no se publica nada sin sesion")
		}
	})

	t.Run("titular es 403", func(t *testing.T) {
		rec := pedir(t, h, http.MethodPost, "/oni/publicaciones", `{"periodo":"2026-01"}`, "tok")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
		}
	})
}

func TestPublicarONIConDistribucionEs201(t *testing.T) {
	escritura := &publicarONIFalso{pub: publicacionEjemplo()}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-dist", Rol: aplicacion.RolDistribucion,
	}}
	h := servidorONI(t, auth, nil, escritura)

	rec := pedir(t, h, http.MethodPost, "/oni/publicaciones", `{"periodo":"2026-01"}`, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	if escritura.periodo != "2026-01" || escritura.actor != "usr-dist" {
		t.Fatalf("se paso periodo=%q actor=%q", escritura.periodo, escritura.actor)
	}
	if decodificar(t, rec)["periodo"] != "2026-01" {
		t.Fatalf("cuerpo: %s", rec.Body)
	}
}

func TestPublicarONIConflictoEs409(t *testing.T) {
	escritura := &publicarONIFalso{err: aplicacion.ErrYaPublicado}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	rec := pedir(t, servidorONI(t, auth, nil, escritura),
		http.MethodPost, "/oni/publicaciones", `{"periodo":"2026-01"}`, "tok")
	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409", rec.Code)
	}
}

func TestPublicarONIPeriodoInvalidoEs400(t *testing.T) {
	escritura := &publicarONIFalso{err: aplicacion.ErrPeriodoInvalido}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	rec := pedir(t, servidorONI(t, auth, nil, escritura),
		http.MethodPost, "/oni/publicaciones", `{"periodo":"enero"}`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400", rec.Code)
	}
}

func TestPublicarONICuerpoRotoEs400(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	rec := pedir(t, servidorONI(t, auth, nil, &publicarONIFalso{}),
		http.MethodPost, "/oni/publicaciones", `{`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400", rec.Code)
	}
}

func TestPublicarONIFalloDeInfraestructuraEs500(t *testing.T) {
	escritura := &publicarONIFalso{err: errors.New("connection refused")}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	rec := pedir(t, servidorONI(t, auth, nil, escritura),
		http.MethodPost, "/oni/publicaciones", `{"periodo":"2026-01"}`, "tok")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("codigo = %d, se esperaba 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatalf("el error interno se filtra: %s", rec.Body)
	}
}
