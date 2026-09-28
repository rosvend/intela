package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// direccionesONI son las variables que, vacias, hacen que produccion no pueda
// publicar ningun periodo: ValidarMetadatos devuelve ErrDireccionAusente y
// POST /oni/publicaciones responde 500. No llevan valor por defecto en el
// codigo (una direccion inventada en la pagina publica es peor que el 500)
// y tampoco en el modulo: el valor vive en infra/envs.
var direccionesONI = []string{
	"ONI_DIRECCION_FISICA",
	"ONI_DIRECCION_ELECTRONICA",
}

// TestConstruirDeclaraSuEntornoEnElModuloAPI falla si cmd/lambda lee una
// variable que el modulo api no inyecta.
//
// CI en verde no lo veia: compose y los tests si definen
// ONI_DIRECCION_FISICA y ONI_DIRECCION_ELECTRONICA, y la Lambda de
// produccion las leia con el predeterminado vacio. El listado publico
// (R-18) quedaba sin nada que mostrar.
func TestConstruirDeclaraSuEntornoEnElModuloAPI(t *testing.T) {
	raiz := raizDelRepo(t)
	leidas := variablesDeConstruir(t, filepath.Join(raiz, "cmd", "lambda", "main.go"))
	if len(leidas) == 0 {
		t.Fatal("construir() no parece leer ninguna variable: el test no esta mirando lo que cree")
	}
	for _, clave := range direccionesONI {
		if !slices.Contains(leidas, clave) {
			t.Fatalf("construir() ya no lee %s; si dejo de ser configuracion, borra la entrada de direccionesONI", clave)
		}
	}

	modulo := filepath.Join(raiz, "infra", "modules", "api", "main.tf")
	entorno := mapaEntorno(t, modulo)
	if len(entorno) == 0 {
		t.Fatal("el modulo api no declara bloque environment: el test no esta mirando lo que cree")
	}
	for _, clave := range leidas {
		if _, ok := entorno[clave]; !ok {
			t.Errorf("construir() lee %s y infra/modules/api/main.tf no la declara en environment", clave)
		}
	}

	cuerpoVariables := leer(t, filepath.Join(raiz, "infra", "modules", "api", "variables.tf"))
	for _, clave := range direccionesONI {
		nombreVar, ok := strings.CutPrefix(strings.TrimSpace(entorno[clave]), "var.")
		if !ok || nombreVar == "" || strings.ContainsAny(nombreVar, " \t\"") {
			t.Errorf("%s tiene que salir de una variable del modulo (var.<nombre>), no de %q", clave, entorno[clave])
			continue
		}
		bloque := bloquePorAncla(t, cuerpoVariables, `variable "`+nombreVar+`"`)
		if tieneDefault(bloque) {
			t.Errorf("variable %q no puede tener default: un valor por defecto publicaria una direccion que no es la del entorno", nombreVar)
		}
	}

	envs, err := filepath.Glob(filepath.Join(raiz, "infra", "envs", "*", "main.tf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) == 0 {
		t.Fatal("no hay infra/envs/*/main.tf")
	}
	for _, env := range envs {
		mod := bloquePorAncla(t, leer(t, env), `module "api"`)
		rel, _ := filepath.Rel(raiz, env)
		for _, clave := range direccionesONI {
			nombreVar := strings.TrimPrefix(strings.TrimSpace(entorno[clave]), "var.")
			valor, ok := asignacion(mod, nombreVar)
			if !ok {
				t.Errorf("%s: module \"api\" no asigna %s", rel, nombreVar)
				continue
			}
			if valor == "" {
				t.Errorf("%s: %s va vacia; POST /oni/publicaciones responderia 500", rel, nombreVar)
			}
		}
	}
}

func raizDelRepo(t *testing.T) string {
	t.Helper()
	_, archivo, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo localizar el test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(archivo), "..", ".."))
}

