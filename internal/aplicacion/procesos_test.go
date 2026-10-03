package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func snapshotDePrueba() reparto.Snapshot {
	return reparto.Snapshot{
		AdminPct:              d("20"),
		SocialPct:             d("10"),
		ReservaPct:            d("5"),
		PondCine:              d("5.0"),
		PondUnitario:          d("2.8"),
		PondSerie:             d("1.3"),
		PondSketch:            d("0.8"),
		Wa:                    d("0.5"),
		Wb:                    d("0.3"),
		Wc:                    d("0.2"),
		GrupoPrivadosPct:      d("50"),
		GrupoRegionalesPct:    d("20"),
		GrupoPremiumPct:       d("10"),
		GrupoLideresPct:       d("10"),
		GrupoEstandarPct:      d("10"),
		AsignacionTercerosPct: d("5"),
		BaseCineTeatro:        reparto.BaseEspectadores,
		Reglamento:            "RD-IX",
	}
}

type repositorioRecaudoFalso struct {
	bolsa         BolsaPersistida
	bolsasPeriodo []BolsaPersistida
	err           error
	pedidoPeriodo []string
}

func (r *repositorioRecaudoFalso) ListarBolsas(_ context.Context) ([]BolsaPersistida, error) {
	return nil, nil
}
func (r *repositorioRecaudoFalso) BolsasDePeriodo(_ context.Context, periodo string) ([]BolsaPersistida, error) {
	r.pedidoPeriodo = append(r.pedidoPeriodo, periodo)
	if r.err != nil {
		return nil, r.err
	}
	return r.bolsasPeriodo, nil
}
func (r *repositorioRecaudoFalso) BolsaPorID(_ context.Context, id string) (BolsaPersistida, error) {
	if r.err != nil {
		return BolsaPersistida{}, r.err
	}
	if r.bolsa.ID == id {
		return r.bolsa, nil
	}
	for _, b := range r.bolsasPeriodo {
		if b.ID == id {
			return b, nil
		}
	}
	return r.bolsa, nil
}
func (r *repositorioRecaudoFalso) ListarUsuarios(_ context.Context) ([]recaudo.Usuario, error) {
	return nil, nil
}

type gestionDeclaracionesFalsa struct {
	porObra map[string]VersionDeclaracion
}

func (g *gestionDeclaracionesFalsa) Guardar(_ context.Context, _ repertorio.Declaracion, _ time.Time, _ string) (int, time.Time, error) {
	return 0, time.Time{}, nil
}
func (g *gestionDeclaracionesFalsa) Historial(_ context.Context, _ string, _ Paginacion) ([]VersionDeclaracion, error) {
	return nil, nil
}
func (g *gestionDeclaracionesFalsa) VigenteEn(_ context.Context, _ string, _ time.Time) (VersionDeclaracion, error) {
	return VersionDeclaracion{}, nil
}
func (g *gestionDeclaracionesFalsa) VigentesDeObras(_ context.Context, _ []string) (map[string]VersionDeclaracion, error) {
	return g.porObra, nil
}

type usosDeRepartoFalso struct {
	usos     []UsoDeReparto
	resumen  ResumenUsosDeCanal
	sinCanal int
}

func (u *usosDeRepartoFalso) UsosDeCanal(_ context.Context, _, _ string, _ int) ([]UsoDeReparto, ResumenUsosDeCanal, error) {
	return u.usos, u.resumen, nil
}
func (u *usosDeRepartoFalso) UsosSinCanal(_ context.Context, _ string) (int, error) {
	return u.sinCanal, nil
}

type repositorioResultadosFalso struct {
	procesoID string
	guardado  reparto.Resultado
	err       error
}

func (r *repositorioResultadosFalso) GuardarResultado(_ context.Context, procesoID string, res reparto.Resultado) error {
	if r.err != nil {
		return r.err
	}
	r.procesoID = procesoID
	r.guardado = res
	return nil
}
func (r *repositorioResultadosFalso) ResultadoPorProceso(_ context.Context, _ string) (reparto.Resultado, error) {
	return r.guardado, nil
}

type repositorioProcesosFalso struct {
	procesos   map[string]ProcesoVista
	guardados  []ProcesoVista
	firmas     []reparto.Firma
	errGuardar error
	errPorID   error
	errFirmar  error
}

func nuevoRepositorioProcesosFalso() *repositorioProcesosFalso {
	return &repositorioProcesosFalso{procesos: map[string]ProcesoVista{}}
}

func (r *repositorioProcesosFalso) GuardarProceso(_ context.Context, p ProcesoVista, revisionAnterior int) error {
	if r.errGuardar != nil {
		return r.errGuardar
	}
	if existente, ok := r.procesos[p.ID]; ok && existente.Revision != revisionAnterior {
		return ErrProcesoConflictoDeConcurrencia
	}
	r.guardados = append(r.guardados, p)
	r.procesos[p.ID] = p
	return nil
}

func (r *repositorioProcesosFalso) ProcesoPorID(_ context.Context, id string) (ProcesoVista, error) {
	if r.errPorID != nil {
		return ProcesoVista{}, r.errPorID
	}
	p, ok := r.procesos[id]
	if !ok {
		return ProcesoVista{}, ErrNoEncontrado
	}
	return p, nil
}

func (r *repositorioProcesosFalso) ListarProcesos(_ context.Context) ([]ProcesoVista, error) {
	var todos []ProcesoVista
	for _, p := range r.procesos {
		todos = append(todos, p)
	}
	return todos, nil
}

func (r *repositorioProcesosFalso) GuardarFirma(_ context.Context, procesoID string, f reparto.Firma) error {
	if r.errFirmar != nil {
		return r.errFirmar
	}
	r.firmas = append(r.firmas, f)
	return nil
}

