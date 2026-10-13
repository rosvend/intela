package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// El contrato SSE que lee web/src/agente, escrito como literales y no derivado de los structs.
var contratoEventosAgente = map[string][]string{
	"tool_call": {"herramienta"},
	"answer":    {"texto", "parcial", "restringida"},
	"error":     {"mensaje"},
}

func TestElContratoSSEDelAgenteFijaNombresDeEventoYCampos(t *testing.T) {
	ag := agenteFalso{eventos: []aplicacion.Evento{
		{Tipo: aplicacion.EventoHerramienta, Herramienta: "buscar_reglamento"},
		{Tipo: aplicacion.EventoRespuesta, Texto: "RD 9.1.1"},
		{Tipo: aplicacion.EventoFallo, Texto: "caido"},
	}}
	rec := pedir(t, servidorConAgente(t, usuarioStaff, ag), http.MethodPost, "/agente/consulta", `{"mensaje":"x"}`, "tok")

	vistos := map[string]bool{}
	for _, bloque := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n\n") {
		lineas := strings.Split(bloque, "\n")
		if len(lineas) != 2 || !strings.HasPrefix(lineas[0], "event: ") || !strings.HasPrefix(lineas[1], "data: ") {
			t.Fatalf("bloque SSE mal formado: %q", bloque)
		}
		nombre := strings.TrimPrefix(lineas[0], "event: ")
		campos, conocido := contratoEventosAgente[nombre]
		if !conocido {
			t.Fatalf("evento %q fuera del contrato", nombre)
		}
		var datos map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lineas[1], "data: ")), &datos); err != nil {
			t.Fatalf("data de %q no es JSON: %v", nombre, err)
		}
		for _, c := range campos {
			if _, hay := datos[c]; !hay {
				t.Errorf("evento %q sin la clave %q (web/src/agente la lee)", nombre, c)
			}
		}
		for c := range datos {
			if !slices.Contains(campos, c) {
				t.Errorf("evento %q trae la clave %q que el contrato no declara", nombre, c)
			}
		}
		vistos[nombre] = true
	}
	for nombre := range contratoEventosAgente {
		if !vistos[nombre] {
			t.Errorf("no se emitio el evento %q", nombre)
		}
	}
}
