package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"testing"
)

// sembrar-dataset en produccion escribe en S3: lee OBJECT_BUCKET y el modulo migrations se lo inyecta (#182).
func TestSembrarDatasetUsaElBucketQueInyectaTerraform(t *testing.T) {
	fichero, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsear main.go: %v", err)
	}
	leeBucket, usaBoveda := false, false
	for _, d := range fichero.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "sembrarDataset" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Value == `"OBJECT_BUCKET"` {
				leeBucket = true
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Boveda" {
				usaBoveda = true
			}
			return true
		})
	}
	if !leeBucket || !usaBoveda {
		t.Fatalf("sembrarDataset tiene que elegir la boveda con objetos.Boveda y OBJECT_BUCKET (lee=%v, boveda=%v)", leeBucket, usaBoveda)
	}

	modulo, err := os.ReadFile("../../infra/modules/migrations/main.tf")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?m)^\s*OBJECT_BUCKET\s*=`).Match(modulo) {
		t.Fatal("infra/modules/migrations/main.tf no inyecta OBJECT_BUCKET: el seed caeria en /tmp")
	}
}
