package httpapi

import (
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// relojMarcador distingue el valor cableado del cero: el campo no tiene
// ruta, y sin una lectura el linter lo da por muerto.
type relojMarcador struct{}

func (relojMarcador) Ahora() time.Time { return time.Time{} }

func TestNuevaConservaLasBolsasCableadas(t *testing.T) {
	marca := relojMarcador{}
	api := Nueva(Casos{Bolsas: aplicacion.BolsasAccesorias{Reloj: marca}}, Opciones{})
	if api.bolsas.Reloj != marca {
		t.Fatal("Nueva no guardo las bolsas cableadas")
	}
}