type parametrosNormativosFalso struct {
	id     string
	snap   reparto.Snapshot
	err    error
	pedido []time.Time
}

func (p *parametrosNormativosFalso) SnapshotEnFecha(_ context.Context, fecha time.Time) (string, reparto.Snapshot, error) {
	p.pedido = append(p.pedido, fecha)
	if p.err != nil {
		return "", reparto.Snapshot{}, p.err
	}
	return p.id, p.snap, nil
}

func (p *parametrosNormativosFalso) SnapshotPorID(_ context.Context, id string) (reparto.Snapshot, error) {
	return p.snap, nil
}

func (p *parametrosNormativosFalso) Vigentes(_ context.Context, ahora time.Time) ([]FilaParametro, error) {
	return nil, nil
}

func TestIniciarProcesoResuelveElSnapshotYAbreElProceso(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	params := &parametrosNormativosFalso{id: "snap-1", snap: reparto.Snapshot{Reglamento: "IX"}}
	bolsas := &repositorioRecaudoFalso{bolsa: BolsaPersistida{ID: "bolsa-1", Periodo: "2026-01", Circuito: reparto.Nacional}}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	v, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaRecaudo {
		t.Fatalf("etapa = %q, se esperaba %q", v.Etapa, reparto.EtapaRecaudo)
	}
	if v.SnapshotID != "snap-1" {
		t.Fatalf("snapshotID = %q, se esperaba %q", v.SnapshotID, "snap-1")
	}
	if v.Reglamento != "IX" {
		t.Fatalf("reglamento = %q, se esperaba el del snapshot congelado (%q)", v.Reglamento, "IX")
	}
	if len(repo.guardados) != 1 || repo.guardados[0].ID != "proc-1" {
		t.Fatalf("guardados = %v, se esperaba una sola escritura de proc-1", repo.guardados)
	}
	if len(params.pedido) != 1 || params.pedido[0].Format("2006-01") != "2026-01" {
		t.Fatalf("snapshot pedido contra %v, se esperaba el primer dia de 2026-01", params.pedido)
	}
}

// TestIniciarProcesoEsIdempotentePorID reproduce lo que un reintento de
// TrabajoEjecutarReparto haria sin esta guarda: reabrir un proceso que un
// humano ya avanzo lo reiniciaria a EtapaRecaudo revision 1, borrando el
// progreso. La clave natural del trabajo ya evita encolar dos veces, pero un
// reintento SI vuelve a tomar el mismo trabajo (ADR sobre Intentos vs
// Corrida), asi que IniciarProceso tiene que ser el que no repita el efecto.
func TestIniciarProcesoEsIdempotentePorID(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	ya, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-viejo", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	ya.Etapa = reparto.EtapaLiquidacionParcial
	ya.Revision = 3
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(ya), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	params := &parametrosNormativosFalso{id: "snap-nuevo"}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params})

	v, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaLiquidacionParcial || v.Revision != 3 {
		t.Fatalf("etapa/revision = %q/%d, se esperaba que el reintento NO reabriera el proceso: %+v", v.Etapa, v.Revision, v)
	}
	if v.SnapshotID != "snap-viejo" {
		t.Fatalf("snapshotID = %q, un reintento no debio volver a congelar el snapshot", v.SnapshotID)
	}
	if len(params.pedido) != 0 {
		t.Fatal("un proceso que ya existe no debio resolver un snapshot nuevo")
	}
}

func TestIniciarProcesoPropagaElErrorDeParametroAusente(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	params := &parametrosNormativosFalso{err: ErrParametroAusente}
	bolsas := &repositorioRecaudoFalso{bolsa: BolsaPersistida{ID: "bolsa-1", Periodo: "2026-01", Circuito: reparto.Nacional}}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	_, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1", "")
	if !errors.Is(err, ErrParametroAusente) {
		t.Fatalf("error = %v, se esperaba ErrParametroAusente", err)
	}
	if len(repo.guardados) != 0 {
		t.Fatal("no debio guardarse un proceso sin snapshot")
	}
}

func TestIniciarProcesoRechazaPeriodoInvalido(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	params := &parametrosNormativosFalso{id: "snap-1"}
	bolsas := &repositorioRecaudoFalso{bolsa: BolsaPersistida{ID: "bolsa-1", Periodo: "2026-01", Circuito: reparto.Nacional}}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	_, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-13", reparto.Nacional, "bolsa-1", "")
	if !errors.Is(err, reparto.ErrProcesoInvalido) {
		t.Fatalf("error = %v, se esperaba ErrProcesoInvalido: un periodo mal formado es un dato mal formado, no un 500", err)
	}
	if len(params.pedido) != 0 {
		t.Error("no debio resolverse un snapshot contra un periodo invalido")
	}
}

// TestIniciarProcesoRechazaSiElCircuitoNoCoincideConLaBolsa reproduce el
// hallazgo #2 de la revision de #159: sin esta comprobacion, una bolsa
// nacional se podia abrir como proceso internacional (o al reves) y
// valorizar con las reglas del circuito equivocado sin ningun error.
func TestIniciarProcesoRechazaSiElCircuitoNoCoincideConLaBolsa(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	params := &parametrosNormativosFalso{id: "snap-1"}
	bolsas := &repositorioRecaudoFalso{bolsa: BolsaPersistida{ID: "bolsa-1", Periodo: "2026-01", Circuito: reparto.Nacional}}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	_, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Internacional, "bolsa-1", "")
	if !errors.Is(err, ErrProcesoBolsaNoCoincide) {
		t.Fatalf("error = %v, se esperaba ErrProcesoBolsaNoCoincide", err)
	}
	if len(repo.guardados) != 0 {
		t.Fatal("no debio abrirse un proceso con el circuito de la bolsa equivocado")
	}
}

