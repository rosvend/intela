package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

type tableroFalso struct {
	obras       []aplicacion.ObraResumen
	liquidacion aplicacion.ResumenLiquidacion
	corrida     aplicacion.CorridaResumen
	conteo      int
	err         error
	actor       aplicacion.Usuario
}

func (f *tableroFalso) MisObras(_ context.Context, actor aplicacion.Usuario) ([]aplicacion.ObraResumen, error) {
	f.actor = actor
	return f.obras, f.err
}

func (f *tableroFalso) UltimaLiquidacion(_ context.Context, actor aplicacion.Usuario) (aplicacion.ResumenLiquidacion, error) {
	f.actor = actor
	return f.liquidacion, f.err
}

func (f *tableroFalso) CargasPendientes(context.Context) (int, error) { return f.conteo, f.err }
func (f *tableroFalso) ObrasEnReserva(context.Context) (int, error)   { return f.conteo, f.err }
func (f *tableroFalso) ONIPendientes(context.Context) (int, error)    { return f.conteo, f.err }
func (f *tableroFalso) UltimaCorrida(context.Context) (aplicacion.CorridaResumen, error) {
	return f.corrida, f.err
}

func servidorConTablero(t *testing.T, usuario aplicacion.Usuario, tab Tablero) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: usuario}
	return Nueva(Casos{Auth: auth, Tablero: tab}, Opciones{}).Router()
}

var rutasTableroStaff = []string{
	"/tablero/cargas-pendientes",
	"/tablero/obras-en-reserva",
	"/tablero/oni",
	"/tablero/ultima-corrida",
}

var rutasTableroTitular = []string{"/tablero/mis-obras", "/tablero/ultima-liquidacion"}

func TestTableroRolesPorRuta(t *testing.T) {
	staff := []aplicacion.Rol{
		aplicacion.RolAdministrador, aplicacion.RolDistribucion,
		aplicacion.RolContabilidad, aplicacion.RolAuditor,
	}
	tab := &tableroFalso{conteo: 1, corrida: aplicacion.CorridaResumen{Periodo: "2026-01"}}
	for _, ruta := range rutasTableroStaff {
		rec := pedir(t, servidorConTablero(t, titularAna(), tab), http.MethodGet, ruta, "", "tok")
		if rec.Code != http.StatusForbidden {
			t.Errorf("titular en %s: codigo = %d, se esperaba 403", ruta, rec.Code)
		}
		for _, rol := range staff {
			u := aplicacion.Usuario{ID: "usr-" + string(rol), Rol: rol}
			rec := pedir(t, servidorConTablero(t, u, tab), http.MethodGet, ruta, "", "tok")
			if rec.Code != http.StatusOK {
				t.Errorf("%s en %s: codigo = %d, se esperaba 200", rol, ruta, rec.Code)
			}
		}
	}
	for _, ruta := range rutasTableroTitular {
		for _, rol := range staff {
			u := aplicacion.Usuario{ID: "usr-" + string(rol), Rol: rol}
			rec := pedir(t, servidorConTablero(t, u, tab), http.MethodGet, ruta, "", "tok")
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s en %s: codigo = %d, se esperaba 403", rol, ruta, rec.Code)
			}
		}
	}
}

func TestTableroSinSesionEs401(t *testing.T) {
	for _, ruta := range append(append([]string{}, rutasTableroStaff...), rutasTableroTitular...) {
		rec := pedir(t, servidorConTablero(t, aplicacion.Usuario{}, &tableroFalso{}), http.MethodGet, ruta, "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: codigo = %d, se esperaba 401", ruta, rec.Code)
		}
	}
}

func TestTableroSinCasoDeUsoEs503(t *testing.T) {
	admin := aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador}
	rec := pedir(t, servidorConTablero(t, admin, nil), http.MethodGet, "/tablero/oni", "", "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, se esperaba 503", rec.Code)
	}
}

