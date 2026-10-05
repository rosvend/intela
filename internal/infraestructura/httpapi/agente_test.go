package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// modeloDeGuion responde en orden; vive aqui para probar el handler con el bucle real.
type modeloDeGuion struct {
	guion []aplicacion.RespuestaModelo
	i     int
}

func (m *modeloDeGuion) Responder(context.Context, aplicacion.PeticionModelo) (aplicacion.RespuestaModelo, error) {
	r := m.guion[min(m.i, len(m.guion)-1)]
	m.i++
	return r, nil
}

type relojDePrueba struct{}

func (relojDePrueba) Ahora() time.Time { return time.Unix(0, 0) }

func agenteReal(t *testing.T, guion ...aplicacion.RespuestaModelo) aplicacion.AgenteConsulta {
	t.Helper()
	eco := aplicacion.Herramienta{
		Nombre: "eco", Descripcion: "x",
		Esquema: json.RawMessage(`{"type":"object","properties":{}}`),
		Ejecutar: func(context.Context, aplicacion.Usuario, json.RawMessage) (any, error) {
			return map[string]string{"ok": "si"}, nil
		},
	}
	cat, err := aplicacion.NuevoCatalogoHerramientas(eco)
	if err != nil {
		t.Fatal(err)
	}
	return aplicacion.AgenteConsulta{Modelo: &modeloDeGuion{guion: guion}, Herramientas: cat, Reloj: relojDePrueba{}}
}

func servidorConAgente(t *testing.T, usuario aplicacion.Usuario, ag Agente) http.Handler {
	t.Helper()
	return Nueva(Casos{Auth: &autenticacionFalsa{usuario: usuario}, Agente: ag}, Opciones{}).Router()
}

var usuarioStaff = aplicacion.Usuario{ID: "usr-dist", Rol: aplicacion.RolDistribucion}

func TestAgenteTransmiteLosEventosPorSSE(t *testing.T) {
	ag := agenteReal(t,
		aplicacion.RespuestaModelo{Llamadas: []aplicacion.LlamadaHerramienta{{ID: "c1", Nombre: "eco", Argumentos: json.RawMessage(`{}`)}}},
		aplicacion.RespuestaModelo{Texto: "Listo, ver RD 9.1.1."},
	)
	h := servidorConAgente(t, usuarioStaff, ag)

	rec := pedir(t, h, http.MethodPost, "/agente/consulta", `{"mensaje":"hola","historial":[{"rol":"usuario","texto":"antes"}]}`, "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q", ct)
	}
	if rec.Header().Get("X-Accel-Buffering") != "no" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("cabeceras de SSE = %v", rec.Header())
	}
	want := "event: tool_call\ndata: {\"herramienta\":\"eco\"}\n\n" +
		"event: answer\ndata: {\"texto\":\"Listo, ver RD 9.1.1.\",\"parcial\":false,\"restringida\":false}\n\n"
	if rec.Body.String() != want {
		t.Errorf("cuerpo =\n%q\nse esperaba\n%q", rec.Body.String(), want)
	}
}

