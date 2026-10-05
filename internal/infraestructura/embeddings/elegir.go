// Package embeddings elige el adaptador de aplicacion.MotorEmbeddings; es el unico sitio que nombra proveedores.
package embeddings

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/embeddings/bedrock"
	"github.com/rosvend/intela/internal/infraestructura/embeddings/falso"
)

// Pisos de similitud coseno por proveedor; PROVISIONALES hasta calibrar con preguntas reales del staff.
const (
	PisoFalso   = 0.2
	PisoBedrock = 0.35
)

// Elegido es el motor configurado con su piso; Motor nil significa "sin proveedor".
type Elegido struct {
	Motor     aplicacion.MotorEmbeddings
	Piso      float64
	Proveedor string
}

// Elegir decide por EMBEDDINGS_PROVEEDOR ("", "falso", "bedrock") y EMBEDDINGS_MODELO; vacio no es error, es ausencia.
func Elegir(ctx context.Context, proveedor, modelo string) (Elegido, error) {
	switch proveedor {
	case "":
		return Elegido{Proveedor: "ninguno"}, nil
	case "falso":
		return Elegido{Motor: falso.Motor{}, Piso: PisoFalso, Proveedor: "falso"}, nil
	case "bedrock":
		if modelo == "" {
			modelo = bedrock.ModeloPorDefecto
		}
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return Elegido{}, fmt.Errorf("configuracion de AWS para bedrock: %w", err)
		}
		return Elegido{Motor: bedrock.Motor{Cliente: bedrockruntime.NewFromConfig(cfg), Modelo: modelo}, Piso: PisoBedrock, Proveedor: "bedrock"}, nil
	default:
		return Elegido{}, fmt.Errorf("EMBEDDINGS_PROVEEDOR desconocido: %q", proveedor)
	}
}