// TestIniciarProcesoRechazaSiElPeriodoNoCoincideConLaBolsa es el mismo
// hallazgo #2, para el periodo: valorizaria con los usos de un periodo que
// no son los de esa bolsa.
func TestIniciarProcesoRechazaSiElPeriodoNoCoincideConLaBolsa(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	params := &parametrosNormativosFalso{id: "snap-1"}
	bolsas := &repositorioRecaudoFalso{bolsa: BolsaPersistida{ID: "bolsa-1", Periodo: "2026-01", Circuito: reparto.Nacional}}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	_, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-02", reparto.Nacional, "bolsa-1", "")
	if !errors.Is(err, ErrProcesoBolsaNoCoincide) {
		t.Fatalf("error = %v, se esperaba ErrProcesoBolsaNoCoincide", err)
	}
	if len(repo.guardados) != 0 {
		t.Fatal("no debio abrirse un proceso con el periodo de la bolsa equivocado")
	}
}

// TestIniciarProcesoIDReutilizadoConDatosDistintosEsConflicto reproduce el
// hallazgo #3 de la revision de #159: reintentar POST /procesos con un id ya
// usado pero periodo/circuito/bolsa DISTINTOS no puede devolver el proceso
// existente en silencio -- el cliente no podria distinguir "se creo" de "se
// devolvio otra cosa con este mismo id".
func TestIniciarProcesoIDReutilizadoConDatosDistintosEsConflicto(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	ya, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(ya), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	params := &parametrosNormativosFalso{id: "snap-nuevo"}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params})

	_, err = uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-2", "")
	if !errors.Is(err, ErrProcesoIDReutilizado) {
		t.Fatalf("error = %v, se esperaba ErrProcesoIDReutilizado", err)
	}
	if len(params.pedido) != 0 {
		t.Fatal("no debio resolverse un snapshot nuevo para un id en conflicto")
	}
}

func procesoNacionalEnVerificacionGuardado(t *testing.T, repo *repositorioProcesosFalso) {
	t.Helper()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaVerificacion
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestFirmarCargaFirmaYPersisteElProceso(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	procesoNacionalEnVerificacionGuardado(t, repo)
	uc := conBitacora(Procesos{Repo: repo})

	v, err := uc.Firmar(t.Context(), "proc-1", reparto.RolDistribucion, "actor-dist")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(v.Firmas) != 1 {
		t.Fatalf("firmas = %v, se esperaba una", v.Firmas)
	}
	if len(repo.firmas) != 1 {
		t.Fatalf("GuardarFirma no se llamo: %v", repo.firmas)
	}
}

func TestFirmarPropagaElRechazoDelDominio(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	uc := conBitacora(Procesos{Repo: repo})

	_, err = uc.Firmar(t.Context(), "proc-1", reparto.RolDistribucion, "actor-dist")
	if !errors.Is(err, reparto.ErrRepartoInvalido) {
		t.Fatalf("error = %v, se esperaba ErrRepartoInvalido (recaudo no es compuerta)", err)
	}
	if len(repo.firmas) != 0 {
		t.Fatal("no debio guardarse una firma sobre un rechazo del dominio")
	}
}

func TestRechazarGatePersisteElRetroceso(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	procesoNacionalEnVerificacionGuardado(t, repo)
	uc := conBitacora(Procesos{Repo: repo})

	v, err := uc.RechazarGate(t.Context(), "proc-1", "faltan soportes", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatalf("etapa = %q, se esperaba retroceder a liquidacion_parcial", v.Etapa)
	}
	if repo.procesos["proc-1"].Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatal("el retroceso no se persistio")
	}
}

// TestAvanzarEtapaRechazaConflictoDeConcurrencia reproduce el hallazgo #4 de
// la revision de #159: dos transiciones concurrentes sobre el mismo proceso
// (aqui, un RechazarGate que se cuela entre la lectura y la escritura de un
// AvanzarEtapa) no pueden pisarse -- la segunda en escribir tiene que fallar
// y obligar a releer, no sobreescribir a ciegas.
func TestAvanzarEtapaRechazaConflictoDeConcurrencia(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaVerificacion
	p, err = p.Firmar(reparto.RolDistribucion, "actor-dist")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err = p.Firmar(reparto.RolContabilidad, "actor-conta")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	uc := conBitacora(Procesos{Repo: repo})

	// Otro actor rechaza la compuerta DESPUES de que este AvanzarEtapa ya
	// leyo el proceso (revision 1) pero ANTES de que escriba: la revision en
	// la base subio a 2 por debajo de sus pies.
	if _, err := uc.RechazarGate(t.Context(), "proc-1", "faltan soportes", ""); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	// Simula que AvanzarEtapa ya habia leido la revision 1 antes del
	// rechazo: fuerza la escritura contra esa revision vieja.
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 1); !errors.Is(err, ErrProcesoConflictoDeConcurrencia) {
		t.Fatalf("error = %v, se esperaba ErrProcesoConflictoDeConcurrencia", err)
	}

	// Y el rechazo del otro actor sigue en pie: no lo piso.
	leido, err := uc.ConsultarEstadoProceso(t.Context(), "proc-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if leido.Etapa != reparto.EtapaLiquidacionParcial || leido.RechazoMotivo != "faltan soportes" {
		t.Fatalf("el rechazo del otro actor no debio perderse: %+v", leido)
	}
}

func TestAvanzarEtapaFueraDeValorizacionSoloPersisteLaEtapa(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	resultados := &repositorioResultadosFalso{}
	uc := conBitacora(Procesos{Repo: repo, Resultados: resultados})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaDeducciones {
		t.Fatalf("etapa = %q, se esperaba %q", v.Etapa, reparto.EtapaDeducciones)
	}
	if resultados.procesoID != "" {
		t.Fatal("no debio invocarse el motor fuera de la valorizacion")
	}
}

