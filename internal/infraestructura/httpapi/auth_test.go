package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// autenticacionFalsa es un doble del caso de uso. La capa HTTP se prueba sin
// base de datos: lo que se comprueba aqui son codigos, cabeceras y la forma
// del JSON, no la logica de credenciales -esa ya tiene sus pruebas en
// internal/aplicacion-.
type autenticacionFalsa struct {
	sesion      aplicacion.Sesion
	errIniciar  error
	usuario     aplicacion.Usuario
	errResolver error
	errCerrar   error

	tokenRecibido string
	revocados     []string
}

func (a *autenticacionFalsa) IniciarSesion(_ context.Context, _, _ string) (aplicacion.Sesion, error) {
	return a.sesion, a.errIniciar
}

func (a *autenticacionFalsa) ResolverSesion(_ context.Context, token string) (aplicacion.Usuario, error) {
	a.tokenRecibido = token
	return a.usuario, a.errResolver
}

func (a *autenticacionFalsa) CerrarSesion(_ context.Context, token string) error {
	a.revocados = append(a.revocados, token)
	return a.errCerrar
}

func servidor(t *testing.T, auth Autenticacion) http.Handler {
	t.Helper()
	return Nueva(nil, Casos{Auth: auth}, Opciones{}).Router()
}

func servidorCon(t *testing.T, auth Autenticacion, ingresos ConsultaIngresos, explicar ExplicarCifra) http.Handler {
	t.Helper()
	return Nueva(nil, Casos{Auth: auth, Ingresos: ingresos, Explicar: explicar}, Opciones{}).Router()
}

func pedir(t *testing.T, h http.Handler, metodo, ruta, cuerpo, token string) *httptest.ResponseRecorder {
	t.Helper()
	var lector io.Reader
	if cuerpo != "" {
		lector = strings.NewReader(cuerpo)
	}
	req := httptest.NewRequest(metodo, ruta, lector)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodificar(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("el cuerpo no es JSON: %v (%q)", err, rec.Body.String())
	}
	return m
}

// ---------------------------------------------------------------------------
// Login

func TestLoginDevuelveElToken(t *testing.T) {
	expira := time.Date(2026, 8, 30, 23, 0, 0, 0, time.UTC)
	auth := &autenticacionFalsa{sesion: aplicacion.Sesion{
		Token:   "tok-123",
		Expira:  expira,
		Usuario: aplicacion.Usuario{ID: "usr-1", Email: "ana@redes.co", Nombre: "Ana", Rol: aplicacion.RolTitular},
	}}

	rec := pedir(t, servidor(t, auth), http.MethodPost, "/auth/session",
		`{"email":"ana@redes.co","clave":"la-clave"}`, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}

	cuerpo := decodificar(t, rec)
	if cuerpo["token"] != "tok-123" {
		t.Fatalf("token = %v", cuerpo["token"])
	}
	// El contrato promete RFC 3339; sin formato explicito el cliente no puede
	// saber cuando renovar.
	if cuerpo["expira"] != expira.Format(time.RFC3339) {
		t.Fatalf("expira = %v, se esperaba %q", cuerpo["expira"], expira.Format(time.RFC3339))
	}

	usuario, ok := cuerpo["usuario"].(map[string]any)
	if !ok {
		t.Fatalf("no vino el usuario: %v", cuerpo)
	}
	if usuario["rol"] != "titular" {
		t.Fatalf("rol = %v", usuario["rol"])
	}
}

// Ni el hash ni la clave pueden asomar por la respuesta.
func TestLoginNoDevuelveNadaSecreto(t *testing.T) {
	auth := &autenticacionFalsa{sesion: aplicacion.Sesion{
		Token:   "tok-123",
		Usuario: aplicacion.Usuario{ID: "usr-1", Email: "ana@redes.co"},
	}}

	rec := pedir(t, servidor(t, auth), http.MethodPost, "/auth/session",
		`{"email":"ana@redes.co","clave":"la-clave-secreta"}`, "")

	if strings.Contains(rec.Body.String(), "la-clave-secreta") {
		t.Fatalf("la clave vuelve en la respuesta: %s", rec.Body)
	}
	for _, prohibido := range []string{"hash", "password", "clave"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), prohibido) {
			t.Fatalf("la respuesta menciona %q: %s", prohibido, rec.Body)
		}
	}
}

