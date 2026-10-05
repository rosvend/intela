package asistente

import (
	"slices"
	"testing"
)

// #69: las dos lecturas de proceso y ONI quedan en el catalogo que ven cmd/api y cmd/lambda.
func TestHerramientasRegistraEstadoCorridaYListarONI(t *testing.T) {
	c, err := Herramientas(nil)
	if err != nil {
		t.Fatalf("Herramientas: %v", err)
	}
	var nombres []string
	for _, e := range c.Esquemas() {
		nombres = append(nombres, e.Nombre)
	}
	for _, quiere := range []string{"estado_corrida", "listar_oni"} {
		if !slices.Contains(nombres, quiere) {
			t.Errorf("falta %q en el catalogo: %v", quiere, nombres)
		}
	}
}
