package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Los nombres de campo que el tablero de #104 ya consume, escritos como
// LITERALES y no derivados de `alertaJSON`.
//
// # Por que literales, y por que un fichero aparte
//
// La prueba que habia decodificaba la respuesta en el MISMO struct cuyas
// etiquetas queria fijar (`var cuerpo []alertaJSON; json.Unmarshal(...)`), asi
// que renombrar una etiqueta renombraba al escritor y al lector a la vez y la
// prueba seguia verde. Cuatro mutantes sobrevivian con la suite entera en
// verde: `resuelta`->`resuelto`, `periodo`->`period`, `referencia`->`reference`
// y `detalle`->`detail`.
//
// Y no hay guardia por ningun otro lado: `npm run contrato:check` compara el
// YAML con TypeScript y NUNCA mira Go, asi que una etiqueta cambiada en el
// struct pasa CI y deja el tablero pintando `undefined` en una celda.
//
// `web/src/reparto/tipos.ts` de #104 declara
// `Alerta = { id, tipo, detalle, periodo?, referencia?, resuelta? }`: esos seis
// son contrato con un cliente que ya existe, y los demas son aditivos.
var camposDelContratoDeAlerta = []string{
	// Los seis que #104 ya lee. Cambiar uno rompe el tablero.
	"id", "tipo", "detalle", "periodo", "referencia", "resuelta",
	// Aditivos: los necesita quien RESUELVE (#39), no quien pinta.
	"ref_tipo", "ref_id", "critica", "detectada",
}

// TestElContratoJSONDeUnaAlertaFijaSusNombres compara las claves REALES del
// JSON contra literales.
//
// Decodifica a `map[string]any` a proposito: asi el unico sitio donde el nombre
// del campo aparece dos veces es el struct y esta lista, y renombrar la
// etiqueta rompe la comparacion en vez de moverse con ella.
func TestElContratoJSONDeUnaAlertaFijaSusNombres(t *testing.T) {
	falso := &anomaliasFalsas{alertas: []aplicacion.Alerta{alertaDeEjemplo()}}
	h := servidorConAnomalias(t, aplicacion.RolDistribucion, falso)

	rec := pedir(t, h, http.MethodGet, "/alertas", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}

	var crudo []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if len(crudo) != 1 {
		t.Fatalf("llegaron %d alertas", len(crudo))
	}

	for _, campo := range camposDelContratoDeAlerta {
		if _, hay := crudo[0][campo]; !hay {
			claves := make([]string, 0, len(crudo[0]))
			for k := range crudo[0] {
				claves = append(claves, k)
			}
			slices.Sort(claves)
			t.Errorf("falta la clave %q en el JSON de una alerta.\n"+
				"Claves presentes: %v\n"+
				"Si es un renombrado deliberado, el tablero de #104 (web/src/reparto/tipos.ts) "+
				"lee id/tipo/detalle/periodo/referencia/resuelta y hay que cambiarlo ahi tambien: "+
				"`npm run contrato:check` no mira Go y no lo va a cazar.",
				campo, claves)
		}
	}

	// Los valores tambien, contra literales: una etiqueta correcta sobre el
	// campo equivocado pasaria la comprobacion de arriba.
	quiero := map[string]any{
		"id":         "3f1d0a4e-0000-4000-8000-000000000001",
		"tipo":       "titular_sin_porcentaje",
		"periodo":    "2025-01",
		"referencia": "obra:obra-1#IPI-00000002",
		"ref_tipo":   "obra",
		"ref_id":     "obra-1",
		"resuelta":   false,
		"critica":    false,
	}
	for campo, esperado := range quiero {
		if got := crudo[0][campo]; got != esperado {
			t.Errorf("%q = %#v, se esperaba %#v", campo, got, esperado)
		}
	}
}

// TestElContratoJSONDelResumenFijaSusNombres hace lo mismo con la respuesta de
// la pasada, que es lo que el panel usa para pintar las tarjetas y el aviso de
// la compuerta.
func TestElContratoJSONDelResumenFijaSusNombres(t *testing.T) {
	falso := &anomaliasFalsas{resumen: aplicacion.ResumenEvaluacion{
		Periodo:          "2025-01",
		Detectadas:       7,
		Nuevas:           6,
		PorTipo:          map[string]int{"oni": 1},
		CriticasAbiertas: 3,
		UsosSinCotejar:   2,
	}}
	h := servidorConAnomalias(t, aplicacion.RolDistribucion, falso)

	rec := pedir(t, h, http.MethodPost, "/alertas/evaluacion", `{"periodo":"2025-01"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}

	var crudo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}

	quiero := map[string]any{
		"periodo":           "2025-01",
		"detectadas":        float64(7),
		"nuevas":            float64(6),
		"criticas_abiertas": float64(3),
		"usos_sin_cotejar":  float64(2),
	}
	for campo, esperado := range quiero {
		got, hay := crudo[campo]
		if !hay {
			t.Errorf("falta la clave %q en el resumen de la evaluacion", campo)
			continue
		}
		if got != esperado {
			t.Errorf("%q = %#v, se esperaba %#v", campo, got, esperado)
		}
	}
	if _, hay := crudo["por_tipo"]; !hay {
		t.Error("falta la clave \"por_tipo\": el tablero pinta una tarjeta por tipo")
	}
}
