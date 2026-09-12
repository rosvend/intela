package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// La tabla que pide el issue: (rol, ruta) -> codigo, a traves del chi
// router, con el doble de sesiones. Los 204 son la superficie vacia; lo
// que se comprueba es el middleware, no el payload.
func TestMatrizRolRuta(t *testing.T) {
	exito := map[string]int{
		"/admin/pipeline":                       http.StatusNoContent,
		"/auditoria/asientos":                   http.StatusNoContent,
		"/mis-liquidaciones":                    http.StatusOK,
		"/mis-liquidaciones/export?formato=pdf": http.StatusOK,
	}
	permitido := map[string][]aplicacion.Rol{
		"/admin/pipeline":                       {aplicacion.RolAdministrador},
		"/auditoria/asientos":                   {aplicacion.RolAuditor, aplicacion.RolAdministrador},
		"/mis-liquidaciones":                    {aplicacion.RolTitular},
		"/mis-liquidaciones/export?formato=pdf": {aplicacion.RolTitular},
	}
	roles := []aplicacion.Rol{
		aplicacion.RolAdministrador,
		aplicacion.RolDistribucion,
		aplicacion.RolContabilidad,
		aplicacion.RolAuditor,
		aplicacion.RolTitular,
	}

	for _, rol := range roles {
		auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol, TitularID: "tit-1"}}
		h := servidor(t, auth)
		for ruta, codigoOK := range exito {
			codigo := http.StatusForbidden
			for _, p := range permitido[ruta] {
				if p == rol {
					codigo = codigoOK
					break
				}
			}
			t.Run(string(rol)+" "+ruta, func(t *testing.T) {
				rec := pedir(t, h, http.MethodGet, ruta, "", "tok")
				if rec.Code != codigo {
					t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, codigo, rec.Body)
				}
			})
		}
	}
}

// Sin sesion el corte es conSesion (401), no requiereRol (403). Un 403
// a quien no presento token le diria que la ruta existe y que el rol es
// el problema.
func TestRutaConRolSinSesionEs401(t *testing.T) {
	h := servidor(t, &autenticacionFalsa{})

	for _, ruta := range []string{"/admin/pipeline", "/auditoria/asientos", "/mis-liquidaciones"} {
		t.Run(ruta, func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, ruta, "", "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("codigo = %d, se esperaba 401 (conSesion), no 403", rec.Code)
			}
			if a := rec.Header().Get("WWW-Authenticate"); !strings.Contains(a, "Bearer") {
				t.Fatalf("WWW-Authenticate = %q", a)
			}
		})
	}
}

func TestRequiereRolDevuelve403JSON(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolTitular}}

	rec := pedir(t, servidor(t, auth), http.MethodGet, "/admin/pipeline", "", "tok")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}
	cuerpo := decodificar(t, rec)
	if cuerpo["error"] != aplicacion.ErrNoAutorizado.Error() {
		t.Fatalf("error = %v, se esperaba %q", cuerpo["error"], aplicacion.ErrNoAutorizado)
	}
}

func TestRequiereRolPasaConElRolPermitido(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}

	rec := pedir(t, servidor(t, auth), http.MethodGet, "/admin/pipeline", "", "tok")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("codigo = %d, se esperaba 204. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// Defensa en profundidad: montado sin conSesion, no se deja pasar el
// Usuario cero. 401, no 403 —no hay sesion que rechazar por rol.
func TestRequiereRolSinUsuarioEnContextoEs401(t *testing.T) {
	h := requiereRol(aplicacion.RolAdministrador)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("no tiene que llegar al handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/pipeline", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}
