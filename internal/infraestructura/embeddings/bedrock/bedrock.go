// Package bedrock adapta MotorEmbeddings a Amazon Titan Text Embeddings V2 via Bedrock InvokeModel.
package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ModeloPorDefecto y Dimensiones: Titan V2 acepta 256, 512 o 1024; 1024 es su valor por defecto.
const (
	ModeloPorDefecto = "amazon.titan-embed-text-v2:0"
	Dimensiones      = 1024
)

// Cliente es lo unico que el adaptador usa de bedrockruntime.Client; las pruebas lo sustituyen.
type Cliente interface {
	InvokeModel(ctx context.Context, in *bedrockruntime.InvokeModelInput, opts ...func(*bedrockruntime.Options)) (*bedrockruntime.InvokeModelOutput, error)
}

// Motor embebe con el modelo de Bedrock indicado; el nombre del modelo viaja con el vector.
type Motor struct {
	Cliente Cliente
	Modelo  string
}

type peticion struct {
	InputText  string `json:"inputText"`
	Dimensions int    `json:"dimensions"`
	Normalize  bool   `json:"normalize"`
}

type respuesta struct {
	Embedding []float32 `json:"embedding"`
}

// Embeber pide el vector normalizado: con norma 1, coseno y producto punto coinciden.
func (m Motor) Embeber(ctx context.Context, texto string) (aplicacion.Embedding, error) {
	cuerpo, err := json.Marshal(peticion{InputText: texto, Dimensions: Dimensiones, Normalize: true})
	if err != nil {
		return aplicacion.Embedding{}, err
	}
	out, err := m.Cliente.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(m.Modelo),
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
		Body:        cuerpo,
	})
	if err != nil {
		return aplicacion.Embedding{}, fmt.Errorf("bedrock %s: %w", m.Modelo, err)
	}
	var r respuesta
	if err := json.Unmarshal(out.Body, &r); err != nil {
		return aplicacion.Embedding{}, fmt.Errorf("bedrock %s: respuesta ilegible: %w", m.Modelo, err)
	}
	if len(r.Embedding) == 0 {
		return aplicacion.Embedding{}, errors.New("bedrock: respuesta sin embedding")
	}
	return aplicacion.Embedding{Modelo: m.Modelo, Vector: r.Embedding}, nil
}