func TestLoginConCredencialesMalasEs401(t *testing.T) {
	auth := &autenticacionFalsa{errIniciar: aplicacion.ErrCredenciales}

	rec := pedir(t, servidor(t, auth), http.MethodPost, "/auth/session",
		`{"email":"ana@redes.co","clave":"la-que-no-es"}`, "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
	cuerpo := decodificar(t, rec)
	if cuerpo["error"] == nil {
		t.Fatalf("falta el campo error: %v", cuerpo)
	}
	// El mensaje no puede decir cual de los dos factores fallo.
	msg := strings.ToLower(cuerpo["error"].(string))
	for _, filtracion := range []string{"no existe", "no encontrado", "usuario desconocido"} {
		if strings.Contains(msg, filtracion) {
			t.Fatalf("el mensaje distingue el factor que fallo: %q", msg)
		}
	}
}

// La distincion que importa: una base caida es 500, no 401. Devolver 401
// manda al usuario a reautenticarse contra una base que no responde.
func TestLoginConFalloDeInfraestructuraEs500(t *testing.T) {
	auth := &autenticacionFalsa{errIniciar: errors.New("connection refused")}

	rec := pedir(t, servidor(t, auth), http.MethodPost, "/auth/session",
		`{"email":"ana@redes.co","clave":"x"}`, "")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("codigo = %d, se esperaba 500", rec.Code)
	}
	// El detalle interno no sale al cliente.
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatalf("el error interno se filtra al cliente: %s", rec.Body)
	}
}

func TestLoginConCuerpoInvalidoEs400(t *testing.T) {
	auth := &autenticacionFalsa{}
	h := servidor(t, auth)

	for _, caso := range []struct {
		nombre string
		cuerpo string
	}{
		{"json roto", `{"email":`},
		{"sin email", `{"clave":"x"}`},
		{"sin clave", `{"email":"ana@redes.co"}`},
		{"vacio", ``},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := pedir(t, h, http.MethodPost, "/auth/session", caso.cuerpo, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400", rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// El middleware

func TestRutaProtegidaSinTokenEs401(t *testing.T) {
	auth := &autenticacionFalsa{}

	rec := pedir(t, servidor(t, auth), http.MethodGet, "/auth/session", "", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
	// Una cabecera WWW-Authenticate le dice al cliente que esquema usar.
	if a := rec.Header().Get("WWW-Authenticate"); !strings.Contains(a, "Bearer") {
		t.Fatalf("WWW-Authenticate = %q, se esperaba que nombrara Bearer", a)
	}
	if decodificar(t, rec)["error"] == nil {
		t.Fatal("el 401 tiene que traer un cuerpo JSON con error")
	}
}

func TestRutaProtegidaConTokenInvalidoEs401(t *testing.T) {
	auth := &autenticacionFalsa{errResolver: aplicacion.ErrNoEncontrado}

	rec := pedir(t, servidor(t, auth), http.MethodGet, "/auth/session", "", "tok-revocado")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
	if auth.tokenRecibido != "tok-revocado" {
		t.Fatalf("el middleware paso %q; no extrajo bien el bearer", auth.tokenRecibido)
	}
}

// Otra vez la misma frontera: si la base esta caida, la sesion de quien
// pregunta no es invalida. 500, no 401.
func TestRutaProtegidaConFalloDeInfraestructuraEs500(t *testing.T) {
	auth := &autenticacionFalsa{errResolver: errors.New("connection refused")}

	rec := pedir(t, servidor(t, auth), http.MethodGet, "/auth/session", "", "tok")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("codigo = %d, se esperaba 500", rec.Code)
	}
}

// Solo el esquema Bearer. Sin esto, un "Authorization: Basic ..." se colaria
// como si el resto de la cadena fuera un token.
func TestSoloSeAceptaElEsquemaBearer(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1"}}
	h := servidor(t, auth)

	for _, cabecera := range []string{
		"Basic dXNlcjpwYXNz",
		"tok-suelto-sin-esquema",
		"Bearer",
		"Bearer ",
	} {
		t.Run(cabecera, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
			req.Header.Set("Authorization", cabecera)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%q dio %d, se esperaba 401", cabecera, rec.Code)
			}
		})
	}
}

// El esquema no distingue mayusculas (RFC 7235). Un cliente que mande
// "bearer" no deberia quedarse fuera.
func TestElEsquemaBearerNoDistingueMayusculas(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAuditor}}
	h := servidor(t, auth)

	for _, esquema := range []string{"Bearer", "bearer", "BEARER"} {
		t.Run(esquema, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
			req.Header.Set("Authorization", esquema+" tok")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%q dio %d, se esperaba 200", esquema, rec.Code)
			}
		})
	}
}

