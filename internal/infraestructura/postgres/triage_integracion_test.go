package postgres

import (
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/infraestructura/triage"
)

// Resolver u-1 a la obra de MENOR similitud y volver a listar u-2, que es el
// mismo titulo y no se ha tocado. Sin historia la sugerencia sigue a la
// similitud; con la resolucion ya guardada, la sugerencia cambia. u-2 sigue
// en ONI: la sugerencia no resuelve.
func TestTriageIntegracionLaHistoriaMejoraSinResolverElCasoReservado(t *testing.T) {
	s, pool := sembrarResolucion(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO candidatos_match (uso_id, obra_id, puntaje, orden, titulo_consultado)
		 VALUES ('u-2', $1, 0.41000, 1, 'titulo emitido')`, obraImdb); err != nil {
		t.Fatalf("anadir la candidata de menor similitud: %v", err)
	}

	listar := aplicacion.CasosIdentificacion{Repo: s, Ejemplos: s, Rankeador: triage.Heuristico{}}
	antes, err := listar.Listar(ctx, aplicacion.FiltroCasos{Estado: aplicacion.EstadoCasoPendiente})
	if err != nil {
		t.Fatalf("Listar antes: %v", err)
	}
	u2 := casoDe(t, antes, "u-2")
	if u2.Estado != aplicacion.EstadoCasoPendiente {
		t.Fatalf("u-2 estado = %s", u2.Estado)
	}
	if u2.Sugerencia == nil || u2.Sugerencia.ObraID != obraIda {
		t.Fatalf("sin historia se esperaba la candidata de mayor similitud: %+v", u2.Sugerencia)
	}

	r := resolucionDePrueba(s, relojQuieto{instanteResolucion})
	resuelto, err := r.Resolver(ctx,
		solicitudResolucion("u-1", "asignar", obraImdb, "es la otra obra"),
		revisorDePrueba, "Ana Perez")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if resuelto.Estado != aplicacion.EstadoCasoAsignado || resuelto.ObraAsignada == nil ||
		resuelto.ObraAsignada.ID != obraImdb {
		t.Fatalf("se aplico algo distinto de lo que pidio la persona: %+v", resuelto)
	}
	if resuelto.Sugerencia == nil || resuelto.Sugerencia.Aceptada == nil || *resuelto.Sugerencia.Aceptada {
		t.Fatalf("la persona cambio la sugerencia y quedo aceptada: %+v", resuelto.Sugerencia)
	}

	var aceptada bool
	var decision, obra string
	if err := pool.QueryRow(ctx,
		`SELECT aceptada, decision, COALESCE(obra_elegida, '')
		   FROM ejemplos_resolucion WHERE uso_id = 'u-1'`).Scan(&aceptada, &decision, &obra); err != nil {
		t.Fatalf("leer el ejemplo: %v", err)
	}
	if aceptada || decision != string(identificacion.DecisionAsignar) || obra != obraImdb {
		t.Fatalf("ejemplo = aceptada %v decision %s obra %s", aceptada, decision, obra)
	}

	despues, err := listar.Listar(ctx, aplicacion.FiltroCasos{Estado: aplicacion.EstadoCasoPendiente})
	if err != nil {
		t.Fatalf("Listar despues: %v", err)
	}
	reservado := casoDe(t, despues, "u-2")
	if reservado.Estado != aplicacion.EstadoCasoPendiente || reservado.Escalon != identificacion.EscalonONI {
		t.Fatalf("el caso reservado se resolvio solo: estado %s escalon %s", reservado.Estado, reservado.Escalon)
	}
	if reservado.Sugerencia == nil || reservado.Sugerencia.ObraID != obraImdb ||
		reservado.Sugerencia.Decision != string(identificacion.DecisionAsignar) {
		t.Fatalf("la historia no mejoro la sugerencia del caso reservado: %+v", reservado.Sugerencia)
	}
	if reservado.Candidatos[0].ObraID != obraIda {
		t.Fatalf("se reescribio el orden de la cascada: %+v", reservado.Candidatos)
	}
}

func casoDe(t *testing.T, pag aplicacion.PaginaCasos, id string) aplicacion.CasoIdentificacion {
	t.Helper()
	for _, c := range pag.Casos {
		if c.UsoID == id {
			return c
		}
	}
	t.Fatalf("no esta el caso %s", id)
	return aplicacion.CasoIdentificacion{}
}
