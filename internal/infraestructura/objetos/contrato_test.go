package objetos

import (
	"context"
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

// probarContrato corre los casos que todo aplicacion.AlmacenObjetos tiene que cumplir.
func probarContrato(t *testing.T, nuevo func(t *testing.T) aplicacion.AlmacenObjetos) {
	t.Helper()
	ctx := context.Background()

	t.Run("PonerYObtener", func(t *testing.T) {
		a := nuevo(t)
		clave := "reportes/2026-01/abc123/parrilla.csv"
		datos := []byte("titulo,emisiones\nX,3\n")
		if err := a.Poner(ctx, clave, datos); err != nil {
			t.Fatalf("Poner: %v", err)
		}
		leido, err := a.Obtener(ctx, clave)
		if err != nil {
			t.Fatalf("Obtener: %v", err)
		}
		if string(leido) != string(datos) {
			t.Fatalf("leido %q, esperado %q", leido, datos)
		}
	})

	// ADR 0006: reescribir una clave es ErrObjetoYaExiste y el contenido no cambia.
	t.Run("NoSobrescribe", func(t *testing.T) {
		a := nuevo(t)
		clave := "reportes/2026-01/sha/x.csv"
		if err := a.Poner(ctx, clave, []byte("original")); err != nil {
			t.Fatalf("primer Poner: %v", err)
		}
		err := a.Poner(ctx, clave, []byte("suplantado"))
		if !errors.Is(err, aplicacion.ErrObjetoYaExiste) {
			t.Fatalf("se esperaba ErrObjetoYaExiste, se obtuvo %v", err)
		}
		leido, err := a.Obtener(ctx, clave)
		if err != nil {
			t.Fatalf("Obtener: %v", err)
		}
		if string(leido) != "original" {
			t.Fatalf("el contenido cambio: %q", leido)
		}
	})

	t.Run("ObtenerInexistente", func(t *testing.T) {
		a := nuevo(t)
		if _, err := a.Obtener(ctx, "no/existe.csv"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})

	// La compensacion del alta tiene que poder llamar a Borrar dos veces.
	t.Run("BorrarQuitaElObjetoYEsIdempotente", func(t *testing.T) {
		a := nuevo(t)
		clave := "afiliaciones/afil-1/rut"
		if err := a.Poner(ctx, clave, []byte("%PDF")); err != nil {
			t.Fatalf("Poner: %v", err)
		}
		if err := a.Borrar(ctx, clave); err != nil {
			t.Fatalf("Borrar: %v", err)
		}
		if _, err := a.Obtener(ctx, clave); !errors.Is(err, aplicacion.ErrNoEncontrado) {
			t.Fatalf("tras borrar se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
		if err := a.Borrar(ctx, clave); err != nil {
			t.Fatalf("borrar de nuevo: %v", err)
		}
	})

	// El periodo y el nombre del fichero entran sin sanear en la clave.
	t.Run("RechazaClavesInvalidas", func(t *testing.T) {
		a := nuevo(t)
		for _, clave := range clavesInvalidas {
			if err := a.Poner(ctx, clave, []byte("x")); !errors.Is(err, ErrClaveInvalida) {
				t.Errorf("clave %q: se esperaba ErrClaveInvalida, se obtuvo %v", clave, err)
			}
		}
	})
}

var clavesInvalidas = []string{
	"reportes/../../../etc/passwd",
	"../fuera.txt",
	"reportes/2026/../../../../tmp/x",
	"/etc/passwd",
	`reportes\..\..\fuera.txt`,
	"reportes/./../../fuera",
	"",
	"reportes//doble",
	"reportes/2026-01/sub dir/x.csv",
}
