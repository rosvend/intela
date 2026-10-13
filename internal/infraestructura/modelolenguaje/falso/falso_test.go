package falso

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rosvend/intela/internal/aplicacion"
)

var herramientas = []aplicacion.EsquemaHerramienta{
	{Nombre: "buscar_reglamento", Parametros: json.RawMessage(`{"type":"object","properties":{"pregunta":{"type":"string"}},"required":["pregunta"]}`)},
	{Nombre: "listar_oni", Parametros: json.RawMessage(`{"type":"object","properties":{"periodo":{"type":"string"}},"required":["periodo"]}`)},
}

func pregunta(texto string) aplicacion.PeticionModelo {
	return aplicacion.PeticionModelo{
		Herramientas: herramientas,
		Mensajes:     []aplicacion.MensajeModelo{{Rol: aplicacion.RolMensajeUsuario, Texto: texto}},
	}
}

func TestFalsoLlamaALaHerramientaCuyoNombreAparece(t *testing.T) {
	r, err := Modelo{}.Responder(t.Context(), pregunta("que dice el Reglamento sobre la reserva?"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Llamadas) != 1 || r.Llamadas[0].Nombre != "buscar_reglamento" {
		t.Fatalf("llamadas = %+v", r.Llamadas)
	}
	var args map[string]string
	if err := json.Unmarshal(r.Llamadas[0].Argumentos, &args); err != nil || args["pregunta"] == "" {
		t.Errorf("argumentos = %s", r.Llamadas[0].Argumentos)
	}
}

func TestFalsoExtraeElPeriodoParaUnArgumentoDePeriodo(t *testing.T) {
	r, _ := Modelo{}.Responder(t.Context(), pregunta("cuantas ONI hay en 2026-01?"))
	if len(r.Llamadas) != 1 || string(r.Llamadas[0].Argumentos) != `{"periodo":"2026-01"}` {
		t.Fatalf("llamadas = %+v", r.Llamadas)
	}
}

func TestFalsoResumeLosResultadosSinVolverALlamar(t *testing.T) {
	p := pregunta("reglamento")
	p.Mensajes = append(p.Mensajes,
		aplicacion.MensajeModelo{Rol: aplicacion.RolMensajeAsistente, Llamadas: []aplicacion.LlamadaHerramienta{{ID: "f1", Nombre: "buscar_reglamento"}}},
		aplicacion.MensajeModelo{Rol: aplicacion.RolMensajeUsuario, Resultados: []aplicacion.ResultadoHerramienta{{LlamadaID: "f1", Contenido: "<tool_result name=\"buscar_reglamento\">\n{\"cita\":\"RD 9.1.1\"}\n</tool_result>"}}},
	)
	r, err := Modelo{}.Responder(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Llamadas) != 0 || !strings.Contains(r.Texto, "RD 9.1.1") {
		t.Fatalf("respuesta = %+v", r)
	}
}

func TestFalsoSinHerramientaQueEncajeDiceQueEsUnaDemostracion(t *testing.T) {
	r, _ := Modelo{}.Responder(t.Context(), pregunta("hola"))
	if len(r.Llamadas) != 0 || !strings.Contains(r.Texto, "demostración") {
		t.Fatalf("respuesta = %+v", r)
	}
}

func TestFalsoMarcasDePruebaFuerzanFalloYRespuestaParcial(t *testing.T) {
	if _, err := (Modelo{}).Responder(t.Context(), pregunta("#fallo")); err == nil {
		t.Error("#fallo deberia devolver error")
	}
	p := pregunta("#parcial reglamento")
	p.Mensajes = append(p.Mensajes, aplicacion.MensajeModelo{Rol: aplicacion.RolMensajeUsuario, Resultados: []aplicacion.ResultadoHerramienta{{LlamadaID: "x"}}})
	r, _ := Modelo{}.Responder(t.Context(), p)
	if len(r.Llamadas) != 1 {
		t.Errorf("#parcial deberia seguir llamando herramientas: %+v", r)
	}
}

func TestFalsoRecortaLosResultadosLargosSinPartirUnaRuna(t *testing.T) {
	p := pregunta("hola")
	datos := strings.Repeat("a", 1199) + strings.Repeat("ñ", 10)
	p.Mensajes = append(p.Mensajes, aplicacion.MensajeModelo{Rol: aplicacion.RolMensajeUsuario, Resultados: []aplicacion.ResultadoHerramienta{{Contenido: datos}}})
	r, err := Modelo{}.Responder(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(r.Texto) {
		t.Fatal("el texto recortado no es UTF-8 valido")
	}
	if !strings.Contains(r.Texto, strings.Repeat("a", 1199)+"...") {
		t.Errorf("no recorto en la runa anterior al limite: %q", r.Texto[len(r.Texto)-12:])
	}
}

func TestNoDisponibleRespondeSinError(t *testing.T) {
	r, err := NoDisponible{}.Responder(t.Context(), pregunta("hola"))
	if err != nil || !strings.Contains(r.Texto, "no está disponible") {
		t.Fatalf("r=%+v err=%v", r, err)
	}
}
