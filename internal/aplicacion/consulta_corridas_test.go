package aplicacion

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func repoConCorridas(vs ...ProcesoVista) *repositorioProcesosFalso {
	r := nuevoRepositorioProcesosFalso()
	for _, v := range vs {
		r.procesos[v.ID] = v
	}
	return r
}

var staffDistribucion = Usuario{ID: "usr-dist", Rol: RolDistribucion}

func TestEstadoCorridaNacionalEnVerificacionNombraLasFirmasQueFaltan(t *testing.T) {
	repo := repoConCorridas(ProcesoVista{
		ID: "proc-1", Circuito: reparto.Nacional, Etapa: reparto.EtapaVerificacion, Periodo: "2026-01", Revision: 2,
		Firmas: []reparto.Firma{{Rol: "distribucion", ActorID: "a", SobreRev: 2}, {Rol: "contabilidad", ActorID: "b", SobreRev: 1}},
	})
	e, err := ConsultarEstadoCorrida{Procesos: repo}.PorID(context.Background(), staffDistribucion, "proc-1")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if e.Circuito != "nacional" || e.Etapa != "verificacion" || e.Paso != 6 || e.TotalPasos != 9 {
		t.Fatalf("estado = %+v, se esperaba nacional, verificacion, paso 6 de 9", e)
	}
	if !e.Compuerta || !slices.Equal(e.FirmasFaltantes, []string{"contabilidad"}) {
		t.Fatalf("compuerta=%v faltantes=%v: la firma de contabilidad es de otra revision y no cuenta", e.Compuerta, e.FirmasFaltantes)
	}
	if e.SiguienteEtapa != "liquidacion_final" || !strings.Contains(e.Pendiente, "contabilidad") {
		t.Fatalf("siguiente=%q pendiente=%q", e.SiguienteEtapa, e.Pendiente)
	}
	if e.EtapaExplicada == "" || e.CircuitoExplicado == "" {
		t.Fatalf("faltan las explicaciones en lenguaje llano: %+v", e)
	}
}

func TestEstadoCorridaInternacionalTieneSuPropioRecorrido(t *testing.T) {
	repo := repoConCorridas(ProcesoVista{ID: "proc-x", Circuito: reparto.Internacional, Etapa: reparto.EtapaPagoRegistro, Periodo: "2026", Revision: 3})
	e, err := ConsultarEstadoCorrida{Procesos: repo}.PorID(context.Background(), staffDistribucion, "proc-x")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if e.Circuito != "internacional" || e.Paso != 6 || e.TotalPasos != 8 || e.SiguienteEtapa != "fees_in_error" {
		t.Fatalf("estado = %+v, se esperaba internacional paso 6 de 8 y luego fees_in_error", e)
	}
	if !slices.Equal(e.FirmasFaltantes, []string{"distribucion", "contabilidad"}) {
		t.Fatalf("faltantes = %v", e.FirmasFaltantes)
	}
}

func TestEstadoCorridaConFirmasCompletasEsperaAlAdministrador(t *testing.T) {
	repo := repoConCorridas(ProcesoVista{
		ID: "proc-1", Circuito: reparto.Nacional, Etapa: reparto.EtapaPagoRegistro, Revision: 4, RechazoMotivo: "cifras de enero",
		Firmas: []reparto.Firma{{Rol: "distribucion", ActorID: "a", SobreRev: 4}, {Rol: "contabilidad", ActorID: "b", SobreRev: 4}},
	})
	e, _ := ConsultarEstadoCorrida{Procesos: repo}.PorID(context.Background(), staffDistribucion, "proc-1")
	if len(e.FirmasFaltantes) != 0 || !strings.Contains(e.Pendiente, "administrador") {
		t.Fatalf("faltantes=%v pendiente=%q", e.FirmasFaltantes, e.Pendiente)
	}
	if e.UltimoRechazo != "cifras de enero" {
		t.Fatalf("ultimo rechazo = %q", e.UltimoRechazo)
	}
}

func TestEstadoCorridaEnAuditoriaNoTieneSiguiente(t *testing.T) {
	repo := repoConCorridas(ProcesoVista{ID: "proc-1", Circuito: reparto.Nacional, Etapa: reparto.EtapaAuditoria})
	e, _ := ConsultarEstadoCorrida{Procesos: repo}.PorID(context.Background(), staffDistribucion, "proc-1")
	if e.SiguienteEtapa != "" || e.Paso != e.TotalPasos {
		t.Fatalf("estado = %+v: auditoria es la ultima etapa", e)
	}
}

