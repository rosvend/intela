package modelolenguaje

import (
	"testing"

	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/anthropic"
	"github.com/rosvend/intela/internal/infraestructura/modelolenguaje/falso"
)

func TestElegirProveedor(t *testing.T) {
	casos := []struct {
		nombre, proveedor, clave string
		want                     any
		nombreWant               string
	}{
		{"sin nada: no disponible", "", "", falso.NoDisponible{}, "no_disponible"},
		{"clave sin proveedor: anthropic", "", "sk-x", anthropic.Modelo{}, "anthropic"},
		{"falso explicito aunque haya clave", "falso", "sk-x", falso.Modelo{}, "falso"},
		{"anthropic sin clave: no disponible, sin caerse", "anthropic", "", falso.NoDisponible{}, "no_disponible"},
		{"proveedor desconocido: no disponible", "otro", "sk-x", falso.NoDisponible{}, "no_disponible"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			m, nombre := Elegir(c.proveedor, c.clave, "")
			if nombre != c.nombreWant {
				t.Errorf("nombre = %q, se esperaba %q", nombre, c.nombreWant)
			}
			switch c.want.(type) {
			case anthropic.Modelo:
				if _, ok := m.(anthropic.Modelo); !ok {
					t.Errorf("modelo = %T", m)
				}
			case falso.Modelo:
				if _, ok := m.(falso.Modelo); !ok {
					t.Errorf("modelo = %T", m)
				}
			case falso.NoDisponible:
				if _, ok := m.(falso.NoDisponible); !ok {
					t.Errorf("modelo = %T", m)
				}
			}
		})
	}
}