// Lo que requiereRol consume: el Usuario puesto en el contexto por conSesion.
func TestUsuarioDeDevuelveElUsuarioDelContexto(t *testing.T) {
	quiero := aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolDistribucion}
	auth := &autenticacionFalsa{usuario: quiero}

	var visto aplicacion.Usuario
	var hubo bool

	api := Nueva(nil, Casos{Auth: auth}, Opciones{})
	h := api.conSesion(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		visto, hubo = UsuarioDe(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/lo-que-sea", nil)
	req.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if !hubo {
		t.Fatal("conSesion no dejo el usuario en el contexto")
	}
	if visto.ID != quiero.ID || visto.Rol != quiero.Rol {
		t.Fatalf("usuario = %+v, se esperaba %+v", visto, quiero)
	}
}

// Sin sesion no hay usuario, y el segundo valor lo dice. Un cero silencioso
// seria un Usuario con rol vacio, que requiereRol podria comparar contra un
// rol y dejar pasar.
func TestUsuarioDeSinSesionDevuelveFalso(t *testing.T) {
	if _, hubo := UsuarioDe(context.Background()); hubo {
		t.Fatal("sin sesion en el contexto, UsuarioDe tiene que devolver false")
	}
}

// ---------------------------------------------------------------------------
// Whoami y logout

func TestWhoamiDevuelveElUsuarioDeLaSesion(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-1", Email: "ana@redes.co", Nombre: "Ana",
		Rol: aplicacion.RolTitular, TitularID: "tit-ana",
	}}

	rec := pedir(t, servidor(t, auth), http.MethodGet, "/auth/session", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	cuerpo := decodificar(t, rec)
	if cuerpo["email"] != "ana@redes.co" {
		t.Fatalf("email = %v", cuerpo["email"])
	}
	if cuerpo["titular_id"] != "tit-ana" {
		t.Fatalf("titular_id = %v", cuerpo["titular_id"])
	}
}

func TestLogoutRevocaYDevuelve204(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1"}}

	rec := pedir(t, servidor(t, auth), http.MethodDelete, "/auth/session", "", "tok-a-revocar")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("codigo = %d, se esperaba 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("un 204 no lleva cuerpo: %q", rec.Body)
	}
	if len(auth.revocados) != 1 || auth.revocados[0] != "tok-a-revocar" {
		t.Fatalf("no se revoco el token presentado: %v", auth.revocados)
	}
}

func TestLogoutSinTokenEs401(t *testing.T) {
	auth := &autenticacionFalsa{}

	rec := pedir(t, servidor(t, auth), http.MethodDelete, "/auth/session", "", "")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
	if len(auth.revocados) != 0 {
		t.Fatal("no se revoca nada sin sesion valida")
	}
}

// El recorrido completo que pide el issue: entrar, usar la sesion, salir, y
// comprobar que ya no vale.
func TestRoundTripLoginUsarLogout(t *testing.T) {
	auth := &autenticacionFalsa{
		sesion: aplicacion.Sesion{
			Token:   "tok-vivo",
			Expira:  time.Now().UTC().Add(time.Hour),
			Usuario: aplicacion.Usuario{ID: "usr-1", Email: "ana@redes.co", Rol: aplicacion.RolTitular},
		},
		usuario: aplicacion.Usuario{ID: "usr-1", Email: "ana@redes.co", Rol: aplicacion.RolTitular},
	}
	h := servidor(t, auth)

	rec := pedir(t, h, http.MethodPost, "/auth/session",
		`{"email":"ana@redes.co","clave":"la-clave"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d", rec.Code)
	}
	token, _ := decodificar(t, rec)["token"].(string)

	if rec := pedir(t, h, http.MethodGet, "/auth/session", "", token); rec.Code != http.StatusOK {
		t.Fatalf("whoami con sesion viva: %d", rec.Code)
	}

	if rec := pedir(t, h, http.MethodDelete, "/auth/session", "", token); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}

	// Tras el logout el caso de uso ya no resuelve ese token.
	auth.errResolver = aplicacion.ErrNoEncontrado
	if rec := pedir(t, h, http.MethodGet, "/auth/session", "", token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("whoami tras el logout: %d, se esperaba 401", rec.Code)
	}
}

// Las sondas siguen abiertas: si el middleware se montara de mas, el
// orquestador dejaria de poder comprobar el proceso.
func TestLasSondasNoPidenSesion(t *testing.T) {
	h := servidor(t, &autenticacionFalsa{})

	for _, ruta := range []string{"/health", "/ready"} {
		t.Run(ruta, func(t *testing.T) {
			if rec := pedir(t, h, http.MethodGet, ruta, "", ""); rec.Code != http.StatusOK {
				t.Fatalf("%s dio %d sin token, tiene que seguir abierta", ruta, rec.Code)
			}
		})
	}
}
