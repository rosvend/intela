package asistente

import (
	"os"
	"testing"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// TestMain apaga el contenedor de las pruebas de integracion; os.Exit se salta los defer.
func TestMain(m *testing.M) {
	codigo := m.Run()
	testhelp.Terminar()
	os.Exit(codigo)
}
