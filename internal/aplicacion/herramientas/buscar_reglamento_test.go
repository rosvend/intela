package herramientas

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

type consultaFija struct {
	r        aplicacion.RespuestaReglamento
	pregunta string
	actor    aplicacion.Usuario
}

func (c *consultaFija) Consultar(_ context.Context, actor aplicacion.Usuario, pregunta string) (aplicacion.RespuestaReglamento, error) {
	c.actor, c.pregunta = actor, pregunta
	return c.r, nil
}

func ejecutarBuscar(t *testing.T, uc *consultaFija, args string) map[string]any {
	t.Helper()
	cat, err := aplicacion.NuevoCatalogoHerramientas(BuscarReglamento(uc))
	if err != nil {
		t.Fatal(err)
	}
	actor := aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAuditor}
	out, err := cat.Ejecutar(t.Context(), actor, "buscar_reglamento", json.RawMessage(args))
	if err != nil {
		t.Fatal(err)
	}
	if uc.actor.ID != "usr-1" {
		t.Errorf("la herramienta no paso el actor de la sesion: %+v", uc.actor)
	}
	crudo, _ := json.Marshal(out)
	var m map[string]any
	if err := json.Unmarshal(crudo, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBuscarReglamentoDevuelveTextoYCita(t *testing.T) {
	uc := &consultaFija{r: aplicacion.RespuestaReglamento{Encontrado: true, Secciones: []aplicacion.CoincidenciaReglamento{{
		Seccion:   aplicacion.SeccionReglamento{Cita: "RD 9.1.1", Reglamento: "Reglamento de Distribucion IX", Titulo: "Fórmula para valorización de una obra", Texto: "Total puntos por obra = ..."},
		Similitud: 0.8123,
	}}}}

	m := ejecutarBuscar(t, uc, `{"pregunta":"como se valoriza una obra"}`)

	if uc.pregunta != "como se valoriza una obra" {
		t.Errorf("pregunta = %q", uc.pregunta)
	}
	secciones, _ := m["secciones"].([]any)
	if m["encontrado"] != true || len(secciones) != 1 {
		t.Fatalf("resultado = %v", m)
	}
	s := secciones[0].(map[string]any)
	if s["cita"] != "RD 9.1.1" || !strings.HasPrefix(s["texto"].(string), "Total puntos") {
		t.Errorf("seccion sin texto o sin cita: %v", s)
	}
}

func TestBuscarReglamentoDiceNoEncontrado(t *testing.T) {
	uc := &consultaFija{r: aplicacion.RespuestaReglamento{Mensaje: aplicacion.TextoReglamentoNoEncontrado}}

	m := ejecutarBuscar(t, uc, `{"pregunta":"receta de arepas"}`)

	if m["encontrado"] != false || m["mensaje"] != aplicacion.TextoReglamentoNoEncontrado {
		t.Fatalf("resultado = %v", m)
	}
}

func TestBuscarReglamentoRecortaTextosLargos(t *testing.T) {
	largo := strings.Repeat("á", MaxRunasTextoSeccion+50)
	uc := &consultaFija{r: aplicacion.RespuestaReglamento{Encontrado: true, Secciones: []aplicacion.CoincidenciaReglamento{{
		Seccion: aplicacion.SeccionReglamento{Cita: "RD 13", Texto: largo}, Similitud: 0.5,
	}}}}

	m := ejecutarBuscar(t, uc, `{"pregunta":"x"}`)

	s := m["secciones"].([]any)[0].(map[string]any)
	if n := len([]rune(s["texto"].(string))); n != MaxRunasTextoSeccion+1 {
		t.Errorf("runas = %d, se esperaba %d (con la elipsis)", n, MaxRunasTextoSeccion+1)
	}
}
