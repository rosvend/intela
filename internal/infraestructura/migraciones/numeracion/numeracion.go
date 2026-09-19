// Package numeracion comprueba la numeracion goose de las migraciones.
//
// Vive aparte de [github.com/rosvend/intela/migrations]: ese paquete solo
// embebe los .sql y viaja en los binarios de produccion (cmd/migrate,
// cmd/lambda-migrate). Esta herramienta importa os/exec para leer el arbol
// de main y solo la usan las pruebas de CI (#110).
package numeracion

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// versionEnNombre lee el entero que goose toma del nombre del fichero.
//
// goose no mira el contenido: la version es el prefijo numerico de
// `NNNNN_descripcion.sql`. Dos ficheros con el mismo prefijo y distinto
// sufijo no chocan en git y hacen que goose entre en panic al cargar el
// directorio.
var versionEnNombre = regexp.MustCompile(`^(\d+)_.+\.sql$`)

// VersionDe devuelve la version goose que lleva nombre, o false si el nombre
// no sigue el patron `NNNNN_descripcion.sql`.
func VersionDe(nombre string) (int64, bool) {
	m := versionEnNombre.FindStringSubmatch(path.Base(nombre))
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// SinVersion lista los .sql de fsys cuyo nombre goose no puede parsear.
//
// goose.CollectMigrations falla con "could not parse SQL migration file"
// ante nombres como `arreglo.sql`. PorVersion los ignora; esta lista es la
// que hace fallar CI antes del despliegue.
func SinVersion(fsys fs.FS) ([]string, error) {
	entradas, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var malos []string
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if _, ok := VersionDe(e.Name()); !ok {
			malos = append(malos, e.Name())
		}
	}
	sort.Strings(malos)
	return malos, nil
}

// PorVersion agrupa los .sql de fsys por su version goose.
//
// Una entrada con len > 1 es el panic "duplicate version N" que goose suelta
// en el despliegue. Los ficheros cuyo nombre no parsea no entran aqui: ver
// [SinVersion].
func PorVersion(fsys fs.FS) (map[int64][]string, error) {
	entradas, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	out := map[int64][]string{}
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, ok := VersionDe(e.Name())
		if !ok {
			continue
		}
		out[v] = append(out[v], e.Name())
	}
	for v := range out {
		sort.Strings(out[v])
	}
	return out, nil
}

// Duplicadas devuelve las versiones que aparecen en mas de un fichero.
func Duplicadas(fsys fs.FS) (map[int64][]string, error) {
	todas, err := PorVersion(fsys)
	if err != nil {
		return nil, err
	}
	dup := map[int64][]string{}
	for v, nombres := range todas {
		if len(nombres) > 1 {
			dup[v] = nombres
		}
	}
	return dup, nil
}

// BaseRef es el arbol de git del que se deriva la lista de migraciones ya
// aplicadas en produccion.
//
// Por defecto `origin/main`: tras un merge a main el despliegue corre goose
// up, asi que lo que main tiene es lo que produccion tiene (o esta a punto de
// tener). En CI se apunta al SHA base del PR para no depender del fetch de
// origin/main.
//
// No se puede derivar del FS de ESTA rama: compararia el arbol consigo mismo
// y la prueba del hueco pasaria siempre.
//
// Si origin/main no esta en el remoto local, falla: un fallback silencioso a
// `main` local podia dar una version aplicada por debajo de la real y dejar
// pasar el hueco (#110).
func BaseRef() string {
	if r := strings.TrimSpace(os.Getenv("MIGRACIONES_BASE_REF")); r != "" {
		return r
	}
	return "origin/main"
}

// DesplegadasEn lista los ficheros `migrations/*.sql` presentes en ref.
//
// Son el hecho sobre el despliegue que la lista a mano dejo de seguir: cuando
// main avanza, esta lista avanza sin que nadie la edite.
func DesplegadasEn(ref string) ([]string, error) {
	if ref == "" {
		ref = BaseRef()
	}
	return listarSQLEn(ref)
}

func listarSQLEn(ref string) ([]string, error) {
	raiz, err := repoRoot()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "ls-tree", "-r", "--name-only", ref, "--", "migrations/")
	cmd.Dir = raiz
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		return nil, fmt.Errorf(
			"listar migraciones de %s: %w%s\n"+
				"hace falta el ref de main (git fetch origin main) "+
				"o exportar MIGRACIONES_BASE_REF a un commit alcanzable",
			ref, err, formatoStderr(stderr),
		)
	}
	var nombres []string
	for _, linea := range strings.Split(string(out), "\n") {
		linea = strings.TrimSpace(linea)
		if linea == "" || !strings.HasSuffix(linea, ".sql") {
			continue
		}
		nombres = append(nombres, path.Base(linea))
	}
	sort.Strings(nombres)
	if len(nombres) == 0 {
		return nil, fmt.Errorf("el ref %s no tiene migrations/*.sql", ref)
	}
	return nombres, nil
}

// VersionAplicada es la mayor version goose entre los nombres dados.
//
// Recibe la lista ya resuelta por [DesplegadasEn]: asi el maximo del mensaje
// de error es el mismo que clasifico las nuevas, sin un segundo git ls-tree.
//
// Una migracion NUEVA (fichero que desplegadas no tiene) con version <= esta
// es el fallo "found N missing migrations before current version X" que goose
// suelta con allowMissing = false.
func VersionAplicada(desplegadas []string) (int64, error) {
	var maxima int64
	for _, n := range desplegadas {
		v, ok := VersionDe(n)
		if !ok {
			return 0, fmt.Errorf("nombre de migracion ilegible entre las desplegadas: %s", n)
		}
		if v > maxima {
			maxima = v
		}
	}
	return maxima, nil
}

// NuevasPorDebajo son los .sql de fsys que no estan en desplegadas y cuya
// version es menor o igual que la maxima de desplegadas.
//
// "Nueva" se decide por nombre de fichero: un renombrado del mismo numero
// cuenta como nueva y falla aqui. Es deliberado frente a adivinar igualdad
// por version -renumerar al mergear es la regla-.
func NuevasPorDebajo(fsys fs.FS, desplegadas []string) ([]string, error) {
	maxima, err := VersionAplicada(desplegadas)
	if err != nil {
		return nil, err
	}
	ya := map[string]struct{}{}
	for _, n := range desplegadas {
		ya[n] = struct{}{}
	}

	todas, err := PorVersion(fsys)
	if err != nil {
		return nil, err
	}
	var malas []string
	for v, nombres := range todas {
		if v > maxima {
			continue
		}
		for _, n := range nombres {
			if _, ok := ya[n]; ok {
				continue
			}
			malas = append(malas, n)
		}
	}
	sort.Strings(malas)
	return malas, nil
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func formatoStderr(b []byte) string {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return ""
	}
	return "\n" + string(b)
}
