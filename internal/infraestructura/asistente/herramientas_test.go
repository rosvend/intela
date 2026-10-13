package asistente

import "testing"

// El catalogo arranca valido: un esquema roto tiene que tumbar el arranque, no la primera pregunta.
func TestHerramientasConstruyeUnCatalogoValido(t *testing.T) {
	if _, err := Herramientas(nil); err != nil {
		t.Fatalf("Herramientas: %v", err)
	}
}
