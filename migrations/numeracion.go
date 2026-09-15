package migrations

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

// PorVersion agrupa los .sql de fsys por su version goose.
//
// Una entrada con len > 1 es el panic "duplicate version N" que goose suelta
// en el despliegue. Los ficheros cuyo nombre no parsea se ignoran: no son
// migraciones.
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
//
// Si ref es el por defecto (`origin/main`) y no esta en el remoto local, se
// intenta `main` antes de fallar: en un clon fresco sin fetch suele existir
// la rama local y no el remote-tracking.
func DesplegadasEn(ref string) ([]string, error) {
	if ref == "" {
		ref = BaseRef()
	}
	nombres, err := listarSQLEn(ref)
	if err != nil && ref == "origin/main" {
		nombres, err = listarSQLEn("main")
		if err == nil {
			return nombres, nil
		}
	}
	return nombres, err
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

// VersionAplicada es la mayor version goose entre las migraciones de ref.
//
// Una migracion NUEVA (fichero que ref no tiene) con version <= VersionAplicada
// es el fallo "found N missing migrations before current version X" que goose
// suelta con allowMissing = false.
func VersionAplicada(ref string) (int64, error) {
	nombres, err := DesplegadasEn(ref)
	if err != nil {
		return 0, err
	}
	var max int64
	for _, n := range nombres {
		v, ok := VersionDe(n)
		if !ok {
			return 0, fmt.Errorf("nombre de migracion ilegible en %s: %s", ref, n)
		}
		if v > max {
			max = v
		}
	}
	return max, nil
}

// NuevasPorDebajo son los .sql de fsys que no estan en desplegadas y cuya
// version es menor o igual que la maxima de desplegadas.
func NuevasPorDebajo(fsys fs.FS, desplegadas []string) ([]string, error) {
	ya := map[string]struct{}{}
	var max int64
	for _, n := range desplegadas {
		ya[n] = struct{}{}
		v, ok := VersionDe(n)
		if !ok {
			return nil, fmt.Errorf("nombre de migracion ilegible entre las desplegadas: %s", n)
		}
		if v > max {
			max = v
		}
	}

	todas, err := PorVersion(fsys)
	if err != nil {
		return nil, err
	}
	var malas []string
	for v, nombres := range todas {
		if v > max {
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
