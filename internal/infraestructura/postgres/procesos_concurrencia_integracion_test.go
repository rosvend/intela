package postgres

import (
	"errors"
	"sync"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Aperturas simultaneas del mismo id: una sola fila, un solo asiento proceso.abierto, y nadie recibe error (B4).
func TestAperturasConcurrentesAsientanUnaSolaApertura(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()
	uc := aplicacion.Procesos{
		Repo: s, Parametros: s, Bolsas: s, Declaraciones: s, Usos: s,
		Resultados: s, Unidad: s, Bitacora: s, Reloj: reloj.Sistema{}, Origen: s,
	}

	const id, n = "proc-concurrente", 8
	var wg sync.WaitGroup
	listos := make(chan struct{})
	errs := make([]error, n)
	vistas := make([]aplicacion.ProcesoVista, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-listos
			vistas[i], errs[i] = uc.IniciarProceso(ctx, id, "2026-01", reparto.Nacional, "bolsa-1", "actor-dist")
		}(i)
	}
	close(listos)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("apertura %d: %v", i, err)
		}
		if vistas[i].ID != id || vistas[i].Revision != 1 {
			t.Fatalf("apertura %d devolvio %+v", i, vistas[i])
		}
	}
	var procesos, aperturas int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM procesos WHERE id = $1`, id).Scan(&procesos); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM asientos WHERE hecho = $1 AND ref_tipo = $2 AND ref_id = $3`,
		aplicacion.HechoProcesoAbierto, aplicacion.RefProceso, id).Scan(&aperturas); err != nil {
		t.Fatal(err)
	}
	if procesos != 1 || aperturas != 1 {
		t.Fatalf("procesos = %d, aperturas asentadas = %d; se esperaba 1 y 1", procesos, aperturas)
	}
}

// Un alta sobre un id que ya existe es conflicto, no una actualizacion silenciosa.
func TestGuardarProcesoComoAltaSobreFilaExistenteEsConflicto(t *testing.T) {
	s, _ := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()
	uc := aplicacion.Procesos{
		Repo: s, Parametros: s, Bolsas: s, Declaraciones: s, Usos: s,
		Resultados: s, Unidad: s, Bitacora: s, Reloj: reloj.Sistema{}, Origen: s,
	}
	v, err := uc.IniciarProceso(ctx, "proc-alta", "2026-01", reparto.Nacional, "bolsa-1", "actor-dist")
	if err != nil {
		t.Fatalf("iniciar: %v", err)
	}
	if err := s.GuardarProceso(ctx, v, aplicacion.RevisionAlta); !errors.Is(err, aplicacion.ErrProcesoConflictoDeConcurrencia) {
		t.Fatalf("alta repetida: err = %v, se esperaba ErrProcesoConflictoDeConcurrencia", err)
	}
}