// variablesDeConstruir lista las claves que construir() lee con config.*.
//
// Sobre el AST: un comentario que mencione config.Cadena("ALGO") no cuenta,
// y una llamada cuya clave no sea un literal hace fallar el test en vez de
// dejar pasar una variable que el despliegue no inyecta.
func variablesDeConstruir(t *testing.T, ruta string) []string {
	t.Helper()
	fset := token.NewFileSet()
	fichero, err := parser.ParseFile(fset, ruta, nil, 0)
	if err != nil {
		t.Fatalf("parsear %s: %v", ruta, err)
	}
	var fn *ast.FuncDecl
	for _, d := range fichero.Decls {
		decl, ok := d.(*ast.FuncDecl)
		if ok && decl.Name.Name == "construir" && decl.Body != nil {
			fn = decl
			break
		}
	}
	if fn == nil {
		t.Fatal("no se encontro func construir() en " + ruta)
	}

	ayudantes := map[string]bool{
		"Cadena": true, "Duracion": true, "Lista": true, "Entero": true, "Bool": true,
	}
	var claves []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		llamada, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := llamada.Fun.(*ast.SelectorExpr)
		if !ok || !ayudantes[sel.Sel.Name] {
			return true
		}
		paq, ok := sel.X.(*ast.Ident)
		if !ok || paq.Name != "config" {
			return true
		}
		if len(llamada.Args) == 0 {
			t.Fatalf("config.%s sin argumentos", sel.Sel.Name)
		}
		lit, ok := llamada.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			t.Fatalf("config.%s usa una clave que no es un literal: el test no puede comprobar que el modulo api la declara", sel.Sel.Name)
		}
		claves = append(claves, strings.Trim(lit.Value, `"`))
		return true
	})
	return claves
}

var claveEntorno = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*=\s*(.+)$`)

func mapaEntorno(t *testing.T, ruta string) map[string]string {
	t.Helper()
	bloque := bloquePorAncla(t, leer(t, ruta), "environment =")
	out := map[string]string{}
	for _, linea := range strings.Split(bloque, "\n") {
		linea = strings.TrimSpace(linea)
		m := claveEntorno.FindStringSubmatch(linea)
		if m == nil {
			continue
		}
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

func leer(t *testing.T, ruta string) string {
	t.Helper()
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer %s: %v", ruta, err)
	}
	return string(b)
}

// bloquePorAncla devuelve el cuerpo entre llaves del primer bloque cuya
// apertura contiene ancla. No interpreta cadenas: en estos .tf las llaves
// viven en la estructura, no dentro de un valor.
func bloquePorAncla(t *testing.T, src, ancla string) string {
	t.Helper()
	i := strings.Index(src, ancla)
	if i < 0 {
		t.Fatalf("no se encontro %q", ancla)
	}
	abre := strings.Index(src[i:], "{")
	if abre < 0 {
		t.Fatalf("%q no abre un bloque", ancla)
	}
	inicio := i + abre
	profundidad := 0
	for j := inicio; j < len(src); j++ {
		switch src[j] {
		case '{':
			profundidad++
		case '}':
			profundidad--
			if profundidad == 0 {
				return src[inicio+1 : j]
			}
		}
	}
	t.Fatalf("bloque %q sin cierre", ancla)
	return ""
}

func tieneDefault(bloque string) bool {
	for _, linea := range strings.Split(bloque, "\n") {
		if strings.HasPrefix(strings.TrimSpace(linea), "default") && strings.Contains(linea, "=") {
			return true
		}
	}
	return false
}

var asignacionHCL = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_]+)\s*=\s*"([^"]*)"\s*$`)

func asignacion(bloque, nombre string) (string, bool) {
	for _, m := range asignacionHCL.FindAllStringSubmatch(bloque, -1) {
		if m[1] == nombre {
			return strings.TrimSpace(m[2]), true
		}
	}
	return "", false
}
