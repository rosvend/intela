package herramientas_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/aplicacion/herramientas"
)

type corridasFalsas struct {
	actor    aplicacion.Usuario
	porID    string
	periodo  string
	llamadaA string
	estado   aplicacion.EstadoCorrida
	estados  []aplicacion.EstadoCorrida
	err      error
}

func (c *corridasFalsas) PorID(_ context.Context, actor aplicacion.Usuario, id string) (aplicacion.EstadoCorrida, error) {
	c.actor, c.porID, c.llamadaA = actor, id, "PorID"
	return c.estado, c.err
}

func (c *corridasFalsas) DePeriodo(_ context.Context, actor aplicacion.Usuario, periodo string) ([]aplicacion.EstadoCorrida, error) {
	c.actor, c.periodo, c.llamadaA = actor, periodo, "DePeriodo"
	return c.estados, c.err
}

var contadora = aplicacion.Usuario{ID: "usr-cont", Rol: aplicacion.RolContabilidad}

func catalogo(t *testing.T, hs ...aplicacion.Herramienta) aplicacion.CatalogoHerramientas {
	t.Helper()
	c, err := aplicacion.NuevoCatalogoHerramientas(hs...)
	if err != nil {
		t.Fatalf("catalogo: %v", err)
	}
	return c
}

func comoJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestEstadoCorridaPorIDPasaElActorYDevuelveLaEtapa(t *testing.T) {
	uc := &corridasFalsas{estado: aplicacion.EstadoCorrida{
		ProcesoID: "proc-1", Circuito: "internacional", Etapa: "verificacion", Paso: 4, TotalPasos: 8,
		FirmasFaltantes: []string{"contabilidad"}, Pendiente: "falta la firma de contabilidad", Compuerta: true,
	}}
	res, err := catalogo(t, herramientas.EstadoCorrida(uc)).Ejecutar(context.Background(), contadora, "estado_corrida", json.RawMessage(`{"proceso_id":"proc-1"}`))
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if uc.llamadaA != "PorID" || uc.porID != "proc-1" || uc.actor != contadora {
		t.Fatalf("llamada = %s(%q) con %+v", uc.llamadaA, uc.porID, uc.actor)
	}
	m := comoJSON(t, res)
	if m["circuito"] != "internacional" || m["etapa"] != "verificacion" || m["compuerta_doble_firma"] != true || m["pendiente"] == "" {
		t.Fatalf("resultado = %v", m)
	}
}

func TestEstadoCorridaSinIDListaElPeriodoConTope(t *testing.T) {
	uc := &corridasFalsas{}
	for i := range 25 {
		uc.estados = append(uc.estados, aplicacion.EstadoCorrida{ProcesoID: fmt.Sprintf("proc-%02d", i), FirmasFaltantes: []string{}})
	}
	res, err := catalogo(t, herramientas.EstadoCorrida(uc)).Ejecutar(context.Background(), contadora, "estado_corrida", json.RawMessage(`{"periodo":"2026-01"}`))
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if uc.llamadaA != "DePeriodo" || uc.periodo != "2026-01" || uc.actor != contadora {
		t.Fatalf("llamada = %s(%q) con %+v", uc.llamadaA, uc.periodo, uc.actor)
	}
	m := comoJSON(t, res)
	if m["total"] != float64(25) || len(m["corridas"].([]any)) != herramientas.MaxCorridasPorConsulta {
		t.Fatalf("total=%v corridas=%d", m["total"], len(m["corridas"].([]any)))
	}
}

func TestEstadoCorridaPropagaLosErroresDelCasoDeUso(t *testing.T) {
	for _, quiere := range []error{aplicacion.ErrNoAutorizado, aplicacion.ErrNoEncontrado, aplicacion.ErrPeriodoInvalido} {
		uc := &corridasFalsas{err: fmt.Errorf("envuelto: %w", quiere)}
		_, err := catalogo(t, herramientas.EstadoCorrida(uc)).Ejecutar(context.Background(), contadora, "estado_corrida", json.RawMessage(`{"proceso_id":"x"}`))
		if !errors.Is(err, quiere) {
			t.Errorf("err = %v, se esperaba %v", err, quiere)
		}
	}
}

func TestEstadoCorridaRechazaArgumentosFueraDelEsquema(t *testing.T) {
	uc := &corridasFalsas{}
	for _, args := range []string{`{"proceso_id":5}`, `{"avanzar":true}`} {
		_, err := catalogo(t, herramientas.EstadoCorrida(uc)).Ejecutar(context.Background(), contadora, "estado_corrida", json.RawMessage(args))
		if !errors.Is(err, aplicacion.ErrArgumentosInvalidos) {
			t.Errorf("%s: err = %v", args, err)
		}
	}
	if uc.llamadaA != "" {
		t.Fatalf("llego al caso de uso con argumentos invalidos: %s", uc.llamadaA)
	}
}
