package main

import (
	"os/exec"
	"strings"
	"testing"
)

// cmd/lambda deja Ingesta y Admision sin cablear: la boveda de hoy es
// objetos.Disco, y el sistema de ficheros de Lambda no la puede hospedar.
// Restaurar ese cableado sigue compilando y los tests de httpapi siguen
// verdes, porque ellos solo comprueban el 503 cuando Admision es nil.
func TestLambdaNoDependeDeLaBovedaEnDisco(t *testing.T) {
	salida, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, salida)
	}
	const boveda = "github.com/rosvend/intela/internal/infraestructura/objetos"
	for _, pkg := range strings.Split(string(salida), "\n") {
		if pkg == boveda {
			t.Fatalf("cmd/lambda importa %s; Admision e Ingesta tienen que quedar sin cablear hasta el adaptador de S3", boveda)
		}
	}
}
