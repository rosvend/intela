package aplicacion

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// rankeadorDeDominio es el adaptador de prueba: el caso de uso habla con el
// puerto, y por detras esta la heuristica del nucleo. La infraestructura real
// es la misma funcion, en internal/infraestructura/triage.
type rankeadorDeDominio struct{}

func (rankeadorDeDominio) Rankear(p identificacion.PedidoTriage) identificacion.Sugerencia {
	return identificacion.Rankear(p)
}

type ejemplosFalsa struct {
	historia          []identificacion.EjemploEtiquetado
	guardados         map[string]EjemploGuardado
	err               error
	guardadosLlamados int
}

func (e *ejemplosFalsa) HistoriaPorClaves(context.Context, []string) ([]identificacion.EjemploEtiquetado, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.historia, nil
}

func (e *ejemplosFalsa) GuardarEjemplo(context.Context, EjemploResolucion) error {
	return e.err
}

func (e *ejemplosFalsa) EjemplosDe(context.Context, []string) (map[string]EjemploGuardado, error) {
	e.guardadosLlamados++
	if e.err != nil {
		return nil, e.err
	}
	if e.guardados == nil {
		return map[string]EjemploGuardado{}, nil
	}
	return e.guardados, nil
}

// La sugerencia no resuelve: con un historial que apunta a la obra de menor
// similitud, el caso sigue pendiente y los candidatos siguen en el orden de
// la cascada.
func TestListarSugiereSinResolver(t *testing.T) {
	creado := instanteDePrueba
	repo := &casosFalsos{pagina: PaginaCasos{Pendientes: 1, Casos: []CasoIdentificacion{
		{
			UsoID: "u1", Titulo: "La Casa", Escalon: identificacion.EscalonONI, ReporteCreado: creado,
			Candidatos: []CandidatoCaso{
				{ObraID: "obra-alta", Titulo: "Alta", Puntaje: decimal.RequireFromString("0.80")},
				{ObraID: "obra-baja", Titulo: "Baja", Puntaje: decimal.RequireFromString("0.62")},
			},
		},
	}}}
	ejemplos := &ejemplosFalsa{historia: []identificacion.EjemploEtiquetado{
		{Clave: identificacion.ClaveDeTitulo("La Casa", ""), Decision: identificacion.DecisionAsignar, ObraID: "obra-baja"},
	}}

	pag, err := CasosIdentificacion{Repo: repo, Ejemplos: ejemplos, Rankeador: rankeadorDeDominio{}}.Listar(
		context.Background(), FiltroCasos{Estado: EstadoCasoPendiente})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	caso := pag.Casos[0]
	if caso.Estado != EstadoCasoPendiente {
		t.Fatalf("estado = %s: una sugerencia no puede resolver el caso", caso.Estado)
	}
	if caso.Sugerencia == nil || caso.Sugerencia.Decision != string(identificacion.DecisionAsignar) ||
		caso.Sugerencia.ObraID != "obra-baja" || caso.Sugerencia.Titulo != "Baja" {
		t.Fatalf("sugerencia = %+v", caso.Sugerencia)
	}
	if caso.Sugerencia.Aceptada != nil {
		t.Fatal("un pendiente no tiene calidad medida")
	}
	if caso.Candidatos[0].ObraID != "obra-alta" {
		t.Fatalf("reordeno la evidencia de la cascada: %+v", caso.Candidatos)
	}
	if caso.Sugerencia.Orden[0] != "obra-baja" {
		t.Fatalf("orden sugerido = %v", caso.Sugerencia.Orden)
	}
	if ejemplos.guardadosLlamados != 0 {
		t.Fatal("un pendiente no relee ejemplos guardados: el primer caso de la pagina es el unico y esta pendiente")
	}
}

// La persona elige la otra obra. Lo que queda escrito es SU decision, y el
// ejemplo queda marcado como no aceptado. No hay otro camino que aplique la
// sugerencia.
func TestResolverGuardaElEjemploYNoAplicaLaSugerencia(t *testing.T) {
	repo, libro := repoDePrueba(), &bitacoraFalsa{}
	clave := identificacion.ClaveDeTitulo("La Nina T3 E12", "")
	repo.ejemplos["uso-previo"] = EjemploResolucion{
		UsoID: "uso-previo", Clave: clave, Decision: identificacion.DecisionAsignar, ObraElegida: "obra-40",
	}
	svc, _ := servicioDeResolucion(repo, libro, &relojEspia{instante: instanteDePrueba})

	caso, err := svc.Resolver(t.Context(), solicitudAsignar("obra-12"), actorResolucion, nombreActor)
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	fila := repo.usos[usoDePrueba]
	if fila.Uso.Escalon != identificacion.EscalonManual || fila.Uso.ObraID != "obra-12" {
		t.Fatalf("la fila siguio la sugerencia en vez de la persona: %+v", fila.Uso)
	}
	ejemplo, hay := repo.ejemplos[usoDePrueba]
	if !hay {
		t.Fatal("la resolucion no quedo como ejemplo etiquetado")
	}
	if ejemplo.Decision != identificacion.DecisionAsignar || ejemplo.ObraElegida != "obra-12" {
		t.Fatalf("el ejemplo no guarda la decision de la persona: %+v", ejemplo)
	}
	if ejemplo.SugerenciaDecision != string(identificacion.DecisionAsignar) || ejemplo.SugerenciaObraID != "obra-40" {
		t.Fatalf("la sugerencia guardada = %s %s", ejemplo.SugerenciaDecision, ejemplo.SugerenciaObraID)
	}
	if ejemplo.Aceptada {
		t.Fatal("la persona cambio la sugerencia y quedo marcada como aceptada")
	}
	if len(ejemplo.Candidatos) != 2 {
		t.Fatalf("el ejemplo no guarda las candidatas: %+v", ejemplo.Candidatos)
	}
	if caso.Sugerencia == nil || caso.Sugerencia.Aceptada == nil || *caso.Sugerencia.Aceptada {
		t.Fatalf("la respuesta no dice que la sugerencia se cambio: %+v", caso.Sugerencia)
	}

	p := payloadDe(t, libro)
	if p["decision"] != "asignar" || p["obra_id"] != "obra-12" {
		t.Fatalf("el asiento no es la decision de la persona: %v", p)
	}
	if p["sugerencia_decision"] != "asignar" || p["sugerencia_obra_id"] != "obra-40" || p["sugerencia_aceptada"] != false {
		t.Fatalf("el asiento no mide la sugerencia: %v", p)
	}
}
