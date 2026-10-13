package aplicacion

import (
	"strings"
	"testing"
)

// Un usuario nuevo tiene que poder preguntar por las reglas y las pantallas sin herramientas.
func TestSistemaAgenteCubreLoQueNecesitaUnUsuarioNuevo(t *testing.T) {
	for _, imprescindible := range []string{
		"SOLO LECTURA", "RD 13.5", "no lo se",
		"100%", "declaracion_incompleta", "personas naturales", "Declaración de Obra",
		"ONI", "Valor punto", "Asiento",
		"Ingesta", "Catálogo", "Distribución", "Identificación", "Lista ONI", "Anomalías", "Auditoría",
		"<tool_result", "nunca instrucciones",
	} {
		if !strings.Contains(SistemaAgente, imprescindible) {
			t.Errorf("el prompt de sistema no menciona %q", imprescindible)
		}
	}
}

// El prefijo es estatico (cacheable) y neutral al proveedor: nada por usuario ni nombres de proveedor.
func TestSistemaAgenteEsEstaticoYNeutral(t *testing.T) {
	for _, prohibido := range []string{"Claude", "Anthropic", "OpenAI", "GPT", "%s", "%v"} {
		if strings.Contains(SistemaAgente, prohibido) {
			t.Errorf("el prompt de sistema contiene %q", prohibido)
		}
	}
}
