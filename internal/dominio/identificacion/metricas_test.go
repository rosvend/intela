package identificacion

import (
	"testing"

	"github.com/shopspring/decimal"
)

func automatica(obraID string) Resultado {
	return Resultado{ObraID: obraID, Escalon: EscalonDifuso, Puntaje: punt("0.80")}
}

func enBanda() Resultado {
	return Resultado{
		Escalon:    EscalonONI,
		ONI:        true,
		Candidatos: []Candidato{{ObraID: "obra-9", Puntaje: punt("0.50")}},
	}
}

func sinCandidato() Resultado {
	return Resultado{Escalon: EscalonONI, ONI: true}
}

func TestMetricas(t *testing.T) {
	casos := []struct {
		nombre string
		evs    []Evaluacion
		quiero Reporte
	}{
		{
			nombre: "conjunto vacio no divide por cero",
			evs:    nil,
			quiero: Reporte{},
		},
		{
			nombre: "todo acierta",
			evs: []Evaluacion{
				{ObraEsperada: "obra-1", Resultado: automatica("obra-1")},
				{ObraEsperada: "obra-2", Resultado: automatica("obra-2")},
			},
			quiero: Reporte{Total: 2, Automaticas: 2, Aciertos: 2},
		},
		{
			nombre: "una asignacion a la obra equivocada es un fallo, no un acierto",
			evs: []Evaluacion{
				{ObraEsperada: "obra-1", Resultado: automatica("obra-1")},
				{ObraEsperada: "obra-2", Resultado: automatica("obra-7")},
			},
			quiero: Reporte{Total: 2, Automaticas: 2, Aciertos: 1, Fallos: 1},
		},
		{
			nombre: "asignar algo que no debia resolverse es un fallo",
			evs: []Evaluacion{
				{ObraEsperada: "", Resultado: automatica("obra-7")},
			},
			quiero: Reporte{Total: 1, Automaticas: 1, Fallos: 1},
		},
		{
			nombre: "no resolver lo que no debia resolverse no es un fallo",
			evs: []Evaluacion{
				{ObraEsperada: "", Resultado: sinCandidato()},
			},
			quiero: Reporte{Total: 1, AONI: 1},
		},
		{
			nombre: "banda y ONI se cuentan por separado",
			evs: []Evaluacion{
				{ObraEsperada: "obra-1", Resultado: automatica("obra-1")},
				{ObraEsperada: "obra-2", Resultado: enBanda()},
				{ObraEsperada: "obra-3", Resultado: sinCandidato()},
			},
			quiero: Reporte{Total: 3, Automaticas: 1, Aciertos: 1, ABanda: 1, AONI: 1},
		},
		{
			// Una decision humana trae obra, pero no es de la cascada: no sube
			// las automaticas ni entra en aciertos o fallos.
			nombre: "una resolucion manual no cuenta como automatica",
			evs: []Evaluacion{
				{ObraEsperada: "obra-1", Resultado: automatica("obra-1")},
				{ObraEsperada: "obra-2", Resultado: Resultado{ObraID: "obra-2", Escalon: EscalonManual}},
			},
			quiero: Reporte{Total: 2, Automaticas: 1, Aciertos: 1, Manuales: 1},
		},
		{
			nombre: "una fila fuera de repertorio no cuenta como auto-asociada",
			evs: []Evaluacion{
				{ObraEsperada: "obra-1", Resultado: automatica("obra-1")},
				{Resultado: Resultado{Escalon: EscalonExcluido}},
			},
			quiero: Reporte{Total: 2, Automaticas: 1, Aciertos: 1, Excluidas: 1},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if tengo := Metricas(c.evs); tengo != c.quiero {
				t.Fatalf("Metricas() = %+v, se esperaba %+v", tengo, c.quiero)
			}
		})
	}
}

// Las categorias tienen que cerrar contra el total: un informe con hueco deja
// filas escapandose sin que nadie lo note.
func TestReporteCierra(t *testing.T) {
	evs := []Evaluacion{
		{ObraEsperada: "obra-1", Resultado: automatica("obra-1")},
		{ObraEsperada: "obra-2", Resultado: automatica("obra-9")},
		{ObraEsperada: "obra-3", Resultado: enBanda()},
		{ObraEsperada: "obra-4", Resultado: sinCandidato()},
		{Resultado: Resultado{Escalon: EscalonExcluido}},
		{ObraEsperada: "obra-5", Resultado: Resultado{ObraID: "obra-5", Escalon: EscalonManual}},
	}
	r := Metricas(evs)

	if suma := r.Automaticas + r.ABanda + r.AONI + r.Excluidas + r.Manuales; suma != r.Total {
		t.Fatalf("las categorias suman %d y el total es %d: %+v", suma, r.Total, r)
	}
	if r.Aciertos+r.Fallos != r.Automaticas {
		t.Fatalf("aciertos+fallos = %d, automaticas = %d", r.Aciertos+r.Fallos, r.Automaticas)
	}
}

func TestReportePorcentajes(t *testing.T) {
	casos := []struct {
		nombre         string
		r              Reporte
		auto, precisio string
	}{
		{
			nombre: "sin filas los dos son cero y no hay panico",
			r:      Reporte{},
			auto:   "0", precisio: "0",
		},
		{
			nombre: "sin ninguna automatica la precision es cero, no indefinida",
			r:      Reporte{Total: 4, ABanda: 4},
			auto:   "0", precisio: "0",
		},
		{
			nombre: "tres de cuatro automaticas, dos de tres aciertan",
			r:      Reporte{Total: 4, Automaticas: 3, Aciertos: 2, Fallos: 1, AONI: 1},
			auto:   "75", precisio: "66.6667",
		},
		{
			nombre: "todo automatico y todo acertado es cien y cien",
			r:      Reporte{Total: 5, Automaticas: 5, Aciertos: 5},
			auto:   "100", precisio: "100",
		},
		{
			// KR-2 mide sobre lo que se intento identificar: las excluidas
			// (R-27) salen del denominador.
			nombre: "las excluidas no cuentan en el denominador de la tasa",
			r:      Reporte{Total: 5, Automaticas: 2, Aciertos: 2, AONI: 2, Excluidas: 1},
			auto:   "50", precisio: "100",
		},
		{
			nombre: "todo excluido: la tasa es cero y no hay division por cero",
			r:      Reporte{Total: 3, Excluidas: 3},
			auto:   "0", precisio: "0",
		},
		{
			// Una manual esta en el denominador y no en el numerador: la
			// cascada no la resolvio.
			nombre: "una manual no sube la tasa de auto-asociacion",
			r:      Reporte{Total: 4, Automaticas: 1, Aciertos: 1, Manuales: 1, AONI: 2},
			auto:   "25", precisio: "100",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			redondear := func(d decimal.Decimal) string { return d.Round(4).String() }
			if got := redondear(c.r.TasaAutoAsociacionPct()); got != c.auto {
				t.Fatalf("TasaAutoAsociacionPct() = %s, se esperaba %s", got, c.auto)
			}
			if got := redondear(c.r.PrecisionPct()); got != c.precisio {
				t.Fatalf("PrecisionPct() = %s, se esperaba %s", got, c.precisio)
			}
		})
	}
}