func TestAvanzarEtapaNacionalValorizaAlEntrarAImporteObra(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaDeducciones
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	decl, err := repertorio.NuevaDeclaracion("obra-1", []repertorio.Parte{
		{TitularID: "titular-1", IPI: "IPI-1", Porcentaje: d("100")},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	uc := conBitacora(Procesos{
		Repo:       repo,
		Parametros: &parametrosNormativosFalso{snap: snapshotDePrueba()},
		Bolsas: &repositorioRecaudoFalso{bolsa: BolsaPersistida{
			ID: "bolsa-1", UsuarioID: "z", Periodo: "2026-01", Circuito: recaudo.Nacional, Bruto: d("1000000"),
		}},
		Declaraciones: &gestionDeclaracionesFalsa{porObra: map[string]VersionDeclaracion{
			"obra-1": {Declaracion: decl},
		}},
		Usos:       &usosDeRepartoFalso{usos: []UsoDeReparto{usoDeCanal("z", reparto.TV, "")}},
		Resultados: &repositorioResultadosFalso{},
		Unidad:     &unidadFalsa{},
		Anomalias:  &compuertaFalsa{},
	})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaImporteObra {
		t.Fatalf("etapa = %q, se esperaba %q", v.Etapa, reparto.EtapaImporteObra)
	}
	resultados := uc.Resultados.(*repositorioResultadosFalso)
	if resultados.procesoID != "proc-1" {
		t.Fatal("el motor no se invoco al entrar a importe_obra (RD 13.5)")
	}
	if resultados.guardado.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID del resultado = %q, se esperaba el snapshot congelado del proceso", resultados.guardado.SnapshotID)
	}
}

// procesoNacionalListoParaValorizar deja proc-1 en deducciones contra una
// bolsa del usuario "procinal", con obra-1 declarada al 100%.
func procesoNacionalListoParaValorizar(t *testing.T, snap reparto.Snapshot, usos *usosDeRepartoFalso) (Procesos, *repositorioProcesosFalso) {
	t.Helper()
	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2025-01", reparto.Nacional, "bolsa-procinal", "snp1-viejo", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaDeducciones
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	decl, err := repertorio.NuevaDeclaracion("obra-1", []repertorio.Parte{
		{TitularID: "titular-1", IPI: "IPI-1", Porcentaje: d("100")},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	uc := conBitacora(Procesos{
		Repo:       repo,
		Parametros: &parametrosNormativosFalso{snap: snap},
		Bolsas: &repositorioRecaudoFalso{bolsa: BolsaPersistida{
			ID: "bolsa-procinal", UsuarioID: "procinal", Periodo: "2025-01", Circuito: recaudo.Nacional, Bruto: d("1000000"),
		}},
		Declaraciones: &gestionDeclaracionesFalsa{porObra: map[string]VersionDeclaracion{"obra-1": {Declaracion: decl}}},
		Usos:          usos,
		Resultados:    &repositorioResultadosFalso{},
	})
	return uc, repo
}

// TestAvanzarEtapaBolsaSinUsosEsErrorTipadoQueDiceQueHacer es el criterio de
// #194: una bolsa sin usos que la ponderen responde con un error que nombra
// la bolsa, el canal y el periodo, y distingue "falta el reporte" de "faltan
// identificar las filas". El motor ya rechazaba la lista vacia, pero con "no
// hay usos" a secas.
func TestAvanzarEtapaBolsaSinUsosEsErrorTipadoQueDiceQueHacer(t *testing.T) {
	t.Parallel()

	casos := []struct {
		nombre     string
		resumen    ResumenUsosDeCanal
		sinCanal   int
		fragmentos []string
		noDice     string
	}{
		{"ninguna fila del canal", ResumenUsosDeCanal{}, 0,
			[]string{"cargue el reporte", "si ya esta cargado, corrija el canal_id"}, "cola"},
		{"reporte cargado con usos sin canal", ResumenUsosDeCanal{}, 3,
			[]string{"3 usos del periodo no traen canal", "corrija la atribucion del canal"}, "cargue el reporte"},
		{"filas del canal sin identificar", ResumenUsosDeCanal{Pendientes: 1, ONI: 2}, 0,
			[]string{"1 pendientes, 2 ONI", "cola de identificacion"}, "cargue el reporte"},
		{"filas del canal solo excluidas o descartadas", ResumenUsosDeCanal{Excluidos: 5, Descartados: 1}, 0,
			[]string{"5 excluidos", "1 descartados", "R-27", "ninguno pondera"}, "identifique"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			t.Parallel()
			uc, repo := procesoNacionalListoParaValorizar(t, snapshotDePrueba(),
				&usosDeRepartoFalso{resumen: c.resumen, sinCanal: c.sinCanal})

			_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
			if !errors.Is(err, ErrBolsaSinUsos) {
				t.Fatalf("se esperaba ErrBolsaSinUsos, dio: %v", err)
			}
			var sinUsos *ErrorBolsaSinUsos
			if !errors.As(err, &sinUsos) {
				t.Fatalf("se esperaba *ErrorBolsaSinUsos, dio: %T", err)
			}
			if sinUsos.BolsaID != "bolsa-procinal" || sinUsos.CanalID != "procinal" || sinUsos.Periodo != "2025-01" {
				t.Errorf("error = %+v, se esperaba bolsa-procinal/procinal/2025-01", *sinUsos)
			}
			for _, f := range append([]string{`"bolsa-procinal"`, `"procinal"`, "2025-01"}, c.fragmentos...) {
				if !strings.Contains(err.Error(), f) {
					t.Errorf("el mensaje %q no dice %s", err, f)
				}
			}
			if strings.Contains(err.Error(), c.noDice) {
				t.Errorf("el mensaje %q no deberia decir %s", err, c.noDice)
			}
			if sinUsos.UsosSinCanal != c.sinCanal {
				t.Errorf("UsosSinCanal = %d, se esperaba %d", sinUsos.UsosSinCanal, c.sinCanal)
			}
			// La corrida no sale de deducciones y no persiste un resultado vacio.
			v, _ := repo.ProcesoPorID(t.Context(), "proc-1")
			if v.Etapa != reparto.EtapaDeducciones {
				t.Errorf("etapa = %q, la corrida no puede avanzar sin usos", v.Etapa)
			}
			if uc.Resultados.(*repositorioResultadosFalso).procesoID != "" {
				t.Error("se guardo un resultado sin usos")
			}
		})
	}
}

// Una corrida de cine abierta con un snapshot que no trae la base de P-18 -la
// de Procinal de #194, congelada con la version 1 de las clausulas- no se
// arregla cargando la fila: el snapshot no se vuelve a resolver (ADR 0005).
// El error lo dice, en vez de sugerir un reintento que fallaria igual.
func TestAvanzarEtapaConSnapshotSinBaseCineDiceQueAbraOtraCorrida(t *testing.T) {
	t.Parallel()

	snap := snapshotDePrueba()
	snap.BaseCineTeatro = ""
	uc, _ := procesoNacionalListoParaValorizar(t, snap,
		&usosDeRepartoFalso{usos: []UsoDeReparto{usoDeCanal("procinal", reparto.Cine, "")}})

	_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("se esperaba reparto.ErrParametroAusente, dio: %v", err)
	}
	for _, f := range []string{"base_cine_teatro", "cine_teatro.base", `"snp1-viejo"`, "abra una corrida nueva", `"bolsa-procinal"`} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("el mensaje %q no dice %s", err, f)
		}
	}
}