func TestTableroMisObrasUsaElTitularDeLaSesion(t *testing.T) {
	tab := &tableroFalso{obras: []aplicacion.ObraResumen{{ID: "obra-1", Titulo: "La Casa", Estado: "completa"}}}

	rec := pedir(t, servidorConTablero(t, titularAna(), tab), http.MethodGet,
		"/tablero/mis-obras?titular_id=tit-beto", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if tab.actor.TitularID != "tit-ana" {
		t.Fatalf("se uso el titular %q, no el de la sesion", tab.actor.TitularID)
	}
	var cuerpo struct {
		Obras []struct{ ID, Titulo, Estado string } `json:"obras"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cuerpo.Obras) != 1 || cuerpo.Obras[0].ID != "obra-1" || cuerpo.Obras[0].Estado != "completa" {
		t.Fatalf("obras = %+v", cuerpo.Obras)
	}
}

func TestTableroMisObrasVaciaEsArreglo(t *testing.T) {
	rec := pedir(t, servidorConTablero(t, titularAna(), &tableroFalso{}), http.MethodGet, "/tablero/mis-obras", "", "tok")
	if rec.Body.String() != "{\"obras\":[]}\n" {
		t.Fatalf("cuerpo = %q", rec.Body.String())
	}
}

func TestTableroUltimaLiquidacion(t *testing.T) {
	casos := []struct {
		nombre string
		tab    *tableroFalso
		codigo int
		cuerpo string
	}{
		{"neto con dos decimales", &tableroFalso{liquidacion: aplicacion.ResumenLiquidacion{
			Periodo: "2026-02", Neto: decimal.RequireFromString("780"), Obras: 2,
		}}, http.StatusOK, "{\"periodo\":\"2026-02\",\"neto\":\"780.00\",\"obras\":2}\n"},
		{"sin lineas es 404", &tableroFalso{err: aplicacion.ErrNoEncontrado}, http.StatusNotFound, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := pedir(t, servidorConTablero(t, titularAna(), c.tab), http.MethodGet,
				"/tablero/ultima-liquidacion?titular_id=tit-beto", "", "tok")
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d", rec.Code, c.codigo)
			}
			if c.tab.actor.TitularID != "tit-ana" {
				t.Fatalf("titular = %q", c.tab.actor.TitularID)
			}
			if c.cuerpo != "" && rec.Body.String() != c.cuerpo {
				t.Fatalf("cuerpo = %q", rec.Body.String())
			}
		})
	}
}

func TestTableroConteosYCorrida(t *testing.T) {
	admin := aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador}
	tab := &tableroFalso{conteo: 4, corrida: aplicacion.CorridaResumen{Periodo: "2026-01", Etapa: "verificacion", Estado: "en_firma"}}
	casos := map[string]string{
		"/tablero/cargas-pendientes": "{\"total\":4}\n",
		"/tablero/obras-en-reserva":  "{\"total\":4}\n",
		"/tablero/oni":               "{\"total\":4}\n",
		"/tablero/ultima-corrida":    "{\"periodo\":\"2026-01\",\"etapa\":\"verificacion\",\"estado\":\"en_firma\"}\n",
	}
	for ruta, quiere := range casos {
		rec := pedir(t, servidorConTablero(t, admin, tab), http.MethodGet, ruta, "", "tok")
		if rec.Code != http.StatusOK || rec.Body.String() != quiere {
			t.Errorf("%s: codigo = %d, cuerpo = %q", ruta, rec.Code, rec.Body.String())
		}
	}
}

func TestTableroUltimaCorridaSinCorridasEs404(t *testing.T) {
	admin := aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador}
	rec := pedir(t, servidorConTablero(t, admin, &tableroFalso{err: aplicacion.ErrNoEncontrado}),
		http.MethodGet, "/tablero/ultima-corrida", "", "tok")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404", rec.Code)
	}
}
