package aplicacion

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

var adminONI = Usuario{ID: "usr-adminONI", Rol: RolAdministrador}

func TestConsultarONILeeSoloLaColaONIConSusFiltros(t *testing.T) {
	creado := time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC)
	repo := &casosFalsos{pagina: PaginaCasos{Pendientes: 7, Casos: []CasoIdentificacion{{
		UsoID: "uso-1", Titulo: "La reina", TituloOriginal: "LA REINA", Fuente: "caracol", Modalidad: "television",
		ReporteID: "rep-1", Periodo: "2026-01", IDsFuente: "cap=12", Escalon: identificacion.EscalonONI, ReporteCreado: creado,
		Candidatos: []CandidatoCaso{{ObraID: "obra-1"}, {ObraID: "obra-2"}},
	}}}}
	cola, err := ConsultarONI{Casos: repo}.Ejecutar(context.Background(), adminONI, FiltroONI{Periodo: " 2026-01 ", Fuente: "caracol", Limite: 5})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	q := repo.consulta
	if !slices.Equal(q.Escalones, []string{identificacion.EscalonONI}) || q.Periodo != "2026-01" || q.Fuente != "caracol" || q.Limite != 5 {
		t.Fatalf("consulta = %+v: solo el escalon ONI, con periodo, fuente y limite", q)
	}
	if cola.Pendientes != 7 || len(cola.Obras) != 1 {
		t.Fatalf("cola = %+v", cola)
	}
	o := cola.Obras[0]
	if o.UsoID != "uso-1" || o.Titulo != "La reina" || o.Periodo != "2026-01" || o.Fuente != "caracol" || !o.DetectadaEn.Equal(creado) || o.Candidatos != 2 {
		t.Fatalf("obra = %+v", o)
	}
}

func TestConsultarONIPorDefectoPideVeinte(t *testing.T) {
	repo := &casosFalsos{}
	cola, err := ConsultarONI{Casos: repo}.Ejecutar(context.Background(), adminONI, FiltroONI{})
	if err != nil {
		t.Fatalf("Ejecutar: %v", err)
	}
	if repo.consulta.Limite != LimiteONIPorDefecto {
		t.Fatalf("limite = %d, se esperaba %d", repo.consulta.Limite, LimiteONIPorDefecto)
	}
	if cola.Obras == nil {
		t.Fatal("una cola vacia es [] y no null")
	}
}

func TestConsultarONIRechazaFiltrosMalFormados(t *testing.T) {
	uc := ConsultarONI{Casos: &casosFalsos{}}
	if _, err := uc.Ejecutar(context.Background(), adminONI, FiltroONI{Periodo: "2026/01"}); !errors.Is(err, ErrPeriodoInvalido) {
		t.Errorf("periodo: err = %v", err)
	}
	for _, l := range []int{-1, LimiteONIMaximo + 1} {
		if _, err := uc.Ejecutar(context.Background(), adminONI, FiltroONI{Limite: l}); !errors.Is(err, ErrFiltroInvalido) {
			t.Errorf("limite %d: err = %v", l, err)
		}
	}
}

func TestConsultarONIPropagaElErrorDelRepositorio(t *testing.T) {
	boom := errors.New("se cayo la base")
	_, err := ConsultarONI{Casos: &casosFalsos{err: boom}}.Ejecutar(context.Background(), adminONI, FiltroONI{})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestConsultarONISoloAdministrador(t *testing.T) {
	for _, actor := range []Usuario{
		{ID: "t", Rol: RolTitular, TitularID: "tit-1"},
		{ID: "d", Rol: RolDistribucion},
		{ID: "c", Rol: RolContabilidad},
		{ID: "a", Rol: RolAuditor},
		{},
	} {
		repo := &casosFalsos{}
		_, err := ConsultarONI{Casos: repo}.Ejecutar(context.Background(), actor, FiltroONI{})
		if !errors.Is(err, ErrNoAutorizado) {
			t.Errorf("rol %q: err = %v, se esperaba ErrNoAutorizado", actor.Rol, err)
		}
		if repo.llamadas != 0 {
			t.Errorf("rol %q: leyo el repositorio antes de autorizar", actor.Rol)
		}
	}
}
