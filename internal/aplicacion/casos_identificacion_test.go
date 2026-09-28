package aplicacion

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

type casosFalsos struct {
	pagina   PaginaCasos
	err      error
	consulta ConsultaCasos
	llamadas int
}

func (c *casosFalsos) ListarCasosIdentificacion(_ context.Context, q ConsultaCasos) (PaginaCasos, error) {
	c.llamadas++
	c.consulta = q
	return c.pagina, c.err
}

func TestListarCasosTraduceElEstadoAEscalones(t *testing.T) {
	casos := []struct {
		estado string
		quiere []string
	}{
		{"", []string{identificacion.EscalonONI, identificacion.EscalonManual, identificacion.EscalonDescartado}},
		{EstadoCasoPendiente, []string{identificacion.EscalonONI}},
		{EstadoCasoAsignado, []string{identificacion.EscalonManual}},
		{EstadoCasoDescartado, []string{identificacion.EscalonDescartado}},
	}
	for _, c := range casos {
		t.Run(c.estado, func(t *testing.T) {
			repo := &casosFalsos{}
			_, err := CasosIdentificacion{Repo: repo}.Listar(context.Background(), FiltroCasos{Estado: c.estado})
			if err != nil {
				t.Fatalf("Listar: %v", err)
			}
			if !reflect.DeepEqual(repo.consulta.Escalones, c.quiere) {
				t.Fatalf("escalones = %v, quiere %v", repo.consulta.Escalones, c.quiere)
			}
		})
	}
}

func TestListarCasosPasaFiltrosYPaginacionConDefecto(t *testing.T) {
	repo := &casosFalsos{}
	_, err := CasosIdentificacion{Repo: repo}.Listar(context.Background(),
		FiltroCasos{Fuente: " caracol ", Periodo: " 2025-01 "})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if repo.consulta.Fuente != "caracol" || repo.consulta.Periodo != "2025-01" {
		t.Fatalf("consulta = %+v", repo.consulta)
	}
	if repo.consulta.Limite != LimiteObrasPorDefecto {
		t.Fatalf("limite = %d, quiere %d", repo.consulta.Limite, LimiteObrasPorDefecto)
	}
}

func TestListarCasosRechazaFiltrosMalformadosSinConsultar(t *testing.T) {
	for _, f := range []FiltroCasos{
		{Estado: "resuelto"},
		{Periodo: "2025-13"},
		{Periodo: "2025-00"},
		{Periodo: "enero"},
	} {
		repo := &casosFalsos{}
		_, err := CasosIdentificacion{Repo: repo}.Listar(context.Background(), f)
		if !errors.Is(err, ErrFiltroCasosInvalido) {
			t.Fatalf("%+v: err = %v, quiere ErrFiltroCasosInvalido", f, err)
		}
		if repo.llamadas != 0 {
			t.Fatalf("%+v: consulto el repositorio con un filtro invalido", f)
		}
	}
}

func TestListarCasosDaEstadoYUltimaActualizacion(t *testing.T) {
	creado := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	resuelto := time.Date(2025, 2, 3, 9, 0, 0, 0, time.UTC)
	repo := &casosFalsos{pagina: PaginaCasos{
		Pendientes: 1,
		Casos: []CasoIdentificacion{
			{UsoID: "u1", Escalon: identificacion.EscalonONI, ReporteCreado: creado,
				Candidatos: []CandidatoCaso{
					{ObraID: "o2", Puntaje: decimal.RequireFromString("0.61")},
					{ObraID: "o1", Puntaje: decimal.RequireFromString("0.55")},
				}},
			{UsoID: "u2", Escalon: identificacion.EscalonManual, ReporteCreado: creado,
				ResueltoEn: &resuelto, ObraAsignada: &ObraAsignada{ID: "o1"},
				ResueltoPor: &Resolutor{ID: "usr-1", Nombre: "Admin"},
				Nota:        "coincide la ficha"},
			// Un descartado no tiene obra y si tiene fecha: la actualizacion
			// del caso es cuando una persona lo decidio.
			{UsoID: "u3", Escalon: identificacion.EscalonDescartado, ReporteCreado: creado,
				ResueltoEn: &resuelto, ResueltoPor: &Resolutor{ID: "usr-1", Nombre: "Admin"},
				Nota: "no es del repertorio"},
		},
	}}
	pag, err := CasosIdentificacion{Repo: repo}.Listar(context.Background(), FiltroCasos{})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if pag.Pendientes != 1 || len(pag.Casos) != 3 {
		t.Fatalf("pagina = %+v", pag)
	}
	p, a, d := pag.Casos[0], pag.Casos[1], pag.Casos[2]
	if p.Estado != EstadoCasoPendiente || !p.UltimaActualizacion.Equal(creado) {
		t.Fatalf("pendiente = %s %v", p.Estado, p.UltimaActualizacion)
	}
	if p.Candidatos[0].ObraID != "o2" {
		t.Fatalf("reordeno los candidatos: %+v", p.Candidatos)
	}
	if a.Estado != EstadoCasoAsignado || !a.UltimaActualizacion.Equal(resuelto) || a.Nota != "coincide la ficha" {
		t.Fatalf("asignado = %s %v %q", a.Estado, a.UltimaActualizacion, a.Nota)
	}
	if d.Estado != EstadoCasoDescartado || !d.UltimaActualizacion.Equal(resuelto) || d.Nota != "no es del repertorio" {
		t.Fatalf("descartado = %s %v %q", d.Estado, d.UltimaActualizacion, d.Nota)
	}
	if d.ObraAsignada != nil {
		t.Fatalf("un descartado no tiene obra: %+v", d.ObraAsignada)
	}
}

// Un descarte sin fecha no es un descarte: la fila quedo a medias y decir que
// esta resuelta afirmaria algo falso.
func TestListarCasosDescartadoSinResueltoEnFalla(t *testing.T) {
	repo := &casosFalsos{pagina: PaginaCasos{Casos: []CasoIdentificacion{
		{UsoID: "u1", Escalon: identificacion.EscalonDescartado},
	}}}
	if _, err := (CasosIdentificacion{Repo: repo}).Listar(context.Background(), FiltroCasos{}); err == nil {
		t.Fatal("un descartado sin resuelto_en tenia que fallar")
	}
}

func TestListarCasosSinCandidatosDaListaVacia(t *testing.T) {
	repo := &casosFalsos{pagina: PaginaCasos{Casos: []CasoIdentificacion{
		{UsoID: "u1", Escalon: identificacion.EscalonONI},
	}}}
	pag, err := CasosIdentificacion{Repo: repo}.Listar(context.Background(), FiltroCasos{})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if pag.Casos[0].Candidatos == nil {
		t.Fatal("candidatos nil: tiene que ser una lista vacia")
	}
	if pag.Casos == nil {
		t.Fatal("casos nil")
	}
}

func TestListarCasosFallaCerradoConUnEscalonAjeno(t *testing.T) {
	repo := &casosFalsos{pagina: PaginaCasos{Casos: []CasoIdentificacion{
		{UsoID: "u1", Escalon: identificacion.EscalonDifuso},
	}}}
	if _, err := (CasosIdentificacion{Repo: repo}).Listar(context.Background(), FiltroCasos{}); err == nil {
		t.Fatal("un uso resuelto por la cascada no es un caso: tenia que fallar")
	}
}

func TestListarCasosPropagaElErrorDelRepositorio(t *testing.T) {
	boom := errors.New("boom")
	_, err := CasosIdentificacion{Repo: &casosFalsos{err: boom}}.Listar(context.Background(), FiltroCasos{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
