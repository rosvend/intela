package oni

import (
	"errors"
	"testing"
)

func TestAnclarFechaSeRegistraUnaSolaVez(t *testing.T) {
	t.Parallel()

	primera, err := AnclarFecha("", "2026-08-31T12:00:00Z")
	if err != nil {
		t.Fatalf("AnclarFecha: %v", err)
	}
	if primera != "2026-08-31T12:00:00Z" {
		t.Fatalf("primera = %q", primera)
	}

	// Un candidato distinto NO sustituye: reescribir la fecha resetearia
	// los tres anos de R-19.
	segunda, err := AnclarFecha(primera, "2029-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("AnclarFecha (segunda): %v", err)
	}
	if segunda != primera {
		t.Fatalf("la fecha anclada cambio de %q a %q", primera, segunda)
	}
}

func TestAnclarFechaSinCandidatoEsError(t *testing.T) {
	t.Parallel()

	_, err := AnclarFecha("", "  ")
	if !errors.Is(err, ErrFechaAusente) {
		t.Fatalf("se esperaba ErrFechaAusente, se obtuvo %v", err)
	}
}

func TestAnclarFechaConservaLaExistenteSiElCandidatoVieneVacio(t *testing.T) {
	t.Parallel()

	got, err := AnclarFecha("2026-01-01T00:00:00Z", "")
	if !errors.Is(err, ErrFechaAusente) {
		t.Fatalf("se esperaba ErrFechaAusente, se obtuvo %v", err)
	}
	if got != "2026-01-01T00:00:00Z" {
		t.Fatalf("no se debe perder la fecha ya anclada: %q", got)
	}
}
