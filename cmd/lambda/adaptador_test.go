package main

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// El adaptador es la unica logica nueva que atraviesa TODAS las peticiones, y
// sus errores son silenciosos: una cabecera que se pierde o una cookie
// aplastada no rompen el build, rompen la sesion de alguien.

func evento(metodo, ruta string) events.LambdaFunctionURLRequest {
	ev := events.LambdaFunctionURLRequest{
		RawPath: ruta,
		Headers: map[string]string{"host": "intela.example"},
	}
	ev.RequestContext.HTTP.Method = metodo
	return ev
}

func TestAHTTPLoBasico(t *testing.T) {
	ev := evento(http.MethodGet, "/auth/session")
	ev.RawQueryString = "page=2&size=10"
	ev.RequestContext.HTTP.SourceIP = "203.0.113.7"
	ev.Headers["authorization"] = "Bearer abc123"

	r, err := aHTTP(context.Background(), ev)
	if err != nil {
		t.Fatalf("aHTTP: %v", err)
	}

	if r.Method != http.MethodGet {
		t.Errorf("metodo = %q, se esperaba GET", r.Method)
	}
	if r.URL.Path != "/auth/session" {
		t.Errorf("ruta = %q", r.URL.Path)
	}
	if got := r.URL.Query().Get("size"); got != "10" {
		t.Errorf("size = %q, se esperaba 10", got)
	}
	// Sin esto el middleware bearer no ve nada y todo responde 401.
	if got := r.Header.Get("Authorization"); got != "Bearer abc123" {
		t.Errorf("Authorization = %q", got)
	}
	// El ADR 0006 pide poder responder quien hizo que.
	if r.RemoteAddr != "203.0.113.7" {
		t.Errorf("RemoteAddr = %q", r.RemoteAddr)
	}
	if r.Host != "intela.example" {
		t.Errorf("Host = %q", r.Host)
	}
}

func TestAHTTPCuerpoEnBase64(t *testing.T) {
	ev := evento(http.MethodPost, "/auth/session")
	ev.Body = base64.StdEncoding.EncodeToString([]byte(`{"correo":"a@b.c"}`))
	ev.IsBase64Encoded = true

	r, err := aHTTP(context.Background(), ev)
	if err != nil {
		t.Fatalf("aHTTP: %v", err)
	}

	cuerpo, _ := io.ReadAll(r.Body)
	if string(cuerpo) != `{"correo":"a@b.c"}` {
		t.Errorf("cuerpo = %q", cuerpo)
	}
	if r.ContentLength != int64(len(cuerpo)) {
		t.Errorf("ContentLength = %d, cuerpo = %d", r.ContentLength, len(cuerpo))
	}
}

func TestAHTTPBase64Invalido(t *testing.T) {
	ev := evento(http.MethodPost, "/")
	ev.Body = "no-es-base64!!!"
	ev.IsBase64Encoded = true

	if _, err := aHTTP(context.Background(), ev); err == nil {
		t.Fatal("se esperaba error con un cuerpo base64 invalido")
	}
}

// Las cookies llegan en su propio campo, fuera de Headers. Si no se recomponen
// aqui, el handler no las ve.
func TestAHTTPCookies(t *testing.T) {
	ev := evento(http.MethodGet, "/")
	ev.Cookies = []string{"a=1", "b=2"}

	r, err := aHTTP(context.Background(), ev)
	if err != nil {
		t.Fatalf("aHTTP: %v", err)
	}
	if got := r.Header.Get("Cookie"); got != "a=1; b=2" {
		t.Errorf("Cookie = %q", got)
	}
}

func TestAHTTPRutaVacia(t *testing.T) {
	r, err := aHTTP(context.Background(), evento(http.MethodGet, ""))
	if err != nil {
		t.Fatalf("aHTTP: %v", err)
	}
	if r.URL.Path != "/" {
		t.Errorf("una ruta vacia debe ser /, no %q", r.URL.Path)
	}
}

