package postgres

import (
	"os"
	"testing"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// TestMain apaga el contenedor cuando termina el binario de pruebas.
//
// os.Exit se salta los defer, asi que el apagado va explicito entre m.Run() y
// la salida.
func TestMain(m *testing.M) {
	codigo := m.Run()
	testhelp.Terminar()
	os.Exit(codigo)
}