func TestEstadoCorridaEnDeduccionesAvisaDeLaCompuertaDeAnomalias(t *testing.T) {
	repo := repoConCorridas(ProcesoVista{ID: "proc-1", Circuito: reparto.Nacional, Etapa: reparto.EtapaDeducciones})
	e, _ := ConsultarEstadoCorrida{Procesos: repo}.PorID(context.Background(), staffDistribucion, "proc-1")
	if !strings.Contains(e.Pendiente, "anomal") {
		t.Fatalf("pendiente = %q: salir de deducciones pasa por la compuerta de anomalias", e.Pendiente)
	}
}

func TestEstadoCorridaNoEncontradaSeVeComoTal(t *testing.T) {
	_, err := ConsultarEstadoCorrida{Procesos: repoConCorridas()}.PorID(context.Background(), staffDistribucion, "nada")
	if !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

func TestEstadoCorridaSoloStaff(t *testing.T) {
	repo := repoConCorridas(ProcesoVista{ID: "proc-1", Circuito: reparto.Nacional, Etapa: reparto.EtapaRecaudo, Periodo: "2026-01"})
	uc := ConsultarEstadoCorrida{Procesos: repo}
	for _, actor := range []Usuario{
		{ID: "usr-tit", Rol: RolTitular, TitularID: "tit-1"},
		{ID: "usr-x", Rol: "gerente"},
		{},
	} {
		if _, err := uc.PorID(context.Background(), actor, "proc-1"); !errors.Is(err, ErrNoAutorizado) {
			t.Errorf("PorID con rol %q: err = %v, se esperaba ErrNoAutorizado", actor.Rol, err)
		}
		if _, err := uc.DePeriodo(context.Background(), actor, ""); !errors.Is(err, ErrNoAutorizado) {
			t.Errorf("DePeriodo con rol %q: err = %v, se esperaba ErrNoAutorizado", actor.Rol, err)
		}
	}
	for _, rol := range []Rol{RolAdministrador, RolDistribucion, RolContabilidad, RolAuditor} {
		if _, err := uc.PorID(context.Background(), Usuario{ID: "u", Rol: rol}, "proc-1"); err != nil {
			t.Errorf("rol %q: %v", rol, err)
		}
	}
}

func TestEstadoCorridaNoAutorizadoNoLeeElRepositorio(t *testing.T) {
	repo := repoConCorridas()
	repo.errPorID = errors.New("no deberia leerse")
	_, err := ConsultarEstadoCorrida{Procesos: repo}.PorID(context.Background(), Usuario{ID: "t", Rol: RolTitular}, "proc-1")
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("err = %v: el RBAC va antes de la lectura", err)
	}
}

func TestCorridasDePeriodoFiltraYOrdena(t *testing.T) {
	repo := repoConCorridas(
		ProcesoVista{ID: "proc-b", Circuito: reparto.Nacional, Etapa: reparto.EtapaRecaudo, Periodo: "2026-02"},
		ProcesoVista{ID: "proc-a", Circuito: reparto.Internacional, Etapa: reparto.EtapaRecaudo, Periodo: "2026-02"},
		ProcesoVista{ID: "proc-c", Circuito: reparto.Nacional, Etapa: reparto.EtapaRecaudo, Periodo: "2026-01"},
		ProcesoVista{ID: "proc-d", Circuito: reparto.Nacional, Etapa: reparto.EtapaRecaudo, Periodo: "2025-12"},
	)
	uc := ConsultarEstadoCorrida{Procesos: repo}
	ids := func(es []EstadoCorrida) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.ProcesoID)
		}
		return out
	}
	casos := []struct {
		periodo string
		quiere  []string
	}{
		{"2026-02", []string{"proc-a", "proc-b"}},
		{"2026", []string{"proc-a", "proc-b", "proc-c"}},
		{"", []string{"proc-a", "proc-b", "proc-c", "proc-d"}},
		{"2024-01", nil},
	}
	for _, c := range casos {
		es, err := uc.DePeriodo(context.Background(), staffDistribucion, c.periodo)
		if err != nil {
			t.Fatalf("%q: %v", c.periodo, err)
		}
		if got := ids(es); !slices.Equal(got, c.quiere) {
			t.Errorf("periodo %q: %v, se esperaba %v", c.periodo, got, c.quiere)
		}
	}
}

func TestCorridasDePeriodoRechazaUnPeriodoMalFormado(t *testing.T) {
	_, err := ConsultarEstadoCorrida{Procesos: repoConCorridas()}.DePeriodo(context.Background(), staffDistribucion, "enero")
	if !errors.Is(err, ErrPeriodoInvalido) {
		t.Fatalf("err = %v, se esperaba ErrPeriodoInvalido", err)
	}
}
