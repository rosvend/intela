package aplicacion

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// padronFalso registra el filtro que le llego y devuelve lo que le pongan. Es
// lo que hace comprobable que el defecto de la paginacion y el paso del filtro
// ocurren ANTES de tocar la base.
type padronFalso struct {
	titulares      []afiliacion.Titular
	err            error
	filtroRecibido FiltroTitulares
}

func (p *padronFalso) BuscarTitulares(_ context.Context, f FiltroTitulares) ([]afiliacion.Titular, error) {
	p.filtroRecibido = f
	return p.titulares, p.err
}

func titularDePrueba(t *testing.T, id, nombre, ipi string, personaNatural bool, clase afiliacion.Clase) afiliacion.Titular {
	t.Helper()

	tit, err := afiliacion.NuevoTitular(id, nombre, ipi, personaNatural, clase)
	if err != nil {
		t.Fatalf("construir el titular de prueba: %v", err)
	}
	return tit
}

func TestBuscarTitularesPasaElFiltroTalCual(t *testing.T) {
	padron := &padronFalso{}
	si := true
	quiero := FiltroTitulares{
		Nombre:         "escritora",
		IPI:            "IPI-00000001",
		PersonaNatural: &si,
		Paginacion:     Paginacion{Limite: 25, Desplazamiento: 10},
	}

	if _, err := (Titulares{Padron: padron}).BuscarTitulares(t.Context(), quiero); err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	// DeepEqual y no == porque el filtro lleva un puntero: con == se
	// compararian las direcciones, y el caso de abajo -un `false` explicito
	// que no es "sin filtro"- es justo el que hay que distinguir.
	if !reflect.DeepEqual(padron.filtroRecibido, quiero) {
		t.Fatalf("filtro = %+v, se esperaba %+v", padron.filtroRecibido, quiero)
	}
}

// La garantia de "filtro vacio = primera pagina" vive en el caso de uso, no en
// cada adaptador: asi cualquier PadronTitulares la hereda y se comprueba sin
// Postgres.
func TestBuscarTitularesAplicaPaginacionPorDefecto(t *testing.T) {
	padron := &padronFalso{}

	if _, err := (Titulares{Padron: padron}).BuscarTitulares(t.Context(), FiltroTitulares{}); err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if padron.filtroRecibido.Limite != LimiteObrasPorDefecto {
		t.Fatalf("Limite = %d, se esperaba %d",
			padron.filtroRecibido.Limite, LimiteObrasPorDefecto)
	}
	if padron.filtroRecibido.Desplazamiento != 0 {
		t.Fatalf("Desplazamiento = %d, se esperaba 0", padron.filtroRecibido.Desplazamiento)
	}
}

// LimiteSinTope es una eleccion explicita: ConDefecto no la sustituye.
func TestBuscarTitularesRespetaLimiteSinTope(t *testing.T) {
	padron := &padronFalso{}
	quiero := FiltroTitulares{Paginacion: Paginacion{Limite: LimiteSinTope}}

	if _, err := (Titulares{Padron: padron}).BuscarTitulares(t.Context(), quiero); err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if padron.filtroRecibido.Limite != LimiteSinTope {
		t.Fatalf("Limite = %d, se esperaba LimiteSinTope (%d)",
			padron.filtroRecibido.Limite, LimiteSinTope)
	}
}

// Un filtro sin `persona_natural` no puede convertirse en un `false`: eso
// devolveria solo las personas juridicas justo cuando nadie lo pidio.
func TestBuscarTitularesSinFiltroDePersonaNaturalNoLaFiltra(t *testing.T) {
	padron := &padronFalso{}

	if _, err := (Titulares{Padron: padron}).BuscarTitulares(t.Context(), FiltroTitulares{}); err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if padron.filtroRecibido.PersonaNatural != nil {
		t.Fatalf("PersonaNatural = %v, se esperaba nil",
			*padron.filtroRecibido.PersonaNatural)
	}
}

// El padron sale entero: una productora esta en la lista, y es el tipo del
// dominio -no un booleano suelto- quien dice que no puede recibir reparto
// (`R-01`, `RD 4.5`).
func TestBuscarTitularesDevuelveElPadronConSuElegibilidad(t *testing.T) {
	ana := titularDePrueba(t, "tit-ana", "Ana Escritora", "IPI-00000001", true, afiliacion.ClaseSocio)
	empresa := titularDePrueba(t, "tit-prod", "Productora del Caribe S.A.S.", "", false, afiliacion.ClaseAdministrado)
	padron := &padronFalso{titulares: []afiliacion.Titular{ana, empresa}}

	titulares, err := (Titulares{Padron: padron}).BuscarTitulares(t.Context(), FiltroTitulares{})
	if err != nil {
		t.Fatalf("BuscarTitulares: %v", err)
	}
	if len(titulares) != 2 {
		t.Fatalf("se esperaban los 2 titulares del padron, llegaron %d", len(titulares))
	}
	if !titulares[0].PuedeRecibirReparto() || titulares[1].PuedeRecibirReparto() {
		t.Fatalf("elegibilidad mal leida del padron: %+v", titulares)
	}
}

// El fallo del puerto sube envuelto y sin perder su causa: el adaptador HTTP
// tiene que poder distinguir un fallo de la base de una lista vacia.
func TestBuscarTitularesPropagaElError(t *testing.T) {
	fallo := errors.New("la base no responde")
	padron := &padronFalso{err: fallo}

	_, err := (Titulares{Padron: padron}).BuscarTitulares(t.Context(), FiltroTitulares{})
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error del puerto, se obtuvo %v", err)
	}
}
