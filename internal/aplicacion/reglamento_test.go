package aplicacion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// motorFijo devuelve el vector asignado a cada texto, o falla.
type motorFijo struct {
	vectores map[string][]float32
	err      error
}

func (m motorFijo) Embeber(_ context.Context, texto string) (Embedding, error) {
	if m.err != nil {
		return Embedding{}, m.err
	}
	return Embedding{Modelo: "fijo", Vector: m.vectores[texto]}, nil
}

// almacenFijo devuelve coincidencias guionizadas y recuerda lo indexado.
type almacenFijo struct {
	coincidencias []CoincidenciaReglamento
	topK          int
	buscado       Embedding
	modelo        string
	indexadas     []SeccionIndexada
}

func (a *almacenFijo) BuscarSecciones(_ context.Context, e Embedding, topK int) ([]CoincidenciaReglamento, error) {
	a.buscado, a.topK = e, topK
	return a.coincidencias, nil
}

func (a *almacenFijo) IndexarSecciones(_ context.Context, modelo string, ss []SeccionIndexada) error {
	a.modelo, a.indexadas = modelo, ss
	return nil
}

func TestConsultarReglamentoDevuelveSoloLoQueSuperaElPiso(t *testing.T) {
	alm := &almacenFijo{coincidencias: []CoincidenciaReglamento{
		{Seccion: SeccionReglamento{Cita: "RD 9.1.1", Texto: "Total puntos por obra"}, Similitud: 0.82},
		{Seccion: SeccionReglamento{Cita: "RD 15", Texto: "Prescripciones"}, Similitud: 0.29},
	}}
	uc := ConsultarReglamento{Motor: motorFijo{vectores: map[string][]float32{"puntos": {1, 0}}}, Almacen: alm, Piso: 0.3}

	r, err := uc.Consultar(t.Context(), staff, "puntos")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Encontrado || len(r.Secciones) != 1 || r.Secciones[0].Seccion.Cita != "RD 9.1.1" {
		t.Fatalf("se esperaba solo RD 9.1.1 por encima del piso; got %+v", r)
	}
	if alm.buscado.Modelo != "fijo" || alm.topK != TopKReglamento {
		t.Errorf("busqueda con modelo %q y topK %d", alm.buscado.Modelo, alm.topK)
	}
}

func TestConsultarReglamentoDiceNoEncontradoAntesQueUnaCoincidenciaDebil(t *testing.T) {
	alm := &almacenFijo{coincidencias: []CoincidenciaReglamento{
		{Seccion: SeccionReglamento{Cita: "RT 3.1"}, Similitud: 0.1},
	}}
	uc := ConsultarReglamento{Motor: motorFijo{}, Almacen: alm, Piso: 0.3}

	r, err := uc.Consultar(t.Context(), staff, "receta de arepas")
	if err != nil {
		t.Fatal(err)
	}
	if r.Encontrado || len(r.Secciones) != 0 || !strings.Contains(r.Mensaje, "No encontré") {
		t.Fatalf("se esperaba 'no encontrado' explicito; got %+v", r)
	}
}

func TestConsultarReglamentoDescartaSeccionesSinCita(t *testing.T) {
	alm := &almacenFijo{coincidencias: []CoincidenciaReglamento{{Seccion: SeccionReglamento{Texto: "huerfano"}, Similitud: 0.9}}}
	uc := ConsultarReglamento{Motor: motorFijo{}, Almacen: alm, Piso: 0.3}

	r, err := uc.Consultar(t.Context(), staff, "x")
	if err != nil {
		t.Fatal(err)
	}
	if r.Encontrado {
		t.Fatalf("texto sin cita no puede salir; got %+v", r)
	}
}

func TestConsultarReglamentoValidaEntradaYActor(t *testing.T) {
	uc := ConsultarReglamento{Motor: motorFijo{}, Almacen: &almacenFijo{}, Piso: 0.3}

	if _, err := uc.Consultar(t.Context(), Usuario{}, "x"); !errors.Is(err, ErrNoAutorizado) {
		t.Errorf("usuario cero: err = %v, se esperaba ErrNoAutorizado", err)
	}
	if _, err := uc.Consultar(t.Context(), staff, "   "); !errors.Is(err, ErrArgumentosInvalidos) {
		t.Errorf("pregunta vacia: err = %v, se esperaba ErrArgumentosInvalidos", err)
	}
	if _, err := uc.Consultar(t.Context(), staff, strings.Repeat("a", MaxRunasPreguntaReglamento+1)); !errors.Is(err, ErrArgumentosInvalidos) {
		t.Errorf("pregunta larga: err = %v, se esperaba ErrArgumentosInvalidos", err)
	}
}

func TestConsultarReglamentoPropagaElFalloDelMotor(t *testing.T) {
	boom := errors.New("proveedor caido")
	uc := ConsultarReglamento{Motor: motorFijo{err: boom}, Almacen: &almacenFijo{}, Piso: 0.3}

	if _, err := uc.Consultar(t.Context(), staff, "x"); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestIndexarReglamentoEmbebeCadaSeccionYReemplazaElIndiceDelModelo(t *testing.T) {
	alm := &almacenFijo{}
	motor := motorFijo{vectores: map[string][]float32{"RD 5\nReparto\nTexto": {1, 2}}}
	uc := IndexarReglamento{Motor: motor, Almacen: alm}

	n, err := uc.Indexar(t.Context(), []SeccionReglamento{{Cita: "RD 5", Titulo: "Reparto", Texto: "Texto"}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || alm.modelo != "fijo" || len(alm.indexadas) != 1 || alm.indexadas[0].Vector[1] != 2 {
		t.Fatalf("n=%d modelo=%q indexadas=%+v", n, alm.modelo, alm.indexadas)
	}
}

func TestIndexarReglamentoSeNiegaAIndexarSinCitaOVacio(t *testing.T) {
	uc := IndexarReglamento{Motor: motorFijo{}, Almacen: &almacenFijo{}}

	if _, err := uc.Indexar(t.Context(), nil); err == nil {
		t.Error("indexar nada borraria el indice entero: se esperaba error")
	}
	if _, err := uc.Indexar(t.Context(), []SeccionReglamento{{Texto: "x"}}); err == nil {
		t.Error("seccion sin cita: se esperaba error")
	}
	if _, err := uc.Indexar(t.Context(), []SeccionReglamento{{Cita: "RD 1", Texto: "a"}, {Cita: "RD 1", Texto: "b"}}); err == nil {
		t.Error("cita duplicada: se esperaba error")
	}
}