func TestAgenteEmiteElErrorComoEventoSSE(t *testing.T) {
	ag := agenteFalso{eventos: []aplicacion.Evento{{Tipo: aplicacion.EventoFallo, Texto: "no disponible"}}}
	rec := pedir(t, servidorConAgente(t, usuarioStaff, ag), http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok")
	if rec.Body.String() != "event: error\ndata: {\"mensaje\":\"no disponible\"}\n\n" {
		t.Errorf("cuerpo = %q", rec.Body.String())
	}
}

func TestAgenteSinSesionEs401(t *testing.T) {
	h := Nueva(Casos{Auth: &autenticacionFalsa{}, Agente: agenteFalso{}}, Opciones{}).Router()
	if rec := pedir(t, h, http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestAgenteLoUsaCualquierRolIncluidoTitular(t *testing.T) {
	ag := agenteFalso{eventos: []aplicacion.Evento{{Tipo: aplicacion.EventoRespuesta, Texto: "hola"}}}
	titular := aplicacion.Usuario{ID: "usr-ana", Rol: aplicacion.RolTitular, TitularID: "tit-ana"}
	rec := pedir(t, servidorConAgente(t, titular, ag), http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestAgenteRechazaConsultasInvalidasConJSONAntesDeTransmitir(t *testing.T) {
	casos := []struct {
		nombre, cuerpo string
		codigo         int
	}{
		{"json roto", `{"mensaje":`, http.StatusBadRequest},
		{"mensaje vacio", `{"mensaje":"  "}`, http.StatusBadRequest},
		{"cuerpo enorme", `{"mensaje":"` + strings.Repeat("a", 70<<10) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConAgente(t, usuarioStaff, agenteReal(t, aplicacion.RespuestaModelo{Texto: "x"}))
			rec := pedir(t, h, http.MethodPost, "/agente/consulta", c.cuerpo, "tok")
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.codigo, rec.Body)
			}
			if !strings.Contains(rec.Header().Get("Content-Type"), "json") {
				t.Errorf("content-type = %q", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestAgenteSinCablearEs503(t *testing.T) {
	h := Nueva(Casos{Auth: &autenticacionFalsa{usuario: usuarioStaff}}, Opciones{}).Router()
	if rec := pedir(t, h, http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestAgenteLimitaPorUsuario(t *testing.T) {
	auth := &autenticacionFalsa{usuario: usuarioStaff}
	h := Nueva(Casos{Auth: auth, Agente: agenteFalso{}}, Opciones{}).Router()
	for i := range limiteAgentePorMinuto {
		if rec := pedir(t, h, http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok"); rec.Code != http.StatusOK {
			t.Fatalf("peticion %d: codigo = %d", i, rec.Code)
		}
	}
	if rec := pedir(t, h, http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("codigo = %d, se esperaba 429", rec.Code)
	}
	auth.usuario = aplicacion.Usuario{ID: "usr-otro", Rol: aplicacion.RolAuditor}
	if rec := pedir(t, h, http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok"); rec.Code != http.StatusOK {
		t.Fatalf("otro usuario: codigo = %d", rec.Code)
	}
}

// escritorSinFlush imita el grabador de cmd/lambda: el cuerpo llega entero al final.
type escritorSinFlush struct {
	cabeceras http.Header
	codigo    int
	cuerpo    strings.Builder
}

func (e *escritorSinFlush) Header() http.Header         { return e.cabeceras }
func (e *escritorSinFlush) WriteHeader(c int)           { e.codigo = c }
func (e *escritorSinFlush) Write(p []byte) (int, error) { return e.cuerpo.Write(p) }

func TestAgenteFuncionaSinFlusherComoEnLambda(t *testing.T) {
	ag := agenteFalso{eventos: []aplicacion.Evento{
		{Tipo: aplicacion.EventoHerramienta, Herramienta: "eco"},
		{Tipo: aplicacion.EventoRespuesta, Texto: "fin", Parcial: true},
	}}
	h := servidorConAgente(t, usuarioStaff, ag)
	req := httptest.NewRequest(http.MethodPost, "/agente/consulta", strings.NewReader(`{"mensaje":"x"}`))
	req.Header.Set("Authorization", "Bearer tok")
	w := &escritorSinFlush{cabeceras: http.Header{}}
	h.ServeHTTP(w, req)
	if w.codigo != http.StatusOK || strings.Count(w.cuerpo.String(), "event: ") != 2 || !strings.Contains(w.cuerpo.String(), `"parcial":true`) {
		t.Fatalf("codigo=%d cuerpo=%q", w.codigo, w.cuerpo.String())
	}
}

type agenteFalso struct{ eventos []aplicacion.Evento }

func (a agenteFalso) Responder(_ context.Context, _ aplicacion.Usuario, _ []aplicacion.TurnoConversacion, _ string, emitir func(aplicacion.Evento)) error {
	for _, e := range a.eventos {
		emitir(e)
	}
	return nil
}
