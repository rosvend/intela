package identificacion

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Resultado de la cascada ADR 0007. Identificacion no toca dinero.
type Resultado struct {
	ObraID    string
	Escalon   string
	Puntaje   decimal.Decimal
	ONI       bool
	Evidencia string
}

type Candidato struct {
	ObraID  string
	Puntaje decimal.Decimal
}

type Entrada struct {
	Fuente     string
	TipoID     string
	ValorID    string
	IDA        string
	EIDR       string
	IMDB       string
	Titulo     string
	TituloOrig string
}

func Cascada(in Entrada, aliasObra, idGlobalObra string, fuzzy []Candidato, umbral decimal.Decimal) Resultado {
	if aliasObra != "" {
		return Resultado{ObraID: aliasObra, Escalon: "alias", Puntaje: decimal.NewFromInt(1), Evidencia: in.Fuente + ":" + in.TipoID + ":" + in.ValorID}
	}
	if idGlobalObra != "" {
		return Resultado{ObraID: idGlobalObra, Escalon: "id_global", Puntaje: decimal.NewFromInt(1), Evidencia: "ida/eidr/imdb"}
	}
	mejor := Candidato{}
	for _, c := range fuzzy {
		if c.Puntaje.GreaterThan(mejor.Puntaje) {
			mejor = c
		}
	}
	if mejor.ObraID != "" && !mejor.Puntaje.LessThan(umbral) {
		return Resultado{ObraID: mejor.ObraID, Escalon: "difuso", Puntaje: mejor.Puntaje, Evidencia: "titulo"}
	}
	return Resultado{ONI: true, Escalon: "oni", Evidencia: "sin_match"}
}

func NormalizarTitulo(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		out = strings.ToLower(s)
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range out {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func SimilitudTitulo(a, b string) decimal.Decimal {
	x, y := NormalizarTitulo(a), NormalizarTitulo(b)
	if x == "" || y == "" {
		return decimal.Zero
	}
	if x == y {
		return decimal.NewFromInt(1)
	}
	bg := func(s string) map[string]int {
		m := map[string]int{}
		p := " " + s + " "
		for i := 0; i < len(p)-1; i++ {
			m[p[i:i+2]]++
		}
		return m
	}
	ba, bb := bg(x), bg(y)
	inter := 0
	sa, sb := 0, 0
	for k, va := range ba {
		sa += va
		if vb, ok := bb[k]; ok {
			if va < vb {
				inter += va
			} else {
				inter += vb
			}
		}
	}
	for _, vb := range bb {
		sb += vb
	}
	if sa+sb == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(2 * inter)).Div(decimal.NewFromInt(int64(sa + sb)))
}
