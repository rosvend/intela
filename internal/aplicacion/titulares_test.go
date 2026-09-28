package aplicacion

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
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

// El guardia de R-01 atravesando el MODELO DE LECTURA, que es como estaba
// cableado antes del item 4 y como seguiria si alguien "unificara" los dos
// valores de los `main` otra vez.
//
// El cableado de un `main` no tiene test unitario (D-007: un
// `Declaraciones.Padron == store` comprueba la linea que se acaba de escribir,
// no la propiedad que importa), asi que lo que se prueba aqui es la propiedad:
// que cablear el caso de uso en vez del store NO esconde a la productora. Hoy
// pasa por una razon concreta y fragil -el modelo de lectura no recorta por su
// cuenta, solo aplica la paginacion que le llega, y el guardia le dice cuantas
// filas pedir-, y ese es justo el hecho que este test deja fijado: el dia que
// `BuscarTitulares` empiece a recortar por algo mas, cae aqui y no en
// produccion.
func TestElGuardiaDeR01VeALaProductoraTambienConElModeloDeLectura(t *testing.T) {
	ana := titularDePrueba(t, "tit-ana", "Ana Escritora", "IPI-00000001", true, afiliacion.ClaseSocio)
	// Con IPI: una juridica puede tenerlo en el padron, y sin el la parte ni
	// siquiera pasaria `repertorio.NuevaDeclaracion`, que es otra puerta.
	productora := titularDePrueba(t, "tit-prod", "Productora del Caribe S.A.S.", "IPI-00000009", false, afiliacion.ClaseAdministrado)
	store := &padronFalso{titulares: []afiliacion.Titular{ana, productora}}

	gestion := &gestionFalsa{}
	d := Declaraciones{
		Gestion: gestion,
		Padron:  Titulares{Padron: store},
		Reloj:   relojFijo{},
	}
	partes := []repertorio.Parte{
		{TitularID: "tit-ana", IPI: "IPI-00000001", Porcentaje: decimal.NewFromInt(50)},
		{TitularID: "tit-prod", IPI: "IPI-00000009", Porcentaje: decimal.NewFromInt(50)},
	}

	_, err := d.GuardarSplits(t.Context(), "obra-1", partes, "usr-admin")
	if !errors.Is(err, ErrTitularNoEsPersonaNatural) {
		t.Fatalf("se esperaba ErrTitularNoEsPersonaNatural, se obtuvo %v", err)
	}
	if gestion.guardadas != 0 {
		t.Fatal("se guardo una declaracion con una sociedad dentro")
	}
}
