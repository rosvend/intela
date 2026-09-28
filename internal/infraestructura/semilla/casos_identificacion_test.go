package semilla

import (
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

func casosPendientes(t *testing.T, store aplicacion.RepositorioCasosIdentificacion, periodo string) aplicacion.PaginaCasos {
	t.Helper()
	pag, err := aplicacion.CasosIdentificacion{Repo: store}.Listar(t.Context(), aplicacion.FiltroCasos{
		Estado: aplicacion.EstadoCasoPendiente, Periodo: periodo,
	})
	if err != nil {
		t.Fatalf("listar casos de %s: %v", periodo, err)
	}
	return pag
}

// La cola de identificacion sale de la cascada real (ADR 0007), no de un UPDATE: con y sin candidatos.
func TestCargarDejaCasosDeIdentificacionPorLaCascada(t *testing.T) {
	store, _ := abrir(t)
	ctx := t.Context()
	if err := Cargar(ctx, store, disco(t), hasher(), clavesPrueba(), false, silencio()); err != nil {
		t.Fatalf("Cargar: %v", err)
	}

	pag := casosPendientes(t, store, PeriodoCasos)
	var conCandidatos, sinCandidatos int
	for _, c := range pag.Casos {
		t.Logf("caso %q: %d candidatos %+v", c.Titulo, len(c.Candidatos), c.Candidatos)
		if len(c.Candidatos) == 0 {
			sinCandidatos++
			continue
		}
		conCandidatos++
		for i, cand := range c.Candidatos {
			if cand.Titulo == "" || cand.Puntaje.IsZero() {
				t.Fatalf("caso %q: candidato %d sin titulo o puntaje: %+v", c.Titulo, i, cand)
			}
			if i > 0 && cand.Puntaje.GreaterThan(c.Candidatos[i-1].Puntaje) {
				t.Fatalf("caso %q: candidatos fuera de orden: %+v", c.Titulo, c.Candidatos)
			}
		}
	}
	if conCandidatos == 0 || sinCandidatos == 0 {
		t.Fatalf("casos pendientes: %d con candidatos, %d sin candidatos; se esperaba al menos uno de cada (%+v)",
			conCandidatos, sinCandidatos, pag.Casos)
	}
	if pag.Pendientes != len(pag.Casos) {
		t.Fatalf("pendientes = %d, casos = %d", pag.Pendientes, len(pag.Casos))
	}
	if otros := casosPendientes(t, store, Periodo); otros.Pendientes != 0 {
		t.Fatalf("el periodo %s del reparto tiene %d casos: los casos no pueden tocar sus cifras", Periodo, otros.Pendientes)
	}
}

// Una base sembrada antes de la cola la recibe con la carga normal, y repetirla no duplica: es el camino de produccion.
func TestCargarCompletaLaColaEnUnaBaseYaSembrada(t *testing.T) {
	store, pool := abrir(t)
	ctx := t.Context()
	almacen := disco(t)
	cargar := func() error { return Cargar(ctx, store, almacen, hasher(), clavesPrueba(), false, silencio()) }
	if err := cargar(); err != nil {
		t.Fatalf("Cargar: %v", err)
	}
	antes := casosPendientes(t, store, PeriodoCasos).Pendientes
	if _, err := pool.Exec(ctx, `DELETE FROM reportes WHERE periodo = $1`, PeriodoCasos); err != nil {
		t.Fatalf("simular una base sembrada antes de la cola: %v", err)
	}

	for i := range 2 {
		if err := cargar(); err != nil {
			t.Fatalf("carga %d sobre una base ya sembrada: %v", i, err)
		}
		if despues := casosPendientes(t, store, PeriodoCasos).Pendientes; despues != antes {
			t.Fatalf("carga %d: pendientes = %d, se esperaban %d", i, despues, antes)
		}
	}
}
