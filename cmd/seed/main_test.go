package main

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestEjecutarSinDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := ejecutar(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || err.Error() != "falta DATABASE_URL" {
		t.Fatalf("se esperaba falta DATABASE_URL, se obtuvo %v", err)
	}
}

// TestDirObjetosPorDefectoEsRelativo: el defecto era `/data/objetos`, una ruta
// de CONTENEDOR, asi que el `go run ./cmd/seed` que documenta ARRANQUE.md y el
// `make seed` fallaban con EACCES en cualquier maquina de desarrollo -nadie
// puede crear /data-. Y fallaban DESPUES de que insertarPadron confirmara, asi
// que la base quedaba con obras y sin reportes y el siguiente intento respondia
// "semilla a medias": el comando documentado dejaba la base inservible en el
// primer uso.
//
// La propiedad que hay que conservar es que la ruta por defecto sea relativa al
// directorio de trabajo. En contenedor la fija OBJECT_DIR, que es lo que hace
// docker-compose.yml.
func TestDirObjetosPorDefectoEsRelativo(t *testing.T) {
	if filepath.IsAbs(dirObjetosPorDefecto) {
		t.Fatalf("OBJECT_DIR por defecto = %q: una ruta absoluta no la puede crear un usuario de desarrollo",
			dirObjetosPorDefecto)
	}
}
