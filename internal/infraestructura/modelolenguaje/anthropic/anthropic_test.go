package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// servidorFalso responde con `respuesta` y guarda el cuerpo JSON de la peticion.
func servidorFalso(t *testing.T, codigo int, respuesta string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var recibido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(cuerpo, &recibido)
		if r.Header.Get("X-Api-Key") != "clave-de-prueba" {
			t.Errorf("x-api-key = %q", r.Header.Get("X-Api-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(codigo)
		_, _ = io.WriteString(w, respuesta)
	}))
	t.Cleanup(srv.Close)
	return srv, &recibido
}

func peticion() aplicacion.PeticionModelo {
	return aplicacion.PeticionModelo{
		Sistema:  "eres el asistente",
		Contexto: "rol administrador",
		Herramientas: []aplicacion.EsquemaHerramienta{{
			Nombre: "eco", Descripcion: "devuelve",
			Parametros: json.RawMessage(`{"type":"object","properties":{"texto":{"type":"string"}},"required":["texto"],"additionalProperties":false}`),
		}},
		Mensajes: []aplicacion.MensajeModelo{
			{Rol: aplicacion.RolMensajeUsuario, Texto: "hola"},
			{Rol: aplicacion.RolMensajeAsistente, Texto: "miro", Llamadas: []aplicacion.LlamadaHerramienta{{ID: "toolu_1", Nombre: "eco", Argumentos: json.RawMessage(`{"texto":"a"}`)}}},
			{Rol: aplicacion.RolMensajeUsuario, Resultados: []aplicacion.ResultadoHerramienta{{LlamadaID: "toolu_1", Contenido: "<tool_result name=\"eco\">{}</tool_result>", EsError: true}}},
		},
	}
}

const respuestaConHerramienta = `{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[
 {"type":"text","text":"voy a buscar"},
 {"type":"tool_use","id":"toolu_2","name":"eco","input":{"texto":"b"}}
],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`

func TestTraduceLaPeticionAlFormatoDelProveedor(t *testing.T) {
	srv, recibido := servidorFalso(t, http.StatusOK, respuestaConHerramienta)
	m := Nuevo("clave-de-prueba", "modelo-x", ConURLBase(srv.URL))

	resp, err := m.Responder(t.Context(), peticion())
	if err != nil {
		t.Fatalf("Responder: %v", err)
	}

	req := *recibido
	if req["model"] != "modelo-x" {
		t.Errorf("model = %v", req["model"])
	}
	system := req["system"].([]any)
	if len(system) != 2 || system[0].(map[string]any)["cache_control"] == nil || system[1].(map[string]any)["text"] != "rol administrador" {
		t.Errorf("system = %v", system)
	}
	tool := req["tools"].([]any)[0].(map[string]any)
	esquema := tool["input_schema"].(map[string]any)
	if tool["name"] != "eco" || esquema["additionalProperties"] != false || esquema["required"].([]any)[0] != "texto" {
		t.Errorf("tool = %v", tool)
	}
	msgs := req["messages"].([]any)
	asistente := msgs[1].(map[string]any)["content"].([]any)
	if asistente[1].(map[string]any)["type"] != "tool_use" || asistente[1].(map[string]any)["id"] != "toolu_1" {
		t.Errorf("turno del asistente = %v", asistente)
	}
	resultado := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if resultado["type"] != "tool_result" || resultado["tool_use_id"] != "toolu_1" || resultado["is_error"] != true {
		t.Errorf("tool_result = %v", resultado)
	}

	if resp.Texto != "voy a buscar" || len(resp.Llamadas) != 1 || resp.Llamadas[0].ID != "toolu_2" || string(resp.Llamadas[0].Argumentos) != `{"texto":"b"}` {
		t.Errorf("respuesta = %+v", resp)
	}
}

func TestUnErrorDelProveedorSubeComoError(t *testing.T) {
	srv, _ := servidorFalso(t, http.StatusServiceUnavailable, `{"type":"error","error":{"type":"overloaded_error","message":"x"}}`)
	m := Nuevo("clave-de-prueba", "modelo-x", ConURLBase(srv.URL))
	if _, err := m.Responder(t.Context(), peticion()); err == nil {
		t.Fatal("se esperaba error")
	}
}

func TestUnRechazoDelModeloVuelveComoTextoSinLlamadas(t *testing.T) {
	srv, _ := servidorFalso(t, http.StatusOK, `{"id":"m","type":"message","role":"assistant","model":"m","content":[],"stop_reason":"refusal","usage":{"input_tokens":1,"output_tokens":1}}`)
	m := Nuevo("clave-de-prueba", "modelo-x", ConURLBase(srv.URL))
	resp, err := m.Responder(t.Context(), peticion())
	if err != nil || resp.Texto == "" || len(resp.Llamadas) != 0 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}
