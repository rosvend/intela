package asistente

import "testing"

// El catalogo arranca valido: un esquema roto tiene que tumbar el arranque, no la primera pregunta.
func TestHerramientasConstruyeUnCatalogoValido(t *testing.T) {
	if _, err := Herramientas(nil); err != nil {
		t.Fatalf("Herramientas: %v", err)
	}
}

// Con proveedor de embeddings, buscar_reglamento queda registrada; sin el, no se anuncia.
func TestBuscarReglamentoSoloConProveedorDeEmbeddings(t *testing.T) {
	tiene := func() bool {
		c, err := Herramientas(nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range c.Esquemas() {
			if e.Nombre == "buscar_reglamento" {
				return true
			}
		}
		return false
	}
	t.Setenv("EMBEDDINGS_PROVEEDOR", "")
	if tiene() {
		t.Error("sin proveedor no se anuncia buscar_reglamento")
	}
	t.Setenv("EMBEDDINGS_PROVEEDOR", "falso")
	if !tiene() {
		t.Error("con proveedor falso buscar_reglamento tiene que estar en el catalogo")
	}
	t.Setenv("EMBEDDINGS_PROVEEDOR", "desconocido")
	if _, err := Herramientas(nil); err == nil {
		t.Error("un proveedor desconocido tiene que tumbar el arranque")
	}
}
