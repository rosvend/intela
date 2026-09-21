package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// sembrarBolsaParaProceso deja una bolsa y un actor listos para que
// procesos.bolsa_id y firmas.actor_id referencien por FK, sin sembrar la
// fila de procesos: eso es lo que Guardar prueba.
func sembrarBolsaParaProceso(t *testing.T) *Store {
	t.Helper()
	pool := testhelp.Pool(t)
	s := &Store{pool: pool}
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('usuario-1', 'Usuario 1', 'tv_abierta')`); err != nil {
		t.Fatalf("sembrar usuario_recaudo: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
		 VALUES ('bolsa-1', 'usuario-1', '2026-01', 'nacional', 1000.00)`); err != nil {
		t.Fatalf("sembrar bolsa: %v", err)
	}
	for _, actorID := range []string{"actor-dist", "actor-conta"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
			 VALUES ($1, $1 || '@redes.test', $1, 'auditor', 'hash-de-prueba-suficientemente-larga')`,
			actorID); err != nil {
			t.Fatalf("sembrar usuario %q: %v", actorID, err)
		}
	}
	return s
}

func procesoDePrueba() aplicacion.ProcesoVista {
	return aplicacion.ProcesoVista{
		ID:         "proc-1",
		Circuito:   reparto.Nacional,
		Etapa:      reparto.EtapaRecaudo,
		Periodo:    "2026-01",
		BolsaID:    "bolsa-1",
		SnapshotID: "snap-1",
		Reglamento: "IX",
		Revision:   1,
	}
}

func TestProcesosGuardarYPorIDRedondaLaFila(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	ctx := t.Context()
	p := procesoDePrueba()

	if err := s.GuardarProceso(ctx, p); err != nil {
		t.Fatalf("guardar proceso: %v", err)
	}

	leido, err := s.ProcesoPorID(ctx, "proc-1")
	if err != nil {
		t.Fatalf("leer proceso: %v", err)
	}
	if leido.ID != p.ID || leido.Circuito != p.Circuito || leido.Etapa != p.Etapa ||
		leido.Periodo != p.Periodo || leido.BolsaID != p.BolsaID || leido.SnapshotID != p.SnapshotID ||
		leido.Reglamento != p.Reglamento || leido.Revision != p.Revision {
		t.Fatalf("leido = %+v, se esperaba %+v", leido, p)
	}
}

func TestProcesosGuardarActualizaEtapaRevisionYRechazo(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	ctx := t.Context()
	p := procesoDePrueba()
	if err := s.GuardarProceso(ctx, p); err != nil {
		t.Fatalf("guardar proceso: %v", err)
	}

	p.Etapa = reparto.EtapaDeducciones
	p.Revision = 2
	p.RechazoMotivo = "faltan soportes"
	if err := s.GuardarProceso(ctx, p); err != nil {
		t.Fatalf("actualizar proceso: %v", err)
	}

	leido, err := s.ProcesoPorID(ctx, "proc-1")
	if err != nil {
		t.Fatalf("leer proceso: %v", err)
	}
	if leido.Etapa != reparto.EtapaDeducciones || leido.Revision != 2 || leido.RechazoMotivo != "faltan soportes" {
		t.Fatalf("leido = %+v, se esperaba la actualizacion", leido)
	}
	// circuito, bolsa_id, snapshot_id y reglamento no cambian tras abrir.
	if leido.Circuito != p.Circuito || leido.BolsaID != p.BolsaID || leido.SnapshotID != p.SnapshotID {
		t.Fatalf("la identidad del proceso no debio cambiar: %+v", leido)
	}
}

func TestProcesosPorIDSinFilaEsNoEncontrado(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	_, err := s.ProcesoPorID(t.Context(), "proc-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}

func TestProcesosListarDevuelveTodos(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	ctx := t.Context()
	p1 := procesoDePrueba()
	p2 := procesoDePrueba()
	p2.ID = "proc-2"
	p2.Circuito = reparto.Internacional
	if err := s.GuardarProceso(ctx, p1); err != nil {
		t.Fatalf("guardar proc-1: %v", err)
	}
	if err := s.GuardarProceso(ctx, p2); err != nil {
		t.Fatalf("guardar proc-2: %v", err)
	}

	lista, err := s.ListarProcesos(ctx)
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("lista = %v, se esperaban 2 procesos", lista)
	}
}

func TestProcesosGuardarFirmaYPorIDTraeLasFirmas(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	ctx := t.Context()
	p := procesoDePrueba()
	p.Etapa = reparto.EtapaVerificacion
	if err := s.GuardarProceso(ctx, p); err != nil {
		t.Fatalf("guardar proceso: %v", err)
	}

	f := reparto.Firma{Rol: string(reparto.RolDistribucion), ActorID: "actor-dist", SobreRev: 1}
	if err := s.GuardarFirma(ctx, "proc-1", f); err != nil {
		t.Fatalf("guardar firma: %v", err)
	}

	leido, err := s.ProcesoPorID(ctx, "proc-1")
	if err != nil {
		t.Fatalf("leer proceso: %v", err)
	}
	if len(leido.Firmas) != 1 || leido.Firmas[0] != f {
		t.Fatalf("firmas = %v, se esperaba %v", leido.Firmas, []reparto.Firma{f})
	}
}

func TestProcesosGuardarFirmaRechazaElMismoRolDosVecesEnLaMismaRevision(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	ctx := t.Context()
	p := procesoDePrueba()
	p.Etapa = reparto.EtapaVerificacion
	if err := s.GuardarProceso(ctx, p); err != nil {
		t.Fatalf("guardar proceso: %v", err)
	}

	f := reparto.Firma{Rol: string(reparto.RolDistribucion), ActorID: "actor-dist", SobreRev: 1}
	if err := s.GuardarFirma(ctx, "proc-1", f); err != nil {
		t.Fatalf("guardar primera firma: %v", err)
	}
	otra := reparto.Firma{Rol: string(reparto.RolDistribucion), ActorID: "actor-conta", SobreRev: 1}
	if err := s.GuardarFirma(ctx, "proc-1", otra); err == nil {
		t.Fatal("se esperaba error: la base tambien exige la clave (proceso_id, rol, revision)")
	}
}

func TestProcesosGuardarFirmaRechazaProcesoInexistente(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	f := reparto.Firma{Rol: string(reparto.RolDistribucion), ActorID: "actor-dist", SobreRev: 1}
	err := s.GuardarFirma(t.Context(), "proc-que-no-existe", f)
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}