func TestAEventoSinEscribirNada(t *testing.T) {
	g := nuevoGrabador()
	res := g.aEvento()

	// net/http trata "el handler no escribio" como 200. Un 0 aqui haria que la
	// Function URL devolviera un error de plataforma.
	if res.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, se esperaba 200", res.StatusCode)
	}
}

// Set-Cookie es la unica cabecera que se repite legitimamente. Unirla con comas
// produce una cookie que el navegador descarta.
func TestAEventoSeparaSetCookie(t *testing.T) {
	g := nuevoGrabador()
	g.Header().Add("Set-Cookie", "sesion=1; HttpOnly")
	g.Header().Add("Set-Cookie", "otra=2; HttpOnly")
	g.Header().Set("Content-Type", "application/json")
	g.WriteHeader(http.StatusCreated)

	res := g.aEvento()

	if res.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d", res.StatusCode)
	}
	if len(res.Cookies) != 2 {
		t.Fatalf("Cookies = %v, se esperaban 2", res.Cookies)
	}
	if _, hay := res.Headers["Set-Cookie"]; hay {
		t.Error("Set-Cookie no debe ir tambien en Headers")
	}
	if res.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", res.Headers["Content-Type"])
	}
}

func TestAEventoTextoVaEnClaro(t *testing.T) {
	g := nuevoGrabador()
	_, _ = g.Write([]byte(`{"error":"ruta no encontrada"}`))

	res := g.aEvento()

	if res.IsBase64Encoded {
		t.Error("un cuerpo de texto valido debe viajar en claro, para poder leerlo en los logs")
	}
	if res.Body != `{"error":"ruta no encontrada"}` {
		t.Errorf("Body = %q", res.Body)
	}
}

func TestAEventoBinarioVaEnBase64(t *testing.T) {
	crudo := []byte{0xff, 0xfe, 0x00, 0x01}

	g := nuevoGrabador()
	_, _ = g.Write(crudo)

	res := g.aEvento()

	if !res.IsBase64Encoded {
		t.Fatal("un cuerpo que no es UTF-8 valido tiene que ir en base64")
	}
	descifrado, err := base64.StdEncoding.DecodeString(res.Body)
	if err != nil {
		t.Fatalf("base64 invalido: %v", err)
	}
	if string(descifrado) != string(crudo) {
		t.Errorf("cuerpo = %v, se esperaba %v", descifrado, crudo)
	}
}

// El primer WriteHeader gana, igual que en net/http.
func TestGrabadorPrimerCodigoGana(t *testing.T) {
	g := nuevoGrabador()
	g.WriteHeader(http.StatusTeapot)
	g.WriteHeader(http.StatusOK)

	if g.aEvento().StatusCode != http.StatusTeapot {
		t.Error("un segundo WriteHeader no debe sobrescribir al primero")
	}
}

// La prueba que importa de verdad: un handler corriente, servido a traves del
// adaptador, se comporta igual que detras de un net/http.Server.
func TestVueltaCompleta(t *testing.T) {
	h := http.NewServeMux()
	h.HandleFunc("/eco", func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"recibido":` + string(cuerpo) + `,"q":"` + r.URL.Query().Get("x") + `"}`))
	})

	ev := evento(http.MethodPost, "/eco")
	ev.RawQueryString = "x=7"
	ev.Body = `"hola"`

	r, err := aHTTP(context.Background(), ev)
	if err != nil {
		t.Fatalf("aHTTP: %v", err)
	}

	g := nuevoGrabador()
	h.ServeHTTP(g, r)
	res := g.aEvento()

	if res.StatusCode != http.StatusAccepted {
		t.Errorf("StatusCode = %d", res.StatusCode)
	}
	if res.Body != `{"recibido":"hola","q":"7"}` {
		t.Errorf("Body = %q", res.Body)
	}
}
