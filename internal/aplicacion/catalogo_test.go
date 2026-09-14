package aplicacion

import (
	"context"
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// catalogoFalso cuenta cuantas veces le tocaron la base. Es lo que hace
// comprobable que la validacion corre ANTES y no despues.
type catalogoFalso struct {
	registros      int
	actualizadas   int
	obraRecibida   repertorio.Obra
	filtroRecibido FiltroObras
	err            error
}

func (c *catalogoFalso) Registrar(_ context.Context, o repertorio.Obra) error {
	c.registros++
	c.obraRecibida = o
	return c.err
}

func (c *catalogoFalso) Actualizar(_ context.Context, o repertorio.Obra) error {
	c.actualizadas++
	c.obraRecibida = o
	return c.err
}

func (c *catalogoFalso) PorID(_ context.Context, _ string) (repertorio.Obra, error) {
	return c.obraRecibida, c.err
}

func (c *catalogoFalso) Buscar(_ context.Context, f FiltroObras) ([]repertorio.Obra, error) {
	c.filtroRecibido = f
	return nil, c.err
}

func metadatosValidos() repertorio.Metadatos {
	return repertorio.Metadatos{
		Titulo: "La Casa de las Dos Palmas",
		Genero: "Drama",
		Anio:   1991,
		Tipo:   repertorio.TipoSerie,
		Coautores: []repertorio.Coautor{
			{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: repertorio.RolGuionista},
		},
	}
}

func TestRegistrarObraConstruyeLaEntidadYLaGuarda(t *testing.T) {
	repo := &catalogoFalso{}
	cat := Catalogo{Obras: repo}

	obra, err := cat.RegistrarObra(t.Context(), "obra-1", metadatosValidos())
	if err != nil {
		t.Fatalf("RegistrarObra: %v", err)
	}
	if obra.ID() != "obra-1" {
		t.Fatalf("ID = %q", obra.ID())
	}
	if repo.registros != 1 {
		t.Fatalf("se esperaba 1 escritura, hubo %d", repo.registros)
	}
	if repo.obraRecibida.ID() != "obra-1" {
		t.Fatalf("al puerto le llego otra obra: %q", repo.obraRecibida.ID())
	}
}

// La invariante se comprueba en el nucleo, no en la base: una obra sin genero
// no llega ni a intentarse. Si llegara, el CHECK la rechazaria con un mensaje
// de restriccion en vez de decir que campo falta.
func TestRegistrarObraInvalidaNoTocaElPuerto(t *testing.T) {
	casos := map[string]func(*repertorio.Metadatos){
		"sin titulo":    func(m *repertorio.Metadatos) { m.Titulo = "" },
		"sin genero":    func(m *repertorio.Metadatos) { m.Genero = "" },
		"sin anio":      func(m *repertorio.Metadatos) { m.Anio = 0 },
		"sin coautores": func(m *repertorio.Metadatos) { m.Coautores = nil },
		"coautor sin IPI": func(m *repertorio.Metadatos) {
			m.Coautores[0].IPI = ""
		},
	}

	for nombre, romper := range casos {
		t.Run(nombre, func(t *testing.T) {
			repo := &catalogoFalso{}
			m := metadatosValidos()
			romper(&m)

			_, err := Catalogo{Obras: repo}.RegistrarObra(t.Context(), "obra-1", m)
			if !errors.Is(err, repertorio.ErrObraInvalida) {
				t.Fatalf("se esperaba ErrObraInvalida, se obtuvo %v", err)
			}
			if repo.registros != 0 {
				t.Fatal("se intento escribir una obra que el dominio rechaza")
			}
		})
	}
}

// El centinela del duplicado sube sin envolver en un texto que lo tape: el
// adaptador HTTP lo distingue con errors.Is para responder 409.
func TestRegistrarObraPropagaElDuplicado(t *testing.T) {
	repo := &catalogoFalso{err: ErrObraDuplicada}

	_, err := Catalogo{Obras: repo}.RegistrarObra(t.Context(), "obra-1", metadatosValidos())
	if !errors.Is(err, ErrObraDuplicada) {
		t.Fatalf("se esperaba ErrObraDuplicada, se obtuvo %v", err)
	}
}

// ActualizarMetadatosObra revalida con el mismo constructor que el alta: una
// obra corregida cumple lo mismo que una recien creada.
func TestActualizarMetadatosObraRevalida(t *testing.T) {
	repo := &catalogoFalso{}
	m := metadatosValidos()
	m.Coautores[0].Rol = "director" // RD 7.3.3: no genera derecho de autor

	_, err := Catalogo{Obras: repo}.ActualizarMetadatosObra(t.Context(), "obra-1", m)
	if !errors.Is(err, repertorio.ErrObraInvalida) {
		t.Fatalf("se esperaba ErrObraInvalida, se obtuvo %v", err)
	}
	if repo.actualizadas != 0 {
		t.Fatal("se intento actualizar con unos metadatos que el dominio rechaza")
	}
}

// El id es el que llega por parametro, y es el unico que puede ser: los
// metadatos no tienen campo donde meter otro.
func TestActualizarMetadatosObraConservaElIdentificador(t *testing.T) {
	repo := &catalogoFalso{}

	obra, err := Catalogo{Obras: repo}.ActualizarMetadatosObra(t.Context(), "obra-1", metadatosValidos())
	if err != nil {
		t.Fatalf("ActualizarMetadatosObra: %v", err)
	}
	if obra.ID() != "obra-1" || repo.obraRecibida.ID() != "obra-1" {
		t.Fatalf("id = %q / %q", obra.ID(), repo.obraRecibida.ID())
	}
	if repo.actualizadas != 1 || repo.registros != 0 {
		t.Fatalf("actualizar no puede dar de alta: %d actualizaciones, %d altas",
			repo.actualizadas, repo.registros)
	}
}

func TestBuscarObrasPasaElFiltroTalCual(t *testing.T) {
	repo := &catalogoFalso{}
	quiero := FiltroObras{Titulo: "palmas", Genero: "Drama", IPI: "IPI-1", Anio: 1991}

	if _, err := (Catalogo{Obras: repo}).BuscarObras(t.Context(), quiero); err != nil {
		t.Fatalf("BuscarObras: %v", err)
	}
	if repo.filtroRecibido != quiero {
		t.Fatalf("filtro = %+v, se esperaba %+v", repo.filtroRecibido, quiero)
	}
}
