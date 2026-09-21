package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

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

func TestReclamacionPorIDSinFilaEsNoEncontrado(t *testing.T) {
	s := sembrarTitularYActores(t)
	_, err := s.ReclamacionPorID(t.Context(), "rec-que-no-existe")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}
