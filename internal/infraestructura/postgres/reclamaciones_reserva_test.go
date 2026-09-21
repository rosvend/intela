package postgres

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

func estadoPersistido(t *testing.T, s *Store, id string) string {
	t.Helper()
	var estado string
	if err := s.pool.QueryRow(t.Context(), `SELECT estado FROM reclamaciones WHERE id = $1`, id).Scan(&estado); err != nil {
		t.Fatalf("leer estado de %q: %v", id, err)
	}
	return estado
}

// sembrarTitularYActores deja una corrida base (proceso-1, titular-a,
// titular-b) y dos usuarios (para actor_id de los avales) listos para que
// reclamaciones/reclamaciones_avales referencien por FK. La reclamacion de
// los tests usa titular-a y proceso-1, que ya vienen de sembrarCorridaBase.
//
// testhelp.Pool(t) devuelve una base RECIEN MIGRADA Y VACIA en cada llamada:
// una segunda llamada aqui borraria lo que sembrarCorridaBase ya escribio.
// Se reutiliza s.pool, el mismo pool que sembrarCorridaBase ya abrio.
func sembrarTitularYActores(t *testing.T) *Store {
	t.Helper()
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	for _, u := range []string{"actor-revisoria", "actor-contabilidad"} {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
			 VALUES ($1, $1 || '@redes.test', $1, 'auditor', 'hash-de-prueba-suficientemente-larga')`,
			u); err != nil {
			t.Fatalf("sembrar usuario %q: %v", u, err)
		}
	}
	return s
}

func TestReclamacionesGuardarYReclamacionPorIDRedondaLaFila(t *testing.T) {
	s := sembrarTitularYActores(t)
	ctx := t.Context()

	r, err := reparto.NuevaReclamacionReserva("rec-1", "titular-a", "proceso-1", "error en el conteo",
		dec("30.00"), true, false)
	if err != nil {
		t.Fatalf("construir reclamacion: %v", err)
	}

	if err := s.GuardarReclamacion(ctx, r); err != nil {
		t.Fatalf("guardar reclamacion: %v", err)
	}

	leido, err := s.ReclamacionPorID(ctx, "rec-1")
	if err != nil {
		t.Fatalf("leer reclamacion: %v", err)
	}
	if leido.ID != r.ID || leido.TitularID != r.TitularID || leido.Detalle != r.Detalle ||
		!leido.MontoSolicitado.Equal(r.MontoSolicitado) || leido.Pagable() {
		t.Fatalf("leido = %+v, se esperaba %+v y no pagable", leido, r)
	}
}

func TestReclamacionesGuardarPersisteLosAvalesYElEstadoPagable(t *testing.T) {
	s := sembrarTitularYActores(t)
	ctx := t.Context()

	r, err := reparto.NuevaReclamacionReserva("rec-1", "titular-a", "proceso-1", "detalle",
		dec("30.00"), true, false)
	if err != nil {
		t.Fatalf("construir reclamacion: %v", err)
	}
	if err := s.GuardarReclamacion(ctx, r); err != nil {
		t.Fatalf("guardar reclamacion: %v", err)
	}

	r, err = r.Avalar(reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-revisoria")
	if err != nil {
		t.Fatalf("avalar: %v", err)
	}
	if err := s.GuardarReclamacion(ctx, r); err != nil {
		t.Fatalf("guardar primer aval: %v", err)
	}
	r, err = r.Avalar(reparto.RolDistribucionYContabilidad, "actor-contabilidad")
	if err != nil {
		t.Fatalf("avalar: %v", err)
	}
	if err := s.GuardarReclamacion(ctx, r); err != nil {
		t.Fatalf("guardar segundo aval: %v", err)
	}

	leido, err := s.ReclamacionPorID(ctx, "rec-1")
	if err != nil {
		t.Fatalf("leer reclamacion: %v", err)
	}
	if !leido.Pagable() {
		t.Fatalf("con los dos avales la reclamacion leida deberia ser pagable: %+v", leido)
	}
	if len(leido.Avales) != 2 {
		t.Fatalf("se esperaban 2 avales, hubo %d", len(leido.Avales))
	}
}

// TestGuardarReclamacionRecalculaEstadoBajoAvalesConcurrentes es la
// reproduccion de B4: los dos avales de RD 14.5.10-12 firmados casi a la
// vez, cada uno desde su propia lectura de la reclamacion, no pueden dejar
// el estado persistido en 'abierta' cuando los dos avales ya estan en la
// base.
func TestGuardarReclamacionRecalculaEstadoBajoAvalesConcurrentes(t *testing.T) {
	s := sembrarTitularYActores(t)
	ctx := t.Context()

	const rondas = 12
	for ronda := 0; ronda < rondas; ronda++ {
		id := fmt.Sprintf("rec-ronda-%d", ronda)
		r, err := reparto.NuevaReclamacionReserva(id, "titular-a", "proceso-1", "detalle", dec("30.00"), true, false)
		if err != nil {
			t.Fatalf("ronda %d: construir reclamacion: %v", ronda, err)
		}
		if err := s.GuardarReclamacion(ctx, r); err != nil {
			t.Fatalf("ronda %d: abrir reclamacion: %v", ronda, err)
		}

		var listas sync.WaitGroup
		arranca := make(chan struct{})
		avalar := func(rol reparto.RolAvalReclamacion, actorID string) {
			defer listas.Done()
			<-arranca
			leido, err := s.ReclamacionPorID(ctx, id)
			if err != nil {
				t.Errorf("ronda %d: leer antes de avalar: %v", ronda, err)
				return
			}
			leido, err = leido.Avalar(rol, actorID)
			if err != nil {
				t.Errorf("ronda %d: avalar: %v", ronda, err)
				return
			}
			if err := s.GuardarReclamacion(ctx, leido); err != nil {
				t.Errorf("ronda %d: guardar aval: %v", ronda, err)
			}
		}
		listas.Add(2)
		go avalar(reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-revisoria")
		go avalar(reparto.RolDistribucionYContabilidad, "actor-contabilidad")
		close(arranca)
		listas.Wait()

		final, err := s.ReclamacionPorID(ctx, id)
		if err != nil {
			t.Fatalf("ronda %d: leer resultado: %v", ronda, err)
		}
		if len(final.Avales) != 2 {
			t.Fatalf("ronda %d: se esperaban 2 avales persistidos, hubo %d", ronda, len(final.Avales))
		}
		if estado := estadoPersistido(t, s, id); estado != "resuelta" {
			t.Fatalf("ronda %d: estado persistido = %q con los 2 avales ya en la base, se esperaba 'resuelta'", ronda, estado)
		}
	}
}

func TestReclamacionPorIDSinFilaEsNoEncontrado(t *testing.T) {
	s := sembrarTitularYActores(t)
	_, err := s.ReclamacionPorID(t.Context(), "rec-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}
