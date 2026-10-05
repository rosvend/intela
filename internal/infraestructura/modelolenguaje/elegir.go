// Package modelolenguaje elige el adaptador de aplicacion.ModeloLenguaje; es el unico sitio que nombra proveedores.
package modelolenguaje

import (
	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/anthropic"
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/falso"
)

// Elegir decide por AGENTE_PROVEEDOR, ANTHROPIC_API_KEY y AGENTE_MODELO; nunca falla: sin proveedor, responde "no disponible".
func Elegir(proveedor, clave, modelo string) (aplicacion.ModeloLenguaje, string) {
	switch proveedor {
	case "falso":
		return falso.Modelo{}, "falso"
	case "anthropic", "":
		if clave == "" {
			return falso.NoDisponible{}, "no_disponible"
		}
		return anthropic.Nuevo(clave, modelo), "anthropic"
	default:
		return falso.NoDisponible{}, "no_disponible"
	}
}
