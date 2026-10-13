// Package modelolenguaje elige el adaptador de aplicacion.ModeloLenguaje; es el unico sitio que nombra proveedores.
package modelolenguaje

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/anthropic"
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/bedrock"
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/falso"
)

// Elegir decide por AGENTE_PROVEEDOR, ANTHROPIC_API_KEY y AGENTE_MODELO; nunca falla: sin proveedor, responde "no disponible".
func Elegir(ctx context.Context, proveedor, clave, modelo string) (aplicacion.ModeloLenguaje, string) {
	switch proveedor {
	case "falso":
		return falso.Modelo{}, "falso"
	case "bedrock":
		return elegirBedrock(ctx, modelo)
	case "anthropic", "":
		if clave == "" {
			return falso.NoDisponible{}, "no_disponible"
		}
		return anthropic.Nuevo(clave, modelo), "anthropic"
	default:
		return falso.NoDisponible{}, "no_disponible"
	}
}

// elegirBedrock toma region y credenciales de la cadena estandar de AWS; un solo intento porque el reintento es del bucle.
func elegirBedrock(ctx context.Context, modelo string) (aplicacion.ModeloLenguaje, string) {
	if modelo == "" {
		modelo = bedrock.ModeloPorDefecto
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRetryMaxAttempts(1))
	if err != nil {
		return falso.NoDisponible{}, "no_disponible"
	}
	return bedrock.Modelo{Cliente: bedrockruntime.NewFromConfig(cfg), Modelo: modelo}, "bedrock"
}
