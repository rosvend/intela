package semilla

import (
	"os"
	"testing"

	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

func TestMain(m *testing.M) {
	codigo := m.Run()
	testhelp.Terminar()
	os.Exit(codigo)
}
