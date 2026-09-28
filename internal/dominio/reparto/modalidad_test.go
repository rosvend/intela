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
