package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Ninguna compuerta compara la PROSA del contrato con lo que el handler hace:
// `contrato:check` compara tipos y `redocly lint` valida estructura, y las dos
// salen en verde con un 400 que el contrato no nombra. Estas pruebas fijan esa
// propiedad para el guardado de una declaracion -la unica ruta que escribe
// porcentajes de reparto-: por cada causa de 400 que `guardarDeclaracion`
// distingue, la descripcion del 400 de la ruta tiene que nombrar su caso.
//
// La lista de causas de abajo es a mano: una causa nueva en el `switch` de
// `guardarDeclaracion` se anade aqui, y esta prueba falla hasta que el contrato
// la nombre.

const rutaDeDeclaracion = "  /obras/{id}/declaracion:"

// causasDelRechazo son los centinelas que `guardarDeclaracion` mapea a 400 y la
// frase, ya normalizada, con la que el contrato tiene que nombrar cada uno.
var causasDelRechazo = []struct {
	centinela error
	marca     string
}{
	{repertorio.ErrDeclaracionInvalida, "la suma se pasa de 100"},
	{aplicacion.ErrTitularInexistente, "que no esta en el padron"},
	{aplicacion.ErrTitularNoEsPersonaNatural, "R-01"},
	{aplicacion.ErrIPIQueNoCuadra, "el que el padron tiene para ese"},
}

func TestElContratoNombraCadaCausaDelRechazoDeUnaDeclaracion(t *testing.T) {
	lineas := lineasDelContrato(t)
	ruta := bloqueDelContrato(t, lineas, rutaDeDeclaracion)

	for _, metodo := range []string{"post", "put"} {
		t.Run(metodo, func(t *testing.T) {
			operacion := bloqueDelContrato(t, ruta, "    "+metodo+":")
			rechazo := bloqueDelContrato(t, operacion, `        "400":`)
			descripcion := bloqueDelContrato(t, rechazo, "          description: |")
			texto := strings.Join(strings.Fields(strings.Join(descripcion, " ")), " ")

			for _, c := range causasDelRechazo {
				if !strings.Contains(texto, c.marca) {
					t.Errorf("la descripcion del 400 de %s %s no nombra el caso de %v: falta %q",
						strings.ToUpper(metodo), strings.TrimSpace(strings.TrimSuffix(rutaDeDeclaracion, ":")), c.centinela, c.marca)
				}
			}
		})
	}
}

// El IPI de una Parte no es solo "obligatorio": tiene que ser el del padron.
// Quien lee el contrato para armar un cliente decide con esta frase que
// validar, y "obligatorio" a secas dice que basta con que no venga vacio.
func TestElContratoDiceQueElIPIDeUnaParteSeConciliaConElPadron(t *testing.T) {
	lineas := lineasDelContrato(t)
	parte := bloqueDelContrato(t, lineas, "    Parte:")
	ipi := bloqueDelContrato(t, parte, "        ipi:")

	texto := strings.Join(strings.Fields(strings.Join(ipi, " ")), " ")
	if !strings.Contains(texto, "padron") {
		t.Errorf("la descripcion de Parte.ipi no dice que se concilia con el padron: %q", texto)
	}
}

func lineasDelContrato(t *testing.T) []string {
	t.Helper()
	bruto, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("no se pudo leer el contrato: %v", err)
	}
	return strings.Split(strings.ReplaceAll(string(bruto), "\r\n", "\n"), "\n")
}

// bloqueDelContrato devuelve las lineas hijas de la primera linea que es
// exactamente `apertura`: las que siguen hasta la proxima linea no vacia con
// sangria igual o menor. Sirve para bajar por el YAML sin un parser, porque
// esta prueba solo necesita leer la prosa de tres descripciones y el fichero
// esta sangrado de forma regular.
func bloqueDelContrato(t *testing.T, lineas []string, apertura string) []string {
	t.Helper()
	sangria := sangriaDe(apertura)
	for i, l := range lineas {
		if l != apertura {
			continue
		}
		fin := len(lineas)
		for j := i + 1; j < len(lineas); j++ {
			if strings.TrimSpace(lineas[j]) == "" {
				continue
			}
			if sangriaDe(lineas[j]) <= sangria {
				fin = j
				break
			}
		}
		return lineas[i+1 : fin]
	}
	t.Fatalf("el contrato no tiene la linea %q donde se esperaba", apertura)
	return nil
}

func sangriaDe(linea string) int {
	return len(linea) - len(strings.TrimLeft(linea, " "))
}
