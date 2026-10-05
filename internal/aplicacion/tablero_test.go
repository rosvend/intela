package aplicacion

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

type repoTablero struct {
	obras            []ObraDeclarada
	ordenes          []liquidacion.OrdenDePago
	obrasLiquidadas  int
	procesosPedidos  []string
	cargas           int
	oni              int
	enReserva        int
	corrida          ProcesoVista
	err              error
	titularRecibido  string
	titularDeConteos string
}

func (r *repoTablero) ObrasDeclaradasDe(_ context.Context, titularID string) ([]ObraDeclarada, error) {
	r.titularRecibido = titularID
	return r.obras, r.err
}

func (r *repoTablero) DeTitular(_ context.Context, titularID string) ([]liquidacion.OrdenDePago, error) {
	r.titularRecibido = titularID
	return r.ordenes, r.err
}

func (r *repoTablero) ObrasDeTitularEnProcesos(_ context.Context, titularID string, procesos []string) (int, error) {
	r.titularDeConteos = titularID
	r.procesosPedidos = procesos
	return r.obrasLiquidadas, r.err
}

func (r *repoTablero) CargasPendientes(context.Context) (int, error) { return r.cargas, r.err }

func (r *repoTablero) CasosONIPendientes(context.Context) (int, error) { return r.oni, r.err }

func (r *repoTablero) UltimaCorrida(context.Context) (ProcesoVista, error) { return r.corrida, r.err }

func (r *repoTablero) ContarObrasEnReserva(context.Context) (int, error) { return r.enReserva, r.err }

func parte(titular, ipi, pct string) repertorio.Parte {
	return repertorio.Parte{TitularID: titular, IPI: ipi, Porcentaje: decimal.RequireFromString(pct)}
}

func titularDeTablero() Usuario {
	return Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}
}

func TestTableroSoloTitularConTitularIDVeSusDatos(t *testing.T) {
	actores := []struct {
		nombre string
		actor  Usuario
	}{
		{"administrador", Usuario{ID: "usr-admin", Rol: RolAdministrador}},
		{"titular sin titular_id", Usuario{ID: "usr-x", Rol: RolTitular}},
		{"auditor con titular_id", Usuario{ID: "usr-aud", Rol: RolAuditor, TitularID: "tit-ana"}},
	}
	for _, a := range actores {
		t.Run(a.nombre, func(t *testing.T) {
			tab := Tablero{Repo: &repoTablero{}}
			if _, err := tab.MisObras(t.Context(), a.actor); !errors.Is(err, ErrNoAutorizado) {
				t.Fatalf("MisObras err = %v, se esperaba ErrNoAutorizado", err)
			}
			if _, err := tab.UltimaLiquidacion(t.Context(), a.actor); !errors.Is(err, ErrNoAutorizado) {
				t.Fatalf("UltimaLiquidacion err = %v, se esperaba ErrNoAutorizado", err)
			}
		})
	}
}

func TestTableroMisObrasCalculaElEstadoConElDominio(t *testing.T) {
	repo := &repoTablero{obras: []ObraDeclarada{
		{ID: "o-1", Titulo: "Completa", Declaracion: repertorio.Declaracion{ObraID: "o-1", Partes: []repertorio.Parte{
			parte("tit-ana", "IPI-1", "60"), parte("tit-beto", "IPI-2", "40"),
		}}},
		{ID: "o-2", Titulo: "Suma 60", Declaracion: repertorio.Declaracion{ObraID: "o-2", Partes: []repertorio.Parte{
			parte("tit-ana", "IPI-1", "60"),
		}}},
		{ID: "o-3", Titulo: "Sin IPI", Declaracion: repertorio.Declaracion{ObraID: "o-3", Partes: []repertorio.Parte{
			parte("tit-ana", "IPI-1", "60"), parte("tit-beto", "", "40"),
		}}},
	}}

	obras, err := Tablero{Repo: repo}.MisObras(t.Context(), titularDeTablero())
	if err != nil {
		t.Fatalf("MisObras: %v", err)
	}
	if repo.titularRecibido != "tit-ana" {
		t.Fatalf("titular = %q", repo.titularRecibido)
	}
	quiere := []ObraResumen{
		{ID: "o-1", Titulo: "Completa", Estado: "completa"},
		{ID: "o-2", Titulo: "Suma 60", Estado: "incompleta"},
		{ID: "o-3", Titulo: "Sin IPI", Estado: "incompleta"},
	}
	if len(obras) != len(quiere) {
		t.Fatalf("obras = %+v", obras)
	}
	for i := range quiere {
		if obras[i] != quiere[i] {
			t.Fatalf("obra %d = %+v, se esperaba %+v", i, obras[i], quiere[i])
		}
	}
}

func TestTableroMisObrasVaciaEsListaNoNil(t *testing.T) {
	obras, err := Tablero{Repo: &repoTablero{}}.MisObras(t.Context(), titularDeTablero())
	if err != nil || obras == nil || len(obras) != 0 {
		t.Fatalf("obras = %#v, err = %v", obras, err)
	}
}