// Con la base en el snapshot, la corrida de cine valoriza: es el otro lado de
// la prueba de arriba, sin Postgres.
func TestAvanzarEtapaValorizaCineConLaBaseDelSnapshot(t *testing.T) {
	t.Parallel()

	snap := snapshotDePrueba()
	snap.BaseCineTeatro = reparto.BaseTaquilla
	uc, _ := procesoNacionalListoParaValorizar(t, snap,
		&usosDeRepartoFalso{usos: []UsoDeReparto{usoDeCanal("procinal", reparto.Cine, "")}})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaImporteObra {
		t.Fatalf("etapa = %q, se esperaba importe_obra", v.Etapa)
	}
	res := uc.Resultados.(*repositorioResultadosFalso).guardado
	if len(res.Obras) != 1 || res.Obras[0].ObraID != "obra-1" {
		t.Fatalf("resultado.Obras = %+v, se esperaba una linea de obra-1", res.Obras)
	}
}

func TestAvanzarEtapaNacionalSinUnidadFallaClaro(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaDeducciones
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	uc := Procesos{Repo: repo, Anomalias: &compuertaFalsa{}} // sin Unidad

	_, err = uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if err == nil {
		t.Fatal("se esperaba un error de cableado, no un panico ni una valorizacion sin atomicidad")
	}
}

func TestAvanzarEtapaInternacionalNuncaValoriza(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Internacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaDeducciones
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	resultados := &repositorioResultadosFalso{}
	uc := conBitacora(Procesos{Repo: repo, Resultados: resultados, Anomalias: &compuertaFalsa{}})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatalf("etapa = %q, el internacional no valoriza por puntos (RD 7.4)", v.Etapa)
	}
	if resultados.procesoID != "" {
		t.Fatal("el internacional nunca debe invocar el motor de valorizacion")
	}
}

func TestAbrirCorridaDelPeriodoAbreUnProcesoPorBolsa(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	bolsas := &repositorioRecaudoFalso{bolsasPeriodo: []BolsaPersistida{
		{ID: "bolsa-1", UsuarioID: "z", Periodo: "2026-01", Circuito: recaudo.Nacional, Bruto: d("1000")},
		{ID: "bolsa-2", UsuarioID: "w", Periodo: "2026-01", Circuito: recaudo.Internacional, Bruto: d("500")},
	}}
	params := &parametrosNormativosFalso{id: "snap-1"}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	if err := uc.AbrirCorridaDelPeriodo(t.Context(), "2026-01", 1); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	lista, err := repo.ListarProcesos(t.Context())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("se esperaba un proceso por bolsa (ADR 0019), hubo %d: %+v", len(lista), lista)
	}
	porBolsa := map[string]ProcesoVista{}
	for _, p := range lista {
		porBolsa[p.BolsaID] = p
	}
	if porBolsa["bolsa-1"].Circuito != reparto.Nacional || porBolsa["bolsa-2"].Circuito != reparto.Internacional {
		t.Fatalf("circuito no coincide con el de su bolsa: %+v", porBolsa)
	}
}

func TestAbrirCorridaDelPeriodoEsIdempotenteReintentandoElMismoTrabajo(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	bolsas := &repositorioRecaudoFalso{bolsasPeriodo: []BolsaPersistida{
		{ID: "bolsa-1", UsuarioID: "z", Periodo: "2026-01", Circuito: recaudo.Nacional, Bruto: d("1000")},
	}}
	params := &parametrosNormativosFalso{id: "snap-1"}
	uc := conBitacora(Procesos{Repo: repo, Parametros: params, Bolsas: bolsas})

	if err := uc.AbrirCorridaDelPeriodo(t.Context(), "2026-01", 1); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// El proceso avanza por accion humana entre reintentos del trabajo.
	if err := repo.GuardarProceso(t.Context(), ProcesoVista{
		ID: repo.guardados[0].ID, Circuito: reparto.Nacional, Etapa: reparto.EtapaLiquidacionParcial,
		Periodo: "2026-01", BolsaID: "bolsa-1", SnapshotID: "snap-1", Revision: 1,
	}, 1); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if err := uc.AbrirCorridaDelPeriodo(t.Context(), "2026-01", 1); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	leido, err := repo.ProcesoPorID(t.Context(), repo.guardados[0].ID)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if leido.Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatalf("etapa = %q, el reintento del trabajo no debio reabrir el proceso ya avanzado", leido.Etapa)
	}
}

