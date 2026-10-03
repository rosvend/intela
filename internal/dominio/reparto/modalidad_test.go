package reparto_test

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestModalidades(t *testing.T) {
	t.Parallel()
	got := reparto.Modalidades()
	want := []reparto.Modalidad{
		reparto.TV, reparto.Cine, reparto.OTT, reparto.Hotel,
		reparto.Teatro, reparto.Transporte, reparto.Suscripcion,
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseModalidad(t *testing.T) {
	t.Parallel()
	for _, m := range reparto.Modalidades() {
		got, err := reparto.ParseModalidad(string(m))
		if err != nil {
			t.Fatalf("%q: %v", m, err)
		}
		if got != m {
			t.Fatalf("got %q want %q", got, m)
		}
	}
	_, err := reparto.ParseModalidad("radio")
	if !errors.Is(err, reparto.ErrModalidadDesconocida) {
		t.Fatalf("err=%v, want ErrModalidadDesconocida", err)
	}
}

// La base de P-18 llega como texto de un parametro: solo las dos medidas que
// puntosCineTeatro sabe usar pasan, y lo demas es un error tipado al congelar
// el snapshot, no una corrida de cine que falla al valorizar (#194).
func TestParseBaseCineTeatro(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{reparto.BaseTaquilla, reparto.BaseEspectadores} {
		got, err := reparto.ParseBaseCineTeatro(ok)
		if err != nil || got != ok {
			t.Errorf("ParseBaseCineTeatro(%q) = %q, %v", ok, got, err)
		}
	}
	for _, malo := range []string{"", "Taquilla", "boletas", " taquilla"} {
		if _, err := reparto.ParseBaseCineTeatro(malo); !errors.Is(err, reparto.ErrBaseCineTeatroDesconocida) {
			t.Errorf("ParseBaseCineTeatro(%q): se esperaba ErrBaseCineTeatroDesconocida, dio %v", malo, err)
		}
	}
}
