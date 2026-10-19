package herramientas_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/aplicacion/herramientas"
)

type oniFalsa struct {
	actor    aplicacion.Usuario
	filtro   aplicacion.FiltroONI
	llamadas int
	cola     aplicacion.ColaONI
	err      error
}

func (o *oniFalsa) Ejecutar(_ context.Context, actor aplicacion.Usuario, f aplicacion.FiltroONI) (aplicacion.ColaONI, error) {
	o.actor, o.filtro = actor, f
	o.llamadas++
	return o.cola, o.err
}

var administrador = aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador}

func TestListarONIPasaFiltrosYActor(t *testing.T) {
	uc := &oniFalsa{cola: aplicacion.ColaONI{Pendientes: 12, Obras: []aplicacion.ONIPendiente{{
		UsoID: "uso-1", Titulo: "La reina", Fuente: "caracol", Periodo: "2026-01",
		DetectadaEn: time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC), Candidatos: 2,
	}}}}
	res, err := catalogo(t, herramientas.ListarONI(uc)).Ejecutar(context.Background(), administrador, "listar_oni",
		json.RawMessage(`{"periodo":"2026-01","fuente":"caracol","limite":5}`))
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if uc.actor != administrador || uc.filtro != (aplicacion.FiltroONI{Periodo: "2026-01", Fuente: "caracol", Limite: 5}) {
		t.Fatalf("llamada con %+v y %+v", uc.actor, uc.filtro)
	}
	m := comoJSON(t, res)
	obras := m["obras"].([]any)
	if m["pendientes_total"] != float64(12) || len(obras) != 1 {
		t.Fatalf("resultado = %v", m)
	}
	o := obras[0].(map[string]any)
	if o["titulo"] != "La reina" || o["detectada_en"] != "2026-02-03" || o["candidatos_dudosos"] != float64(2) {
		t.Fatalf("obra = %v", o)
	}
}

func TestListarONISinFiltrosLeeLaColaEntera(t *testing.T) {
	uc := &oniFalsa{cola: aplicacion.ColaONI{Obras: []aplicacion.ONIPendiente{}}}
	res, err := catalogo(t, herramientas.ListarONI(uc)).Ejecutar(context.Background(), administrador, "listar_oni", nil)
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if uc.filtro != (aplicacion.FiltroONI{}) {
		t.Fatalf("filtro = %+v", uc.filtro)
	}
	if obras, ok := comoJSON(t, res)["obras"].([]any); !ok || len(obras) != 0 {
		t.Fatalf("una cola vacia es [], no null: %v", res)
	}
}

func TestListarONIPropagaLosErroresDelCasoDeUso(t *testing.T) {
	for _, quiere := range []error{aplicacion.ErrNoAutorizado, aplicacion.ErrPeriodoInvalido, aplicacion.ErrFiltroInvalido} {
		uc := &oniFalsa{err: fmt.Errorf("envuelto: %w", quiere)}
		_, err := catalogo(t, herramientas.ListarONI(uc)).Ejecutar(context.Background(), administrador, "listar_oni", json.RawMessage(`{}`))
		if !errors.Is(err, quiere) {
			t.Errorf("err = %v, se esperaba %v", err, quiere)
		}
	}
}

func TestListarONIRechazaArgumentosFueraDelEsquema(t *testing.T) {
	uc := &oniFalsa{}
	for _, args := range []string{`{"limite":2.5}`, `{"periodo":202601}`, `{"titular_id":"tit-1"}`} {
		_, err := catalogo(t, herramientas.ListarONI(uc)).Ejecutar(context.Background(), administrador, "listar_oni", json.RawMessage(args))
		if !errors.Is(err, aplicacion.ErrArgumentosInvalidos) {
			t.Errorf("%s: err = %v", args, err)
		}
	}
	if uc.llamadas != 0 {
		t.Fatal("llego al caso de uso con argumentos invalidos")
	}
}