func TestConsultarEstadoProcesoDelegaAlRepositorio(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	procesoNacionalEnVerificacionGuardado(t, repo)
	uc := conBitacora(Procesos{Repo: repo})

	v, err := uc.ConsultarEstadoProceso(t.Context(), "proc-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.ID != "proc-1" {
		t.Fatalf("ID = %q, se esperaba proc-1", v.ID)
	}
}

func TestListarProcesosDelegaAlRepositorio(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	procesoNacionalEnVerificacionGuardado(t, repo)
	uc := conBitacora(Procesos{Repo: repo})

	lista, err := uc.ListarProcesos(t.Context())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("lista = %v, se esperaba un proceso", lista)
	}
}

// compuertaFalsa es una CompuertaAnomalias con respuesta fija.
type compuertaFalsa struct {
	criticas int
	err      error
	pedidos  []string
}

func (c *compuertaFalsa) Bloqueantes(_ context.Context, periodo string) (int, error) {
	c.pedidos = append(c.pedidos, periodo)
	return c.criticas, c.err
}

func procesoEnDeducciones(t *testing.T, circuito reparto.Circuito) *repositorioProcesosFalso {
	t.Helper()
	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", circuito, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaDeducciones
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return repo
}

func TestAvanzarEtapaConCriticasAbiertasNoSaleDeDeducciones(t *testing.T) {
	t.Parallel()

	for _, circuito := range []reparto.Circuito{reparto.Nacional, reparto.Internacional} {
		repo := procesoEnDeducciones(t, circuito)
		resultados := &repositorioResultadosFalso{}
		compuerta := &compuertaFalsa{criticas: 2}
		unidad := &unidadFalsa{}
		bitacora := &bitacoraFalsa{}
		uc := conBitacora(Procesos{
			Repo: repo, Resultados: resultados, Unidad: unidad, Bitacora: bitacora, Anomalias: compuerta,
		})

		_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
		if !errors.Is(err, ErrAnomaliasCriticasAbiertas) {
			t.Fatalf("%s: err = %v, se esperaba ErrAnomaliasCriticasAbiertas", circuito, err)
		}
		if len(compuerta.pedidos) != 1 || compuerta.pedidos[0] != "2026-01" {
			t.Fatalf("%s: la compuerta se consulto con %v, se esperaba [2026-01]", circuito, compuerta.pedidos)
		}
		if got := repo.procesos["proc-1"].Etapa; got != reparto.EtapaDeducciones {
			t.Fatalf("%s: etapa = %q, el proceso no debio moverse", circuito, got)
		}
		if resultados.procesoID != "" {
			t.Fatalf("%s: se valorizo con criticas abiertas", circuito)
		}
		// El nacional evalua DENTRO de la unidad y la confirma: revertirla
		// borraria las alertas y el 409 apuntaria a una bandeja vacia (#166).
		if circuito == reparto.Nacional {
			if unidad.entradas != 1 || !unidad.confirmo {
				t.Fatalf("nacional: entradas=%d confirmo=%v, la evaluacion tiene que confirmarse", unidad.entradas, unidad.confirmo)
			}
		} else if unidad.entradas != 0 {
			t.Fatalf("internacional: entradas=%d, la compuerta no abre unidad", unidad.entradas)
		}
		if len(bitacora.asientos) != 0 {
			t.Fatalf("%s: se asentaron %d hechos con la compuerta cerrada", circuito, len(bitacora.asientos))
		}
	}
}

func TestAvanzarEtapaSinCompuertaFallaCerrada(t *testing.T) {
	t.Parallel()

	repo := procesoEnDeducciones(t, reparto.Internacional)
	// El resto del cableado completo: sin bitacora la transicion fallaria igual y esto no probaria la compuerta.
	uc := conBitacora(Procesos{Repo: repo, Resultados: &repositorioResultadosFalso{}})
	uc.Anomalias = nil

	_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "")
	if err == nil || !strings.Contains(err.Error(), "compuerta de anomalias") {
		t.Fatalf("err = %v: sin compuerta cableada la corrida no puede salir de deducciones", err)
	}
	if got := repo.procesos["proc-1"].Etapa; got != reparto.EtapaDeducciones {
		t.Fatalf("etapa = %q, el proceso no debio moverse", got)
	}
}

func TestAvanzarEtapaPropagaElFalloDeLaCompuerta(t *testing.T) {
	t.Parallel()

	repo := procesoEnDeducciones(t, reparto.Internacional)
	falla := errors.New("base caida")
	uc := conBitacora(Procesos{Repo: repo, Anomalias: &compuertaFalsa{err: falla}})

	if _, err := uc.AvanzarEtapa(t.Context(), "proc-1", ""); !errors.Is(err, falla) {
		t.Fatalf("err = %v, se esperaba el fallo de la compuerta", err)
	}
}

func TestLaCompuertaSoloSeConsultaAlSalirDeDeducciones(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Internacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	compuerta := &compuertaFalsa{criticas: 5}
	uc := conBitacora(Procesos{Repo: repo, Anomalias: compuerta})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "") // recaudo -> deducciones
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaDeducciones || len(compuerta.pedidos) != 0 {
		t.Fatalf("etapa = %q, pedidos = %v: recaudo->deducciones no pasa por la compuerta", v.Etapa, compuerta.pedidos)
	}
}

