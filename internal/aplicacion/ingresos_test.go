package aplicacion

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

type repoIngresos struct {
	filas           []Ingreso
	err             error
	titularRecibido string
	filtro          FiltroIngresos
	llamadas        int
}

func (r *repoIngresos) IngresosDe(_ context.Context, titularID string, f FiltroIngresos) ([]Ingreso, error) {
	r.llamadas++
	r.titularRecibido = titularID
	r.filtro = f
	return r.filas, r.err
}

func TestMisIngresosPasaElTitularDeLaSesion(t *testing.T) {
	repo := &repoIngresos{filas: []Ingreso{{
		Ref:  FormarRef("proc-1", "obra-completa", "tit-ana"),
		Neto: decimal.RequireFromString("4500.00"),
	}}}
	c := ConsultaIngresos{Repo: repo}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}
	f := FiltroIngresos{ObraID: "obra-completa", Fuente: "caracol", Periodo: "2026-01"}

	filas, err := c.MisIngresos(context.Background(), actor, f)
	if err != nil {
		t.Fatalf("MisIngresos: %v", err)
	}
	if repo.titularRecibido != "tit-ana" {
		t.Fatalf("el repositorio recibio titular %q; tiene que ser el de la sesion", repo.titularRecibido)
	}
	if repo.filtro != f {
		t.Fatalf("filtro = %+v", repo.filtro)
	}
	if len(filas) != 1 || filas[0].Neto.StringFixed(2) != "4500.00" {
		t.Fatalf("filas = %+v", filas)
	}
}

func TestMisIngresosNuncaTomaUnTitularDelFiltro(t *testing.T) {
	repo := &repoIngresos{}
	c := ConsultaIngresos{Repo: repo}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	// Un cliente malicioso no tiene donde poner el titular ajeno: el
	// filtro no lleva ese campo. Se comprueba igual que el caso de uso
	// ignora el ID de usuario (usr-ana no es tit-ana).
	_, err := c.MisIngresos(context.Background(), actor, FiltroIngresos{})
	if err != nil {
		t.Fatalf("MisIngresos: %v", err)
	}
	if repo.titularRecibido != "tit-ana" {
		t.Fatalf("titularRecibido = %q", repo.titularRecibido)
	}
	if repo.titularRecibido == actor.ID {
		t.Fatal("se comparo el id de usuario; hay que usar TitularID")
	}
}

func TestMisIngresosAdminNoLista(t *testing.T) {
	repo := &repoIngresos{filas: []Ingreso{{Ref: "x"}}}
	c := ConsultaIngresos{Repo: repo}

	_, err := c.MisIngresos(context.Background(), Usuario{ID: "usr-1", Rol: RolAdministrador}, FiltroIngresos{})
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("err = %v, se esperaba ErrNoAutorizado", err)
	}
	if repo.llamadas != 0 {
		t.Fatal("no tiene que consultar la base si el rol no es titular")
	}
}

func TestMisIngresosTitularSinTitularIDNoLista(t *testing.T) {
	repo := &repoIngresos{}
	c := ConsultaIngresos{Repo: repo}

	_, err := c.MisIngresos(context.Background(), Usuario{ID: "usr-ana", Rol: RolTitular}, FiltroIngresos{})
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("err = %v", err)
	}
	if repo.llamadas != 0 {
		t.Fatal("sin TitularID no se consulta")
	}
}

func TestMisIngresosNilSeVuelveListaVacia(t *testing.T) {
	c := ConsultaIngresos{Repo: &repoIngresos{filas: nil}}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	filas, err := c.MisIngresos(context.Background(), actor, FiltroIngresos{})
	if err != nil {
		t.Fatalf("MisIngresos: %v", err)
	}
	if filas == nil {
		t.Fatal("nil se serializa como JSON null; tiene que ser una lista vacia")
	}
	if len(filas) != 0 {
		t.Fatalf("len = %d", len(filas))
	}
}

func TestParsearRef(t *testing.T) {
	p, o, tit, err := ParsearRef("proc-1:obra-completa:tit-ana")
	if err != nil {
		t.Fatalf("ParsearRef: %v", err)
	}
	if p != "proc-1" || o != "obra-completa" || tit != "tit-ana" {
		t.Fatalf("%s %s %s", p, o, tit)
	}
	if FormarRef(p, o, tit) != "proc-1:obra-completa:tit-ana" {
		t.Fatal("FormarRef no es el inverso")
	}
}

func TestParsearRefInvalidoEsNoEncontrado(t *testing.T) {
	for _, ref := range []string{"", "solo-uno", "a:b", "a:b:c:d", "::tit-ana", "p::t"} {
		_, _, _, err := ParsearRef(ref)
		if !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("ref %q: err = %v", ref, err)
		}
	}
}
