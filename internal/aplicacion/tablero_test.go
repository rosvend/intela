package aplicacion

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

type repoTablero struct {
	obras           []ObraDeclarada
	lineas          []LineaDeTitular
	cargas          int
	oni             int
	corrida         ProcesoVista
	declaraciones   map[string]repertorio.Declaracion
	catalogo        []Obra
	paginacion      Paginacion
	err             error
	titularRecibido string
}

func (r *repoTablero) ObrasDeclaradasDe(_ context.Context, titularID string) ([]ObraDeclarada, error) {
	r.titularRecibido = titularID
	return r.obras, r.err
}

func (r *repoTablero) LineasDeTitular(_ context.Context, titularID string) ([]LineaDeTitular, error) {
	r.titularRecibido = titularID
	return r.lineas, r.err
}

func (r *repoTablero) CargasPendientes(context.Context) (int, error) { return r.cargas, r.err }

func (r *repoTablero) CasosONIPendientes(context.Context) (int, error) { return r.oni, r.err }

func (r *repoTablero) UltimaCorrida(context.Context) (ProcesoVista, error) { return r.corrida, r.err }

func (r *repoTablero) Declaraciones(context.Context) (map[string]repertorio.Declaracion, error) {
	return r.declaraciones, r.err
}

func (r *repoTablero) ListarObras(_ context.Context, p Paginacion) ([]Obra, error) {
	r.paginacion = p
	return r.catalogo, r.err
}

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

func linea(periodo string, etapa reparto.Etapa, obra, neto string) LineaDeTitular {
	return LineaDeTitular{
		Periodo: periodo, Circuito: reparto.Nacional, Etapa: etapa,
		ObraID: obra, Neto: decimal.RequireFromString(neto),
	}
}

func TestTableroUltimaLiquidacion(t *testing.T) {
	casos := []struct {
		nombre string
		lineas []LineaDeTitular
		quiere ResumenLiquidacion
	}{
		{"toma el ultimo periodo", []LineaDeTitular{
			linea("2026-01", reparto.EtapaAuditoria, "o-1", "3900"),
			linea("2026-02", reparto.EtapaLiquidacionFinal, "o-1", "780"),
		}, ResumenLiquidacion{Periodo: "2026-02", Neto: decimal.RequireFromString("780"), Obras: 1}},
		{"la misma obra en dos corridas cuenta una vez", []LineaDeTitular{
			linea("2026-02", reparto.EtapaLiquidacionFinal, "o-1", "780"),
			linea("2026-02", reparto.EtapaPagoRegistro, "o-1", "220"),
			linea("2026-02", reparto.EtapaPagoRegistro, "o-2", "100"),
		}, ResumenLiquidacion{Periodo: "2026-02", Neto: decimal.RequireFromString("1100"), Obras: 2}},
		{"una corrida sin firmar no suma ni define el periodo", []LineaDeTitular{
			linea("2026-01", reparto.EtapaLiquidacionFinal, "o-1", "3900"),
			linea("2026-02", reparto.EtapaImporteTitular, "o-1", "780"),
			linea("2026-02", reparto.EtapaVerificacion, "o-2", "50"),
		}, ResumenLiquidacion{Periodo: "2026-01", Neto: decimal.RequireFromString("3900"), Obras: 1}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &repoTablero{lineas: c.lineas}
			r, err := Tablero{Repo: repo}.UltimaLiquidacion(t.Context(), titularDeTablero())
			if err != nil {
				t.Fatalf("UltimaLiquidacion: %v", err)
			}
			if repo.titularRecibido != "tit-ana" {
				t.Fatalf("titular = %q", repo.titularRecibido)
			}
			if r.Periodo != c.quiere.Periodo || r.Obras != c.quiere.Obras || !r.Neto.Equal(c.quiere.Neto) {
				t.Fatalf("resumen = %+v, se esperaba %+v", r, c.quiere)
			}
		})
	}
}

func TestTableroUltimaLiquidacionSinLineasFirmadasEsNoEncontrado(t *testing.T) {
	casos := map[string]*repoTablero{
		"sin lineas":          {},
		"solo sin firmar":     {lineas: []LineaDeTitular{linea("2026-02", reparto.EtapaImporteTitular, "o-1", "780")}},
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

func TestTableroObrasEnReservaCuentaTodoElCatalogo(t *testing.T) {
	repo := &repoTablero{
		declaraciones: map[string]repertorio.Declaracion{
			"o-1": {ObraID: "o-1", Partes: []repertorio.Parte{parte("a", "IPI-1", "100")}},
			"o-2": {ObraID: "o-2", Partes: []repertorio.Parte{parte("a", "IPI-1", "60")}},
			"o-3": {ObraID: "o-3", Partes: []repertorio.Parte{parte("a", "IPI-1", "60"), parte("b", "", "40")}},
		},
		// o-4 no tiene ninguna declaracion: el motor la retiene igual (R-04).
		catalogo: []Obra{{ID: "o-1"}, {ID: "o-2"}, {ID: "o-3"}, {ID: "o-4"}},
	}
	n, err := Tablero{Repo: repo}.ObrasEnReserva(t.Context())
	if err != nil || n != 3 {
		t.Fatalf("n = %d, err = %v, se esperaban 3", n, err)
	}
	if repo.paginacion.Limite != LimiteSinTope {
		t.Fatalf("paginacion = %+v, se esperaba el censo entero", repo.paginacion)
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
