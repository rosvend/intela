package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

// casosExentosEnLambda son los campos de httpapi.Casos que cmd/lambda deja a
// proposito sin cablear, con el motivo por el que no puede cablearlos.
//
// Es una lista CERRADA y esa es toda la gracia: el unico sitio donde se puede
// decir "esto va sin cablear" es aqui, con su razon escrita al lado. Una
// omision que no aparezca en esta lista es un descuido, no una decision, y
// [TestLambdaCableaLosMismosCasosQueLaAPI] la convierte en un fallo de CI.
var casosExentosEnLambda = map[string]string{
	"Ingesta": "la boveda de reportes crudos es objetos.Disco y el FS de Lambda " +
		"es de solo lectura salvo /tmp, que se recicla con el contenedor (ADR 0006/0014)",
}

// TestLambdaCableaLosMismosCasosQueLaAPI compara los campos de `httpapi.Casos`
// que cablea cada binario.
//
// Existe porque la omision no falla de forma visible en ningun sitio: el
// binario compila, arranca, sirve todas las demas rutas, y la que falta
// responde 503 con un cuerpo que dice "no esta configurada en esta
// instalacion". Y `cmd/lambda` es el que atiende el TRAFICO REAL --
// `docs/cd.md` pone la API de produccion en esta Lambda, con Amplify
// reescribiendo `/api/*` hacia su Function URL --, asi que un caso de uso sin
// cablear aqui esta caido en produccion y entero en las pruebas.
//
// Sobre `/alertas` ademas es INVISIBLE: `web/src/tablero/ausente.ts` mete el
// 503 en el conjunto AUSENTE y lo pinta como "Sin datos todavia", bajo una
// cabecera que dice que nada se reparte con criticas abiertas. El operador lee
// periodo limpio donde hay un backend desconectado.
func TestLambdaCableaLosMismosCasosQueLaAPI(t *testing.T) {
	enAPI := camposDeCasos(t, "main.go", "../api/main.go")
	enLambda := camposDeCasos(t, "main.go", "main.go")

	if len(enAPI) == 0 {
		t.Fatal("no se encontro ningun campo de httpapi.Casos en cmd/api/main.go: el test no esta mirando lo que cree")
	}

	for _, campo := range enAPI {
		if slices.Contains(enLambda, campo) {
			continue
		}
		motivo, exento := casosExentosEnLambda[campo]
		if !exento {
			t.Errorf("cmd/api cablea %q en httpapi.Casos y cmd/lambda no, y no esta en casosExentosEnLambda.\n"+
				"cmd/lambda es la API de produccion (docs/cd.md): sin cablear, esa ruta responde 503 con trafico real.\n"+
				"Si la omision es deliberada, declarala en casosExentosEnLambda con su motivo; si no, cablea el caso de uso.",
				campo)
			continue
		}
		t.Logf("%q va sin cablear a proposito en cmd/lambda: %s", campo, motivo)
	}

	// Y al reves: una exencion que ya no corresponde a una omision real es
	// prosa que miente. Si alguien cablea Ingesta y no borra la entrada, la
	// lista deja de describir el binario.
	for campo := range casosExentosEnLambda {
		if slices.Contains(enLambda, campo) {
			t.Errorf("%q esta en casosExentosEnLambda pero cmd/lambda SI lo cablea: borra la exencion", campo)
		}
	}
}

// camposDeCasos devuelve los nombres de campo del literal `httpapi.Casos{...}`
// de un main.
//
// Sobre el AST y no con una expresion regular: un `grep` de "Anomalias" pasaria
// con el identificador dentro de un comentario, que es exactamente el estado
// que este test tiene que distinguir de un cableado de verdad.
func camposDeCasos(t *testing.T, nombreLogico, ruta string) []string {
	t.Helper()

	fichero, err := parser.ParseFile(token.NewFileSet(), ruta, nil, 0)
	if err != nil {
		t.Fatalf("parsear %s (%s): %v", nombreLogico, ruta, err)
	}

	var campos []string
	ast.Inspect(fichero, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Casos" {
			return true
		}
		paquete, ok := sel.X.(*ast.Ident)
		if !ok || paquete.Name != "httpapi" {
			return true
		}
		for _, elem := range lit.Elts {
			kv, ok := elem.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			clave, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			campos = append(campos, clave.Name)
		}
		return true
	})
	return campos
}