func TestAvanzarEtapaRepiteLaCompuertaAlEntrarAVerificacion(t *testing.T) {
	t.Parallel()

	for _, circuito := range []reparto.Circuito{reparto.Nacional, reparto.Internacional} {
		repo := nuevoRepositorioProcesosFalso()
		p, err := reparto.AbrirProceso("proc-1", "2026-01", circuito, "bolsa-1", "snap-1", "IX")
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		p.Etapa = reparto.EtapaLiquidacionParcial
		if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		compuerta := &compuertaFalsa{criticas: 1}
		uc := Procesos{Repo: repo, Anomalias: compuerta}

		_, err = uc.AvanzarEtapa(t.Context(), "proc-1", "")
		if !errors.Is(err, ErrAnomaliasCriticasAbiertas) {
			t.Fatalf("%s: err = %v, se esperaba ErrAnomaliasCriticasAbiertas", circuito, err)
		}
		if len(compuerta.pedidos) != 1 || compuerta.pedidos[0] != "2026-01" {
			t.Fatalf("%s: pedidos = %v, se esperaba volver a evaluar 2026-01", circuito, compuerta.pedidos)
		}
		if got := repo.procesos["proc-1"].Etapa; got != reparto.EtapaLiquidacionParcial {
			t.Fatalf("%s: etapa = %q, no debio entrar a verificacion con criticas", circuito, got)
		}
	}
}

// conBitacora completa el cableado de asientos que toda transicion exige.
func conBitacora(uc Procesos) Procesos {
	if uc.Bitacora == nil {
		uc.Bitacora = &bitacoraFalsa{}
	}
	if uc.Reloj == nil {
		uc.Reloj = relojFijo{instante: instanteProceso}
	}
	if uc.Unidad == nil {
		uc.Unidad = &unidadFalsa{}
	}
	if uc.Origen == nil {
		uc.Origen = origenCompleto{}
	}
	if uc.Anomalias == nil {
		uc.Anomalias = &compuertaFalsa{}
	}
	return uc
}

// origenCompleto da un origen trivial a todo uso pedido.
type origenCompleto struct{}

func (origenCompleto) OrigenDeUsos(_ context.Context, ids []string) (map[string]OrigenDeUso, error) {
	m := make(map[string]OrigenDeUso, len(ids))
	for _, id := range ids {
		m[id] = OrigenDeUso{UsoID: id, ReporteID: "rep", Escalon: "alias"}
	}
	return m, nil
}

// emisionFalsa es el doble de [EmisionLiquidacion]. Apunta en que etapa estaba
// la corrida en el repositorio en el momento de la llamada: es lo que prueba
// que al ENTRAR se emite despues de guardar la etapa y al SALIR antes.
type emisionFalsa struct {
	repo   *repositorioProcesosFalso
	err    error
	vistas []reparto.Etapa
	ids    []string
}

func (e *emisionFalsa) GenerarLiquidacion(_ context.Context, procesoID string) ([]OrdenVista, error) {
	e.ids = append(e.ids, procesoID)
	e.vistas = append(e.vistas, e.repo.procesos[procesoID].Etapa)
	return nil, e.err
}

