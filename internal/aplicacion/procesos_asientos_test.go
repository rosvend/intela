package aplicacion

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var instanteProceso = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

type entornoProcesos struct {
	repo     *repositorioProcesosFalso
	bitacora *bitacoraFalsa
	unidad   *unidadFalsa
	uc       Procesos
}

func nuevoEntornoProcesos() entornoProcesos {
	repo := nuevoRepositorioProcesosFalso()
	bit := &bitacoraFalsa{}
	u := &unidadFalsa{}
	return entornoProcesos{
		repo: repo, bitacora: bit, unidad: u,
		uc: Procesos{
			Repo:       repo,
			Parametros: &parametrosNormativosFalso{id: "snap-1", snap: reparto.Snapshot{Reglamento: "IX"}},
			Bolsas:     &repositorioRecaudoFalso{bolsa: BolsaPersistida{ID: "bolsa-1", Periodo: "2026-01", Circuito: reparto.Nacional}},
			Bitacora:   bit,
			Reloj:      relojFijo{instante: instanteProceso},
			Unidad:     u,
			Anomalias:  &compuertaFalsa{},
		},
	}
}

func (e entornoProcesos) guardar(t *testing.T, etapa reparto.Etapa) {
	t.Helper()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = etapa
	if err := e.repo.GuardarProceso(t.Context(), aProcesoVista(p), 0); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}

func unicoAsiento(t *testing.T, b *bitacoraFalsa, hecho string) (Asiento, AsientoProceso) {
	t.Helper()
	var hallados []Asiento
	for _, a := range b.asientos {
		if a.Hecho == hecho {
			hallados = append(hallados, a)
		}
	}
	if len(hallados) != 1 {
		t.Fatalf("se esperaba un asiento %q, hubo %d: %+v", hecho, len(hallados), b.asientos)
	}
	var payload AsientoProceso
	if err := json.Unmarshal(hallados[0].Payload, &payload); err != nil {
		t.Fatalf("payload de %q no es JSON: %v", hecho, err)
	}
	return hallados[0], payload
}

func TestIniciarProcesoAsientaLaAperturaEnLaMismaUnidad(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()

	if _, err := e.uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1", "actor-dist"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	a, p := unicoAsiento(t, e.bitacora, HechoProcesoAbierto)
	if a.RefTipo != RefProceso || a.RefID != "proc-1" || a.ActorID != "actor-dist" || !a.Cuando.Equal(instanteProceso) {
		t.Fatalf("asiento mal referenciado: %+v", a)
	}
	if p.SnapshotID != "snap-1" || p.Reglamento != "IX" || p.BolsaID != "bolsa-1" || p.Etapa != string(reparto.EtapaRecaudo) {
		t.Fatalf("payload incompleto: %+v", p)
	}
	if e.unidad.entradas != 1 || !e.unidad.confirmo {
		t.Fatalf("la apertura y su asiento no corrieron en una sola unidad: %+v", e.unidad)
	}
}

func TestIniciarProcesoReintentadoNoAsientaDosVeces(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()
	for range 2 {
		if _, err := e.uc.IniciarProceso(t.Context(), "proc-1", "2026-01", reparto.Nacional, "bolsa-1", ""); err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
	}
	unicoAsiento(t, e.bitacora, HechoProcesoAbierto)
}

func TestAvanzarEtapaAsientaActorYEtapas(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()
	e.guardar(t, reparto.EtapaRecaudo)

	if _, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "actor-dist"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	a, p := unicoAsiento(t, e.bitacora, HechoProcesoEtapaAvanzada)
	if a.ActorID != "actor-dist" {
		t.Fatalf("actor = %q, se esperaba actor-dist", a.ActorID)
	}
	if p.EtapaAnterior != string(reparto.EtapaRecaudo) || p.Etapa != string(reparto.EtapaDeducciones) {
		t.Fatalf("etapas = %q -> %q", p.EtapaAnterior, p.Etapa)
	}
}

