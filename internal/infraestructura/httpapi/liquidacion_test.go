package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
)

type liquidacionesFalsas struct {
	vistas      []aplicacion.OrdenVista
	errListar   error
	errTitular  error
	actorListar aplicacion.Usuario
	actorMia    aplicacion.Usuario
}

func (l *liquidacionesFalsas) Listar(_ context.Context, actor aplicacion.Usuario) ([]aplicacion.OrdenVista, error) {
	l.actorListar = actor
	return l.vistas, l.errListar
}

func (l *liquidacionesFalsas) DeTitular(_ context.Context, actor aplicacion.Usuario) ([]aplicacion.OrdenVista, error) {
	l.actorMia = actor
	return l.vistas, l.errTitular
}

func servidorConLiq(t *testing.T, auth Autenticacion, liq ConsultaLiquidaciones) http.Handler {
	t.Helper()
	return Nueva(nil, auth, liq, Opciones{}).Router()
}

func ordenVistaPrueba() aplicacion.OrdenVista {
	return aplicacion.OrdenVista{
		Orden: liquidacion.OrdenDePago{
			ID:        "liq-prc-1-tit-ana",
			ProcesoID: "prc-1",
			TitularID: "tit-ana",
			Periodo:   "2026",
			Bruto:     decimal.RequireFromString("1000.00"),
			Deducciones: []liquidacion.Deduccion{
				{Concepto: liquidacion.ConceptoAdministracion, Monto: decimal.RequireFromString("200.00")},
				{Concepto: liquidacion.ConceptoSocial, Monto: decimal.RequireFromString("100.00")},
			},
			Neto:       decimal.RequireFromString("700.00"),
			Estado:     liquidacion.EstadoAceptadaPorSilencio,
			EnviadaDia: "2026-01-01",
		},
		Pagable: true,
	}
}

func TestGetLiquidacionesAdmin(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-admin", Rol: aplicacion.RolAdministrador,
	}}
	liq := &liquidacionesFalsas{vistas: []aplicacion.OrdenVista{ordenVistaPrueba()}}
	h := servidorConLiq(t, auth, liq)

	rec := pedir(t, h, http.MethodGet, "/liquidaciones", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	if liq.actorListar.Rol != aplicacion.RolAdministrador {
		t.Fatalf("el caso de uso no recibio al admin: %+v", liq.actorListar)
	}

	var cuerpo listadoJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cuerpo.Liquidaciones) != 1 {
		t.Fatalf("len = %d", len(cuerpo.Liquidaciones))
	}
	o := cuerpo.Liquidaciones[0]
	if o.Bruto != "1000.00" || o.Neto != "700.00" {
		t.Fatalf("bruto/neto = %s/%s", o.Bruto, o.Neto)
	}
	if len(o.Deducciones) != 2 {
		t.Fatalf("deducciones = %d, tienen que ir itemizadas", len(o.Deducciones))
	}
	if o.Deducciones[0].Concepto != liquidacion.ConceptoAdministracion || o.Deducciones[0].Monto != "200.00" {
		t.Fatalf("primera deduccion = %+v", o.Deducciones[0])
	}
	if o.Estado != string(liquidacion.EstadoAceptadaPorSilencio) {
		t.Fatalf("estado = %q", o.Estado)
	}
	if !o.Pagable {
		t.Fatal("pagable tiene que viajar en el JSON")
	}
	if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestGetMisLiquidacionesTitular(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-titular", Rol: aplicacion.RolTitular, TitularID: "tit-ana",
	}}
	liq := &liquidacionesFalsas{vistas: []aplicacion.OrdenVista{ordenVistaPrueba()}}
	h := servidorConLiq(t, auth, liq)

	rec := pedir(t, h, http.MethodGet, "/mis-liquidaciones", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
	if liq.actorMia.TitularID != "tit-ana" {
		t.Fatalf("el caso de uso no recibio al titular de la sesion: %+v", liq.actorMia)
	}
}

func TestGetLiquidacionesSinTokenEs401(t *testing.T) {
	h := servidorConLiq(t, &autenticacionFalsa{}, &liquidacionesFalsas{})
	rec := pedir(t, h, http.MethodGet, "/liquidaciones", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestGetLiquidacionesTitularEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-titular", Rol: aplicacion.RolTitular, TitularID: "tit-ana",
	}}
	liq := &liquidacionesFalsas{errListar: aplicacion.ErrNoAutorizado}
	h := servidorConLiq(t, auth, liq)

	rec := pedir(t, h, http.MethodGet, "/liquidaciones", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, cuerpo = %s", rec.Code, rec.Body.String())
	}
}

func TestGetMisLiquidacionesAdminEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-admin", Rol: aplicacion.RolAdministrador,
	}}
	liq := &liquidacionesFalsas{errTitular: aplicacion.ErrNoAutorizado}
	h := servidorConLiq(t, auth, liq)

	rec := pedir(t, h, http.MethodGet, "/mis-liquidaciones", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestGetLiquidacionesListaVaciaEsArray(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{Rol: aplicacion.RolAuditor}}
	liq := &liquidacionesFalsas{vistas: []aplicacion.OrdenVista{}}
	h := servidorConLiq(t, auth, liq)

	rec := pedir(t, h, http.MethodGet, "/liquidaciones", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	arr, ok := cuerpo["liquidaciones"].([]any)
	if !ok {
		t.Fatalf("liquidaciones no es array: %#v", cuerpo["liquidaciones"])
	}
	if arr == nil {
		t.Fatal("liquidaciones null; tiene que ser []")
	}
}

func TestGetLiquidacionesFalloDeInfraestructuraEs500(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{Rol: aplicacion.RolAuditor}}
	liq := &liquidacionesFalsas{errListar: errors.New("connection refused")}
	h := servidorConLiq(t, auth, liq)

	rec := pedir(t, h, http.MethodGet, "/liquidaciones", "", "tok")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("codigo = %d", rec.Code)
	}
}