// procesoEnEtapa guarda proc-1 en la etapa y revision dadas, con las firmas de
// la compuerta sobre esa misma revision cuando la etapa es una compuerta.
func procesoEnEtapa(t *testing.T, circuito reparto.Circuito, etapa reparto.Etapa, revision int) *repositorioProcesosFalso {
	t.Helper()
	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", circuito, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = etapa
	p.Revision = revision
	if etapa == reparto.EtapaVerificacion {
		p.Firmas = []reparto.Firma{
			{Rol: string(reparto.RolDistribucion), ActorID: "actor-dist", SobreRev: revision},
			{Rol: string(reparto.RolContabilidad), ActorID: "actor-conta", SobreRev: revision},
		}
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	repo.guardados = nil
	return repo
}

// TestAvanzarEtapaEmiteLaLiquidacionAlEntrarALiquidacionFinal es #193: salir de
// verificacion hacia liquidacion_final emite las ordenes del periodo en la
// misma unidad, DESPUES de guardar la etapa -- la liquidacion lee la etapa
// nueva dentro de la transaccion.
func TestAvanzarEtapaEmiteLaLiquidacionAlEntrarALiquidacionFinal(t *testing.T) {
	t.Parallel()

	repo := procesoEnEtapa(t, reparto.Nacional, reparto.EtapaVerificacion, 1)
	emision := &emisionFalsa{repo: repo}
	libro := &bitacoraFalsa{}
	uc := conBitacora(Procesos{Repo: repo, Liquidacion: emision, Bitacora: libro})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if v.Etapa != reparto.EtapaLiquidacionFinal || v.Revision != 2 {
		t.Fatalf("etapa/revision = %q/%d, se esperaba liquidacion_final/2", v.Etapa, v.Revision)
	}
	if !slices.Equal(emision.ids, []string{"proc-1"}) {
		t.Fatalf("GenerarLiquidacion = %v, se esperaba una llamada con proc-1", emision.ids)
	}
	if emision.vistas[0] != reparto.EtapaLiquidacionFinal {
		t.Fatalf("al emitir la corrida estaba en %q; la etapa se guarda antes", emision.vistas[0])
	}
	if len(libro.asientos) != 1 || libro.asientos[0].Hecho != HechoProcesoEtapaAvanzada {
		t.Fatalf("asientos = %+v, se esperaba el de la etapa avanzada", libro.asientos)
	}
}

// TestAvanzarEtapaToleraLaEsperaDelPeriodoAlEntrar: la corrida que llega antes
// que sus hermanas entra igual a liquidacion_final (ADR 0024).
func TestAvanzarEtapaToleraLaEsperaDelPeriodoAlEntrar(t *testing.T) {
	t.Parallel()

	repo := procesoEnEtapa(t, reparto.Nacional, reparto.EtapaVerificacion, 1)
	emision := &emisionFalsa{repo: repo, err: fmt.Errorf("%w: faltan prc-b", ErrLiquidacionEnEspera)}
	uc := conBitacora(Procesos{Repo: repo, Liquidacion: emision})

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin")
	if err != nil {
		t.Fatalf("esperar a las hermanas no es un fallo al entrar: %v", err)
	}
	if v.Etapa != reparto.EtapaLiquidacionFinal || repo.procesos["proc-1"].Etapa != reparto.EtapaLiquidacionFinal {
		t.Fatalf("etapa = %q, se esperaba liquidacion_final", v.Etapa)
	}
}

// TestAvanzarEtapaNoEntraSiLaEmisionFalla: cualquier otro fallo de la
// liquidacion revierte la transicion, para que avanzar y emitir sean un solo
// hecho.
func TestAvanzarEtapaNoEntraSiLaEmisionFalla(t *testing.T) {
	t.Parallel()

	for _, causa := range []error{ErrPeriodoYaLiquidado, ErrCorridaNoCuadra, ErrParametroAusente} {
		repo := procesoEnEtapa(t, reparto.Nacional, reparto.EtapaVerificacion, 1)
		emision := &emisionFalsa{repo: repo, err: causa}
		unidad := &unidadFalsa{}
		libro := &bitacoraFalsa{}
		uc := conBitacora(Procesos{Repo: repo, Liquidacion: emision, Unidad: unidad, Bitacora: libro})

		_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin")
		if !errors.Is(err, causa) {
			t.Fatalf("err = %v, se esperaba %v", err, causa)
		}
		if unidad.confirmo {
			t.Fatalf("%v: la unidad no puede confirmar una etapa cuya emision fallo", causa)
		}
		if len(libro.asientos) != 0 {
			t.Fatalf("%v: asientos = %+v; sin emision no hay etapa avanzada", causa, libro.asientos)
		}
	}
}

// TestAvanzarEtapaNoSaleDeLiquidacionFinalSinLiquidar: salir hacia
// pago_registro vuelve a pedir la liquidacion ANTES de guardar, y si el
// periodo sigue esperando la corrida no se mueve: no se paga lo que no se
// liquido.
func TestAvanzarEtapaNoSaleDeLiquidacionFinalSinLiquidar(t *testing.T) {
	t.Parallel()

	repo := procesoEnEtapa(t, reparto.Nacional, reparto.EtapaLiquidacionFinal, 2)
	emision := &emisionFalsa{repo: repo, err: fmt.Errorf("%w: faltan prc-b", ErrLiquidacionEnEspera)}
	uc := conBitacora(Procesos{Repo: repo, Liquidacion: emision})

	_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin")
	if !errors.Is(err, ErrLiquidacionEnEspera) {
		t.Fatalf("err = %v, se esperaba ErrLiquidacionEnEspera", err)
	}
	if len(repo.guardados) != 0 {
		t.Fatalf("guardados = %+v; la corrida no sale de liquidacion_final", repo.guardados)
	}
	if emision.vistas[0] != reparto.EtapaLiquidacionFinal {
		t.Fatalf("al pedir la liquidacion la corrida estaba en %q; se pide antes de guardar", emision.vistas[0])
	}

	emision.err = nil
	v, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin")
	if err != nil {
		t.Fatalf("con el periodo liquidado sale: %v", err)
	}
	if v.Etapa != reparto.EtapaPagoRegistro {
		t.Fatalf("etapa = %q, se esperaba pago_registro", v.Etapa)
	}
}

// TestAvanzarEtapaSoloLiquidaEnLaFronteraDeLiquidacionFinalNacional: ninguna
// otra transicion llama a la liquidacion, y el internacional tampoco -- no
// valoriza (RD 7.4), asi que no tiene lineas de titular de las que emitir.
func TestAvanzarEtapaSoloLiquidaEnLaFronteraDeLiquidacionFinalNacional(t *testing.T) {
	t.Parallel()

	casos := []struct {
		circuito reparto.Circuito
		etapa    reparto.Etapa
		revision int
	}{
		{reparto.Internacional, reparto.EtapaVerificacion, 1},
		{reparto.Internacional, reparto.EtapaLiquidacionFinal, 2},
		{reparto.Nacional, reparto.EtapaImporteTitular, 1},
		{reparto.Nacional, reparto.EtapaLiquidacionParcial, 1},
	}
	for _, c := range casos {
		repo := procesoEnEtapa(t, c.circuito, c.etapa, c.revision)
		emision := &emisionFalsa{repo: repo}
		uc := conBitacora(Procesos{Repo: repo, Liquidacion: emision})
		if _, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin"); err != nil {
			t.Fatalf("%s desde %s: %v", c.circuito, c.etapa, err)
		}
		if len(emision.ids) != 0 {
			t.Fatalf("%s desde %s: GenerarLiquidacion = %v, no debio llamarse", c.circuito, c.etapa, emision.ids)
		}
	}
}

// TestAvanzarEtapaSinLiquidacionFallaClaro: un Procesos cableado sin la
// liquidacion no puede dejar entrar una corrida a liquidacion_final como si
// nada: seria el defecto de #193 otra vez, sin ningun aviso.
func TestAvanzarEtapaSinLiquidacionFallaClaro(t *testing.T) {
	t.Parallel()

	repo := procesoEnEtapa(t, reparto.Nacional, reparto.EtapaVerificacion, 1)
	uc := conBitacora(Procesos{Repo: repo})

	_, err := uc.AvanzarEtapa(t.Context(), "proc-1", "actor-admin")
	if err == nil || !strings.Contains(err.Error(), "falta Liquidacion") {
		t.Fatalf("err = %v, se esperaba un error de cableado que nombre Liquidacion", err)
	}
}
