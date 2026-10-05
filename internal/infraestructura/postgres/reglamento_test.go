package postgres

import (
	"context"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// motorConstante embebe cualquier texto en el mismo vector del modelo m1.
type motorConstante []float32

func (m motorConstante) Embeber(context.Context, string) (aplicacion.Embedding, error) {
	return aplicacion.Embedding{Modelo: "m1", Vector: m}, nil
}

func seccion(cita, texto string, v ...float32) aplicacion.SeccionIndexada {
	return aplicacion.SeccionIndexada{
		SeccionReglamento: aplicacion.SeccionReglamento{Cita: cita, Reglamento: "Reglamento de Distribucion IX", Titulo: "t " + cita, Texto: texto},
		Vector:            v,
	}
}

func TestReglamentoBuscaPorCosenoDentroDelModelo(t *testing.T) {
	s := Nuevo(testhelp.Pool(t))
	ctx := t.Context()
	if err := s.IndexarSecciones(ctx, "m1", []aplicacion.SeccionIndexada{
		seccion("RD 9.1.1", "Total puntos por obra", 1, 0, 0),
		seccion("RD 15", "Prescripciones", 0, 1, 0),
		seccion("RD 5", "Reparto", 0.7, 0.7, 0),
	}); err != nil {
		t.Fatalf("Reemplazar: %v", err)
	}
	// Otro modelo con otra dimension: no puede colarse ni romper la consulta.
	if err := s.IndexarSecciones(ctx, "m2", []aplicacion.SeccionIndexada{seccion("RD 9.1.1", "x", 1, 0)}); err != nil {
		t.Fatalf("Reemplazar m2: %v", err)
	}

	cs, err := s.BuscarSecciones(ctx, aplicacion.Embedding{Modelo: "m1", Vector: []float32{0.9, 0.1, 0}}, 2)
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if len(cs) != 2 || cs[0].Seccion.Cita != "RD 9.1.1" || cs[1].Seccion.Cita != "RD 5" {
		t.Fatalf("orden por similitud incorrecto: %+v", cs)
	}
	if cs[0].Similitud < 0.99 || cs[0].Similitud > 1 || cs[0].Seccion.Texto != "Total puntos por obra" {
		t.Errorf("primera coincidencia = %+v", cs[0])
	}
}

func TestReglamentoPisoDejaFueraLoQueNoSeParece(t *testing.T) {
	s := Nuevo(testhelp.Pool(t))
	ctx := t.Context()
	if err := s.IndexarSecciones(ctx, "m1", []aplicacion.SeccionIndexada{
		seccion("RD 9.1.1", "Total puntos por obra", 1, 0, 0),
		seccion("RD 15", "Prescripciones", 0, 1, 0),
	}); err != nil {
		t.Fatal(err)
	}
	uc := aplicacion.ConsultarReglamento{Almacen: s, Piso: 0.5, Motor: motorConstante{0, 0.2, 1}}

	r, err := uc.Consultar(ctx, aplicacion.Usuario{ID: "u", Rol: aplicacion.RolAuditor}, "algo lejano")
	if err != nil {
		t.Fatal(err)
	}
	if r.Encontrado {
		t.Fatalf("nada supera el piso y se devolvio %+v", r.Secciones)
	}

	uc.Motor = motorConstante{0.1, 1, 0}
	r, err = uc.Consultar(ctx, aplicacion.Usuario{ID: "u", Rol: aplicacion.RolAuditor}, "prescripcion")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Encontrado || len(r.Secciones) != 1 || r.Secciones[0].Seccion.Cita != "RD 15" {
		t.Fatalf("se esperaba solo RD 15 sobre el piso; got %+v", r)
	}
}

func TestReglamentoReemplazarBorraNumeralesQueYaNoExisten(t *testing.T) {
	s := Nuevo(testhelp.Pool(t))
	ctx := t.Context()
	if err := s.IndexarSecciones(ctx, "m1", []aplicacion.SeccionIndexada{seccion("RD 1", "a", 1, 0), seccion("RD 2", "b", 0, 1)}); err != nil {
		t.Fatal(err)
	}
	if err := s.IndexarSecciones(ctx, "m1", []aplicacion.SeccionIndexada{seccion("RD 2", "b2", 0, 1)}); err != nil {
		t.Fatal(err)
	}

	cs, err := s.BuscarSecciones(ctx, aplicacion.Embedding{Modelo: "m1", Vector: []float32{1, 0}}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Seccion.Cita != "RD 2" || cs[0].Seccion.Texto != "b2" {
		t.Fatalf("quedo un numeral huerfano o texto viejo: %+v", cs)
	}
}

func TestReglamentoBuscarSinIndiceDevuelveVacio(t *testing.T) {
	s := Nuevo(testhelp.Pool(t))

	cs, err := s.BuscarSecciones(t.Context(), aplicacion.Embedding{Modelo: "nadie", Vector: []float32{1}}, 3)
	if err != nil || len(cs) != 0 {
		t.Fatalf("cs=%v err=%v", cs, err)
	}
}