func TestFirmarAsientaRolActorYRevision(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()
	e.guardar(t, reparto.EtapaVerificacion)

	if _, err := e.uc.Firmar(t.Context(), "proc-1", reparto.RolDistribucion, "actor-dist"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	a, p := unicoAsiento(t, e.bitacora, HechoFirmaRegistrada)
	if a.ActorID != "actor-dist" || p.Firma == nil {
		t.Fatalf("asiento de firma incompleto: %+v / %+v", a, p)
	}
	if p.Firma.Rol != string(reparto.RolDistribucion) || p.Firma.ActorID != "actor-dist" || p.Firma.SobreRevision != 1 {
		t.Fatalf("firma asentada = %+v", *p.Firma)
	}
	if p.Etapa != string(reparto.EtapaVerificacion) {
		t.Fatalf("etapa = %q, se esperaba la compuerta firmada", p.Etapa)
	}
}

func TestRechazarGateAsientaMotivoYActor(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()
	e.guardar(t, reparto.EtapaVerificacion)

	if _, err := e.uc.RechazarGate(t.Context(), "proc-1", "faltan soportes", "actor-conta"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	a, p := unicoAsiento(t, e.bitacora, HechoProcesoCompuertaRechazada)
	if a.ActorID != "actor-conta" || p.Motivo != "faltan soportes" {
		t.Fatalf("rechazo mal asentado: %+v / %+v", a, p)
	}
	if p.EtapaAnterior != string(reparto.EtapaVerificacion) || p.Etapa != string(reparto.EtapaLiquidacionParcial) {
		t.Fatalf("etapas = %q -> %q", p.EtapaAnterior, p.Etapa)
	}
}

func TestUnAsientoFallidoAbortaLaTransicion(t *testing.T) {
	t.Parallel()
	errBitacora := errors.New("bitacora caida")

	casos := map[string]func(e entornoProcesos) error{
		"iniciar": func(e entornoProcesos) error {
			_, err := e.uc.IniciarProceso(t.Context(), "proc-2", "2026-01", reparto.Nacional, "bolsa-1", "a")
			return err
		},
		"avanzar": func(e entornoProcesos) error { _, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "a"); return err },
		"firmar": func(e entornoProcesos) error {
			_, err := e.uc.Firmar(t.Context(), "proc-1", reparto.RolDistribucion, "a")
			return err
		},
		"rechazar": func(e entornoProcesos) error {
			_, err := e.uc.RechazarGate(t.Context(), "proc-1", "m", "a")
			return err
		},
	}
	for nombre, operar := range casos {
		t.Run(nombre, func(t *testing.T) {
			e := nuevoEntornoProcesos()
			etapa := reparto.EtapaVerificacion
			if nombre == "avanzar" {
				etapa = reparto.EtapaRecaudo
			}
			e.guardar(t, etapa)
			e.bitacora.err = errBitacora

			if err := operar(e); !errors.Is(err, errBitacora) {
				t.Fatalf("error = %v, se esperaba el de la bitacora", err)
			}
			if e.unidad.confirmo {
				t.Fatal("la unidad no debio confirmarse con el asiento fallido")
			}
		})
	}
}

func TestProcesosSinBitacoraFallaAntesDeEscribir(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()
	e.guardar(t, reparto.EtapaVerificacion)
	e.uc.Bitacora = nil

	if _, err := e.uc.Firmar(t.Context(), "proc-1", reparto.RolDistribucion, "a"); err == nil {
		t.Fatal("se esperaba un error de cableado")
	}
	if len(e.repo.firmas) != 0 {
		t.Fatal("no debio escribirse una firma sin poder asentarla")
	}
}

func TestAbrirCorridaDelPeriodoAsientaConActorDeSistema(t *testing.T) {
	t.Parallel()
	e := nuevoEntornoProcesos()
	e.uc.Bolsas = &repositorioRecaudoFalso{bolsasPeriodo: []BolsaPersistida{
		{ID: "bolsa-1", UsuarioID: "z", Periodo: "2026-01", Circuito: recaudo.Nacional, Bruto: d("1000")},
	}}

	if err := e.uc.AbrirCorridaDelPeriodo(t.Context(), "2026-01", 1); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	a, _ := unicoAsiento(t, e.bitacora, HechoProcesoAbierto)
	if a.ActorID != actorSistema {
		t.Fatalf("actor = %q, el calendario abre como sistema", a.ActorID)
	}
}
