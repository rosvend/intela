package aplicacion

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

type repoExplicacion struct {
	valor              Explicacion
	err                error
	proceso, obra, tit string
	llamadas           int
}

func (r *repoExplicacion) PorLinea(_ context.Context, procesoID, obraID, titularID string) (Explicacion, error) {
	r.llamadas++
	r.proceso, r.obra, r.tit = procesoID, obraID, titularID
	return r.valor, r.err
}

func cifraAna() Explicacion {
	return Explicacion{
		Ref:       FormarRef("proc-2026-01", "obra-completa", "tit-ana"),
		TitularID: "tit-ana",
		Neto:      decimal.RequireFromString("4500.00"),
		Bruto:     decimal.RequireFromString("6000.00"),
		Corrida:   CorridaLinaje{ProcesoID: "proc-2026-01", Periodo: "2026-01", Circuito: "nacional"},
		Reporte:   ReporteLinaje{ID: "rpt-caracol-2026-01", Fuente: "caracol", SHA256: "aa"},
		Obra:      ObraLinaje{ID: "obra-completa", Titulo: "La Casa de las Dos Palmas", Escalon: "alias", Puntaje: decimal.RequireFromString("1.00000")},
		Regla:     ReglaLinaje{SnapshotID: "snap-2026-01", Reglamento: "RD-IX"},
		Split:     SplitLinaje{TitularID: "tit-ana", IPI: "IPI-00000001", Porcentaje: decimal.RequireFromString("60.0000"), Version: 1},
		Deducciones: []Deduccion{
			{Concepto: "gastos administrativos", Porcentaje: decimal.RequireFromString("10.00"), Monto: decimal.RequireFromString("600.00")},
		},
	}
}

func TestExplicarCifraTitularVeLaSuya(t *testing.T) {
	x := cifraAna()
	repo := &repoExplicacion{valor: x}
	e := ExplicarCifra{Repo: repo}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	got, err := e.Explicar(context.Background(), actor, x.Ref)
	if err != nil {
		t.Fatalf("Explicar: %v", err)
	}
	if repo.proceso != "proc-2026-01" || repo.obra != "obra-completa" || repo.tit != "tit-ana" {
		t.Fatalf("PorLinea recibia %s %s %s", repo.proceso, repo.obra, repo.tit)
	}
	if got.Neto.StringFixed(2) != "4500.00" {
		t.Fatalf("neto = %s", got.Neto)
	}
	if got.Bruto.StringFixed(2) != "6000.00" {
		t.Fatal("el bruto vive en la explicacion, no se puede perder")
	}
	if got.Regla.SnapshotID != "snap-2026-01" || got.Obra.Escalon != "alias" {
		t.Fatalf("linaje incompleto: %+v", got)
	}
}

func TestExplicarCifraTitularNoVeLaAjena(t *testing.T) {
	repo := &repoExplicacion{valor: Explicacion{TitularID: "tit-beto"}}
	e := ExplicarCifra{Repo: repo}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	_, err := e.Explicar(context.Background(), actor, FormarRef("proc-1", "obra-beto", "tit-beto"))
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("err = %v, se esperaba ErrNoAutorizado (403, no 404)", err)
	}
}

func TestExplicarCifraAuditorVeCifraAjena(t *testing.T) {
	x := cifraAna()
	x.TitularID = "tit-beto"
	repo := &repoExplicacion{valor: x}
	e := ExplicarCifra{Repo: repo}

	got, err := e.Explicar(context.Background(), Usuario{ID: "usr-rf", Rol: RolAuditor}, x.Ref)
	if err != nil {
		t.Fatalf("el auditor tiene que ver cualquier cifra: %v", err)
	}
	if got.TitularID != "tit-beto" {
		t.Fatalf("TitularID = %q", got.TitularID)
	}
}

func TestExplicarCifraNoEncontrada(t *testing.T) {
	e := ExplicarCifra{Repo: &repoExplicacion{err: ErrNoEncontrado}}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	_, err := e.Explicar(context.Background(), actor, FormarRef("proc-x", "obra-x", "tit-ana"))
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("err = %v", err)
	}
}

func TestExplicarCifraRefInvalidaNoConsulta(t *testing.T) {
	repo := &repoExplicacion{}
	e := ExplicarCifra{Repo: repo}
	actor := Usuario{ID: "usr-ana", Rol: RolTitular, TitularID: "tit-ana"}

	_, err := e.Explicar(context.Background(), actor, "no-es-una-ref")
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("err = %v", err)
	}
	if repo.llamadas != 0 {
		t.Fatal("una ref mal formada no tiene que ir a la base")
	}
}
