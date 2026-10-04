package objetos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

func TestDiscoCumpleElContrato(t *testing.T) {
	probarContrato(t, func(t *testing.T) aplicacion.AlmacenObjetos { return Disco{Dir: t.TempDir()} })
}

// Una clave invalida no deja nada escrito, ni fuera de la raiz ni dentro.
func TestPonerRechazaEscapeDelDirectorio(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}
	for _, clave := range clavesInvalidas {
		_ = d.Poner(context.Background(), clave, []byte("x"))
	}
	var vistos []string
	_ = filepath.Walk(raiz, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			vistos = append(vistos, p)
		}
		return nil
	})
	if len(vistos) != 0 {
		t.Fatalf("no se debio escribir nada, se escribio: %v", vistos)
	}
}

func TestPonerDejaElFicheroBajoLaClave(t *testing.T) {
	raiz := t.TempDir()
	if err := (Disco{Dir: raiz}).Poner(context.Background(), "reportes/2026-01/abc123/parrilla.csv", []byte("x")); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	if _, err := os.Stat(filepath.Join(raiz, "reportes", "2026-01", "abc123", "parrilla.csv")); err != nil {
		t.Fatalf("el fichero no quedo donde toca: %v", err)
	}
}
