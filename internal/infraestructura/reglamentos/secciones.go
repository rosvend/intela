// Package reglamentos lee docs/reglamentos/**/*.md y los parte en numerales citables (RD 9.1.1).
package reglamentos

import (
	"bufio"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rosvend/intela/internal/aplicacion"
)

// abreviaturas es la convencion de citas de docs/reglamentos/README.md.
var abreviaturas = map[string]string{
	"Reglamento de Distribucion":          "RD",
	"Reglamento de Tarifas":               "RT",
	"Reglamento de Socios":                "RS",
	"Reglamento de Anticipos a Afiliados": "RA",
}

var (
	reNumeral = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+(\S.*)$`)
	reSeccion = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
)

// Leer devuelve una seccion por numeral; los archivos sin numeral (indice, presentacion) no son citables y se omiten.
func Leer(fsys fs.FS) ([]aplicacion.SeccionReglamento, error) {
	var out []aplicacion.SeccionReglamento
	err := fs.WalkDir(fsys, ".", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(ruta) != ".md" {
			return err
		}
		datos, err := fs.ReadFile(fsys, ruta)
		if err != nil {
			return err
		}
		ss, err := partir(string(datos))
		if err != nil {
			return fmt.Errorf("%s: %w", ruta, err)
		}
		out = append(out, ss...)
		return nil
	})
	return out, err
}

func partir(md string) ([]aplicacion.SeccionReglamento, error) {
	meta, cuerpo, ok := frontmatter(md)
	if !ok {
		return nil, nil
	}
	m := reSeccion.FindStringSubmatch(meta["seccion"])
	if m == nil {
		return nil, nil
	}
	abrev, ok := abreviaturas[meta["reglamento"]]
	if !ok {
		return nil, fmt.Errorf("reglamento %q sin abreviatura de cita", meta["reglamento"])
	}
	nombre := strings.TrimSpace(meta["reglamento"] + " " + meta["version"])

	var (
		out    []aplicacion.SeccionReglamento
		actual = aplicacion.SeccionReglamento{Cita: abrev + " " + m[1], Reglamento: nombre, Titulo: m[2]}
		ultimo = []int{atoi(m[1])}
		texto  strings.Builder
	)
	cerrar := func() {
		actual.Texto = strings.TrimSpace(texto.String())
		// Un literal sin cuerpo (RD 5.1) tiene su contenido en el titulo; un capitulo sin cuerpo (RD 9) solo agrupa.
		if actual.Texto == "" && actual.Cita != abrev+" "+m[1] {
			actual.Texto = actual.Titulo
		}
		if actual.Texto != "" {
			out = append(out, actual)
		}
		texto.Reset()
	}
	sc := bufio.NewScanner(strings.NewReader(cuerpo))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		linea := sc.Text()
		if strings.HasPrefix(linea, "# ") {
			continue
		}
		// Un numeral abre seccion solo si cuelga de la del archivo y avanza: una referencia hacia atras es texto.
		if n := reNumeral.FindStringSubmatch(strings.TrimSpace(linea)); n != nil && strings.HasPrefix(n[1], m[1]+".") {
			if partes := componentes(n[1]); slices.Compare(partes, ultimo) > 0 {
				cerrar()
				actual = aplicacion.SeccionReglamento{Cita: abrev + " " + n[1], Reglamento: nombre, Titulo: strings.TrimSpace(n[2])}
				ultimo = partes
				continue
			}
		}
		texto.WriteString(linea)
		texto.WriteByte('\n')
	}
	cerrar()
	return out, sc.Err()
}

// frontmatter separa el bloque --- clave: valor --- del cuerpo.
func frontmatter(md string) (map[string]string, string, bool) {
	resto, ok := strings.CutPrefix(md, "---\n")
	if !ok {
		return nil, "", false
	}
	bloque, cuerpo, ok := strings.Cut(resto, "\n---\n")
	if !ok {
		return nil, "", false
	}
	meta := map[string]string{}
	for _, l := range strings.Split(bloque, "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok {
			meta[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return meta, cuerpo, true
}

func componentes(numeral string) []int {
	var out []int
	for _, p := range strings.Split(numeral, ".") {
		out = append(out, atoi(p))
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
