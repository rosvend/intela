package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type clienteFijo struct {
	entrada *bedrockruntime.InvokeModelInput
	cuerpo  string
	err     error
}

func (c *clienteFijo) InvokeModel(_ context.Context, in *bedrockruntime.InvokeModelInput, _ ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error) {
	c.entrada = in
	if c.err != nil {
		return nil, c.err
	}
	return &bedrockruntime.InvokeModelOutput{Body: []byte(c.cuerpo)}, nil
}

func TestEmbeberPideTitanV2NormalizadoYLeeElVector(t *testing.T) {
	c := &clienteFijo{cuerpo: `{"embedding":[0.1,-0.2,0.3],"inputTextTokenCount":3}`}
	m := Motor{Cliente: c, Modelo: ModeloPorDefecto}

	e, err := m.Embeber(t.Context(), "declaracion incompleta")
	if err != nil {
		t.Fatal(err)
	}
	if e.Modelo != ModeloPorDefecto || len(e.Vector) != 3 || e.Vector[1] != -0.2 {
		t.Fatalf("e = %+v", e)
	}
	if *c.entrada.ModelId != ModeloPorDefecto || *c.entrada.ContentType != "application/json" {
		t.Errorf("entrada = %+v", c.entrada)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(c.entrada.Body, &cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo["inputText"] != "declaracion incompleta" || cuerpo["normalize"] != true || cuerpo["dimensions"] != float64(Dimensiones) {
		t.Errorf("cuerpo = %v", cuerpo)
	}
}

func TestEmbeberFallaSinVectorOConErrorDelProveedor(t *testing.T) {
	if _, err := (Motor{Cliente: &clienteFijo{cuerpo: `{"embedding":[]}`}, Modelo: "m"}).Embeber(t.Context(), "x"); err == nil {
		t.Error("un vector vacio no puede pasar como embedding")
	}
	boom := errors.New("throttled")
	if _, err := (Motor{Cliente: &clienteFijo{err: boom}, Modelo: "m"}).Embeber(t.Context(), "x"); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}
