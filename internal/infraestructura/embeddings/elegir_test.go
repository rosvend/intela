package embeddings

import "testing"

func TestElegir(t *testing.T) {
	ninguno, err := Elegir(t.Context(), "", "")
	if err != nil || ninguno.Motor != nil {
		t.Errorf("sin proveedor: %+v %v", ninguno, err)
	}
	f, err := Elegir(t.Context(), "falso", "")
	if err != nil || f.Motor == nil || f.Piso != PisoFalso {
		t.Errorf("falso: %+v %v", f, err)
	}
	t.Setenv("AWS_REGION", "us-east-1")
	b, err := Elegir(t.Context(), "bedrock", "")
	if err != nil || b.Motor == nil || b.Piso != PisoBedrock {
		t.Errorf("bedrock: %+v %v", b, err)
	}
	if _, err := Elegir(t.Context(), "openai", ""); err == nil {
		t.Error("un proveedor desconocido tiene que fallar, no caer en otro")
	}
}