func orden(periodo, circuito, neto string, procesos ...string) liquidacion.OrdenDePago {
	return liquidacion.OrdenDePago{
		ID: "liq-" + periodo + "-" + circuito, Periodo: periodo, Circuito: circuito,
		Procesos: procesos, Neto: decimal.RequireFromString(neto), Estado: liquidacion.EstadoEnviada,
	}
}

// La ultima liquidacion sale de ordenes_pago (ADR 0024), la misma fuente que /mis-liquidaciones.
func TestTableroUltimaLiquidacion(t *testing.T) {
	diferida := orden("2026-03", "nacional", "40", "proc-5")
	diferida.Estado = liquidacion.EstadoDiferida
	casos := []struct {
		nombre   string
		ordenes  []liquidacion.OrdenDePago
		quiere   ResumenLiquidacion
		procesos []string
	}{
		{"toma el ultimo periodo", []liquidacion.OrdenDePago{
			orden("2026-01", "nacional", "3900", "proc-1"),
			orden("2026-02", "nacional", "780", "proc-2"),
		}, ResumenLiquidacion{Periodo: "2026-02", Neto: decimal.RequireFromString("780"), Obras: 2}, []string{"proc-2"}},
		{"los dos circuitos del periodo suman", []liquidacion.OrdenDePago{
			orden("2026-02", "nacional", "780", "proc-2", "proc-3"),
			orden("2026-02", "internacional", "100", "proc-4"),
		}, ResumenLiquidacion{Periodo: "2026-02", Neto: decimal.RequireFromString("880"), Obras: 2}, []string{"proc-2", "proc-3", "proc-4"}},
		{"una diferida es la liquidacion de su periodo", []liquidacion.OrdenDePago{
			orden("2026-02", "nacional", "780", "proc-2"), diferida,
		}, ResumenLiquidacion{Periodo: "2026-03", Neto: decimal.RequireFromString("40"), Obras: 2}, []string{"proc-5"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &repoTablero{ordenes: c.ordenes, obrasLiquidadas: 2}
			r, err := Tablero{Repo: repo}.UltimaLiquidacion(t.Context(), titularDeTablero())
			if err != nil {
				t.Fatalf("UltimaLiquidacion: %v", err)
			}
			if repo.titularRecibido != "tit-ana" || repo.titularDeConteos != "tit-ana" {
				t.Fatalf("titular = %q / %q", repo.titularRecibido, repo.titularDeConteos)
			}
			if r.Periodo != c.quiere.Periodo || r.Obras != c.quiere.Obras || !r.Neto.Equal(c.quiere.Neto) {
				t.Fatalf("resumen = %+v, se esperaba %+v", r, c.quiere)
			}
			if !slices.Equal(repo.procesosPedidos, c.procesos) {
				t.Fatalf("procesos = %v, se esperaba %v", repo.procesosPedidos, c.procesos)
			}
		})
	}
}

func TestTableroUltimaLiquidacionSinOrdenesEsNoEncontrado(t *testing.T) {
	casos := map[string]*repoTablero{
		"sin ordenes":         {},
		"error del adaptador": {err: ErrNoEncontrado},
	}
	for nombre, repo := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := (Tablero{Repo: repo}).UltimaLiquidacion(t.Context(), titularDeTablero()); !errors.Is(err, ErrNoEncontrado) {
				t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
			}
		})
	}
}

func TestTableroObrasEnReserva(t *testing.T) {
	n, err := Tablero{Repo: &repoTablero{enReserva: 3}}.ObrasEnReserva(t.Context())
	if err != nil || n != 3 {
		t.Fatalf("n = %d, err = %v, se esperaban 3", n, err)
	}
}

func TestTableroConteos(t *testing.T) {
	repo := &repoTablero{cargas: 3, oni: 7}
	tab := Tablero{Repo: repo}
	if n, err := tab.CargasPendientes(t.Context()); err != nil || n != 3 {
		t.Fatalf("cargas = %d, err = %v", n, err)
	}
	if n, err := tab.ONIPendientes(t.Context()); err != nil || n != 7 {
		t.Fatalf("oni = %d, err = %v", n, err)
	}
}

func TestTableroUltimaCorrida(t *testing.T) {
	casos := []struct {
		nombre string
		repo   *repoTablero
		quiere CorridaResumen
		err    error
	}{
		{"en firma", &repoTablero{corrida: ProcesoVista{
			Periodo: "2026-02", Circuito: reparto.Nacional, Etapa: reparto.EtapaVerificacion,
		}}, CorridaResumen{Periodo: "2026-02", Etapa: "verificacion", Estado: "en_firma"}, nil},
		{"cerrada", &repoTablero{corrida: ProcesoVista{
			Periodo: "2026-01", Circuito: reparto.Nacional, Etapa: reparto.EtapaAuditoria,
		}}, CorridaResumen{Periodo: "2026-01", Etapa: "auditoria", Estado: "cerrada"}, nil},
		{"sin corridas", &repoTablero{err: ErrNoEncontrado}, CorridaResumen{}, ErrNoEncontrado},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := Tablero{Repo: c.repo}.UltimaCorrida(t.Context())
			if !errors.Is(err, c.err) {
				t.Fatalf("err = %v, se esperaba %v", err, c.err)
			}
			if got != c.quiere {
				t.Fatalf("corrida = %+v, se esperaba %+v", got, c.quiere)
			}
		})
	}
}
