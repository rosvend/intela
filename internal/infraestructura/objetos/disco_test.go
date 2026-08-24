package objetos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Las claves que un atacante controla. El periodo sale de un formulario y el
// nombre de multipart.FileHeader.Filename: los dos entran sin sanear en la
// clave que se le pasa al almacen.
func TestPonerRechazaEscapeDelDirectorio(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}

	claves := []string{
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

	for _, clave := range claves {
		t.Run(clave, func(t *testing.T) {
			err := d.Poner(context.Background(), clave, []byte("x"))
			if !errors.Is(err, ErrClaveInvalida) {
				t.Fatalf("clave %q: se esperaba ErrClaveInvalida, se obtuvo %v", clave, err)
			}
		})
	}

	// Nada escrito fuera de la raiz, y nada dentro tampoco.
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

func TestPonerYObtener(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}
	clave := "reportes/2026-01/abc123/parrilla.csv"
	datos := []byte("titulo,emisiones\nX,3\n")

	if err := d.Poner(context.Background(), clave, datos); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	leido, err := d.Obtener(context.Background(), clave)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if string(leido) != string(datos) {
		t.Fatalf("leido %q, esperado %q", leido, datos)
	}
	if _, err := os.Stat(filepath.Join(raiz, "reportes", "2026-01", "abc123", "parrilla.csv")); err != nil {
		t.Fatalf("el fichero no quedo donde toca: %v", err)
	}
}

// ADR 0006: la copia cruda es inmutable. Reescribir una clave ya usada es un
// error, no una actualizacion.
func TestPonerNoSobrescribe(t *testing.T) {
	d := Disco{Dir: t.TempDir()}
	clave := "reportes/2026-01/sha/x.csv"

	if err := d.Poner(context.Background(), clave, []byte("original")); err != nil {
		t.Fatalf("primer Poner: %v", err)
	}
	err := d.Poner(context.Background(), clave, []byte("suplantado"))
	if err == nil {
		t.Fatal("se esperaba error al reescribir una clave existente")
	}

	leido, err := d.Obtener(context.Background(), clave)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if string(leido) != "original" {
		t.Fatalf("el contenido cambio: %q", leido)
	}
}

func TestObtenerInexistente(t *testing.T) {
	d := Disco{Dir: t.TempDir()}
	_, err := d.Obtener(context.Background(), "no/existe.csv")
	if err == nil || !strings.Contains(err.Error(), "no encontrado") {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}
