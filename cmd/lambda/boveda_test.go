package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Sin bucket la Lambda no arranca: una boveda en /tmp desapareceria con el contenedor (ADR 0006, 0023).
func TestConstruirSinBucketFallaAntesDeConectar(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://nadie@127.0.0.1:1/nada")
	t.Setenv("OBJECT_BUCKET", "")
	_, err := construir()
	if err == nil || !strings.Contains(err.Error(), "OBJECT_BUCKET") {
		t.Fatalf("se esperaba un error que nombre OBJECT_BUCKET, se obtuvo %v", err)
	}
}

// La Lambda nunca monta objetos.Disco: su sistema de ficheros solo deja escribir en /tmp.
func TestLambdaNoUsaLaBovedaEnDisco(t *testing.T) {
	fichero, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsear main.go: %v", err)
	}
	ast.Inspect(fichero, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		paq, ok := sel.X.(*ast.Ident)
		if ok && paq.Name == "objetos" && (sel.Sel.Name == "Disco" || sel.Sel.Name == "Boveda") {
			t.Errorf("cmd/lambda usa objetos.%s; la boveda de produccion es objetos.NuevoS3", sel.Sel.Name)
		}
		return true
	})
}
