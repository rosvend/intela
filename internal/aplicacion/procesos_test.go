package aplicacion

import (
	"context"
	"errors"
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
func (r *repositorioRecaudoFalso) BolsaPorID(_ context.Context, _ string) (BolsaPersistida, error) {
	if r.err != nil {
		return BolsaPersistida{}, r.err
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
	usos []UsoDeReparto
}

func (u *usosDeRepartoFalso) UsosDeCanal(_ context.Context, _, _ string, _ int) ([]UsoDeReparto, ResumenUsosDeCanal, error) {
	return u.usos, ResumenUsosDeCanal{}, nil
}
func (u *usosDeRepartoFalso) UsosSinCanal(_ context.Context, _ string) (int, error) { return 0, nil }

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
	procesos    map[string]ProcesoVista
	guardados   []ProcesoVista
	firmas      []reparto.Firma
	errGuardar  error
	errPorID    error
	errFirmar   error
}

func nuevoRepositorioProcesosFalso() *repositorioProcesosFalso {
	return &repositorioProcesosFalso{procesos: map[string]ProcesoVista{}}
}

func (r *repositorioProcesosFalso) GuardarProceso(_ context.Context, p ProcesoVista) error {
	if r.errGuardar != nil {
		return r.errGuardar
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
	id       string
	snap     reparto.Snapshot
	err      error
	pedido   []time.Time
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
	uc := Procesos{Repo: repo, Parametros: params}

	v, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1")
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
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(ya)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	params := &parametrosNormativosFalso{id: "snap-nuevo"}
	uc := Procesos{Repo: repo, Parametros: params}

	v, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1")
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
	uc := Procesos{Repo: repo, Parametros: params}

	_, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1")
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
	uc := Procesos{Repo: repo, Parametros: params}

	_, err := uc.IniciarProceso(t.Context(), "proc-1", "2026-13", reparto.Nacional, "bolsa-1")
	if err == nil {
		t.Fatal("se esperaba error con un periodo invalido")
	}
	if len(params.pedido) != 0 {
		t.Error("no debio resolverse un snapshot contra un periodo invalido")
	}
}

func procesoNacionalEnVerificacionGuardado(t *testing.T, repo *repositorioProcesosFalso) {
	t.Helper()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaVerificacion
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestFirmarCargaFirmaYPersisteElProceso(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	procesoNacionalEnVerificacionGuardado(t, repo)
	uc := Procesos{Repo: repo}

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
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	uc := Procesos{Repo: repo}

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
	uc := Procesos{Repo: repo}

	v, err := uc.RechazarGate(t.Context(), "proc-1", "faltan soportes")
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

func TestAvanzarEtapaFueraDeValorizacionSoloPersisteLaEtapa(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	resultados := &repositorioResultadosFalso{}
	uc := Procesos{Repo: repo, Resultados: resultados}

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1")
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
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	decl, err := repertorio.NuevaDeclaracion("obra-1", []repertorio.Parte{
		{TitularID: "titular-1", IPI: "IPI-1", Porcentaje: d("100")},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	uc := Procesos{
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
	}

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1")
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

func TestAvanzarEtapaNacionalSinUnidadFallaClaro(t *testing.T) {
	t.Parallel()

	repo := nuevoRepositorioProcesosFalso()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaDeducciones
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	uc := Procesos{Repo: repo} // sin Unidad

	_, err = uc.AvanzarEtapa(t.Context(), "proc-1")
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
	if err := repo.GuardarProceso(t.Context(), aProcesoVista(p)); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	resultados := &repositorioResultadosFalso{}
	uc := Procesos{Repo: repo, Resultados: resultados}

	v, err := uc.AvanzarEtapa(t.Context(), "proc-1")
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
	uc := Procesos{Repo: repo, Parametros: params, Bolsas: bolsas}

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
	uc := Procesos{Repo: repo, Parametros: params, Bolsas: bolsas}

	if err := uc.AbrirCorridaDelPeriodo(t.Context(), "2026-01", 1); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	// El proceso avanza por accion humana entre reintentos del trabajo.
	if err := repo.GuardarProceso(t.Context(), ProcesoVista{
		ID: repo.guardados[0].ID, Circuito: reparto.Nacional, Etapa: reparto.EtapaLiquidacionParcial,
		Periodo: "2026-01", BolsaID: "bolsa-1", SnapshotID: "snap-1", Revision: 1,
	}); err != nil {
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
	uc := Procesos{Repo: repo}

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
	uc := Procesos{Repo: repo}

	lista, err := uc.ListarProcesos(t.Context())
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(lista) != 1 {
		t.Fatalf("lista = %v, se esperaba un proceso", lista)
	}
}
