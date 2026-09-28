package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

type ingresosFalsos struct {
	filas  []aplicacion.Ingreso
	err    error
	actor  aplicacion.Usuario
	filtro aplicacion.FiltroIngresos
}

func (i *ingresosFalsos) MisIngresos(_ context.Context, actor aplicacion.Usuario, f aplicacion.FiltroIngresos) ([]aplicacion.Ingreso, error) {
	i.actor = actor
	i.filtro = f
	return i.filas, i.err
}

func filaAna() aplicacion.Ingreso {
	return aplicacion.Ingreso{
		Ref:     aplicacion.FormarRef("proc-2026-01", "obra-completa", "tit-ana"),
		ObraID:  "obra-completa",
		Obra:    "La Casa de las Dos Palmas",
		Fuente:  "caracol",
		Periodo: "2026-01",
		Neto:    decimal.RequireFromString("3600.00"),
	}
}

func TestMisIngresosDevuelveSoloLoDelTitular(t *testing.T) {
	ing := &ingresosFalsos{filas: []aplicacion.Ingreso{filaAna()}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, ing), http.MethodGet, "/mis-ingresos", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ing.actor.TitularID != "tit-ana" {
		t.Fatalf("el caso de uso recibio TitularID %q", ing.actor.TitularID)
	}
	var cuerpo listaIngresosJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cuerpo.Ingresos) != 1 || cuerpo.Ingresos[0].ObraID != "obra-completa" {
		t.Fatalf("ingresos = %+v", cuerpo.Ingresos)
	}
	if cuerpo.Ingresos[0].Neto != "3600.00" {
		t.Fatalf("neto = %q", cuerpo.Ingresos[0].Neto)
	}
	if strings.Contains(rec.Body.String(), `"bruto"`) {
		t.Fatal("el bruto no puede ir en el listado (OE-6)")
	}
}

func TestMisIngresosPasaLosFiltros(t *testing.T) {
	ing := &ingresosFalsos{filas: []aplicacion.Ingreso{}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, ing), http.MethodGet,
		"/mis-ingresos?obra=obra-completa&fuente=caracol&periodo=2026-01", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ing.filtro.ObraID != "obra-completa" || ing.filtro.Fuente != "caracol" || ing.filtro.Periodo != "2026-01" {
		t.Fatalf("filtro = %+v", ing.filtro)
	}
}

func TestMisIngresosIgnoraTitularIDEnLaQuery(t *testing.T) {
	ing := &ingresosFalsos{filas: []aplicacion.Ingreso{filaAna()}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, ing), http.MethodGet,
		"/mis-ingresos?titular_id=tit-beto", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if ing.actor.TitularID != "tit-ana" {
		t.Fatalf("se uso un titular de la query: %q", ing.actor.TitularID)
	}
}

func TestMisIngresosOtroRolEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	rec := pedir(t, servidorCon(t, auth, &ingresosFalsos{}), http.MethodGet, "/mis-ingresos", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
}

func TestMisIngresosSinCasoDeUsoEs503(t *testing.T) {
	auth := &autenticacionFalsa{usuario: titularAna()}
	h := Nueva(Casos{Auth: auth}, Opciones{}).Router()

	rec := pedir(t, h, http.MethodGet, "/mis-ingresos", "", "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, se esperaba 503. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestExplicarSinCasoDeUsoEs503(t *testing.T) {
	auth := &autenticacionFalsa{usuario: titularAna()}
	h := Nueva(Casos{Auth: auth}, Opciones{}).Router()

	rec := pedir(t, h, http.MethodGet, "/explicar/proc-1:obra-1:tit-ana", "", "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, se esperaba 503. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestMisIngresosSinSesionEs401(t *testing.T) {
	rec := pedir(t, servidorCon(t, &autenticacionFalsa{}, &ingresosFalsos{}), http.MethodGet, "/mis-ingresos", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}
