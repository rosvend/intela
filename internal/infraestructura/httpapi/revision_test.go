package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

type colaFalsa struct {
	items []aplicacion.ItemRevision
	err   error
}

func (c *colaFalsa) ListarRevision(context.Context) ([]aplicacion.ItemRevision, error) {
	return c.items, c.err
}

func servidorConCola(t *testing.T, cola ColaRevision) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	return Nueva(nil, auth, nil, cola, Opciones{}).Router()
}

func TestListarColaRevisionDevuelveElEsquemaCompartido(t *testing.T) {
	cola := &colaFalsa{items: []aplicacion.ItemRevision{
		{
			ID: "tv-fecha", Tipo: aplicacion.TipoRevisionNormalizacion,
			Codigo: "fecha_inparseable", Motivo: "fecha_inparseable: fecha ayer",
			Fuente: "caracol", Titulo: "Fecha rota", ReporteID: "rep-1",
		},
	}}

	rec := pedir(t, servidorConCola(t, cola), http.MethodGet, "/admin/cola-revision", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}

	var items []itemRevisionJSON
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	got := items[0]
	if got.ID != "tv-fecha" || got.Tipo != "normalizacion" || got.Codigo != "fecha_inparseable" {
		t.Fatalf("item = %+v", got)
	}
	if got.ReporteID != "rep-1" || got.Titulo != "Fecha rota" {
		t.Fatalf("identidad = %+v", got)
	}
}

func TestListarColaRevisionVaciaEsListaVacia(t *testing.T) {
	rec := pedir(t, servidorConCola(t, &colaFalsa{}), http.MethodGet, "/admin/cola-revision", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	cuerpo := rec.Body.String()
	if cuerpo != "[]\n" && cuerpo != "[]" {
		t.Fatalf("cuerpo = %q, se esperaba []", cuerpo)
	}
}

func TestListarColaRevisionExigeAdministrador(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolTitular}}
	h := Nueva(nil, auth, nil, &colaFalsa{}, Opciones{}).Router()

	rec := pedir(t, h, http.MethodGet, "/admin/cola-revision", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
}

func TestListarColaRevisionSinSesionEs401(t *testing.T) {
	h := Nueva(nil, &autenticacionFalsa{}, nil, &colaFalsa{}, Opciones{}).Router()
	rec := pedir(t, h, http.MethodGet, "/admin/cola-revision", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}
