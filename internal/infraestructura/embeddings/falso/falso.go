// Package falso es un MotorEmbeddings determinista y sin red (hashing de palabras) para pruebas y desarrollo local.
package falso

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Modelo y Dimension identifican este motor en el indice: no se mezcla con vectores de un proveedor real.
const (
	Modelo    = "falso-hash-256"
	Dimension = 256
)

// Motor captura solo coincidencia lexica, no parafraseo; basta para probar el flujo de punta a punta.
type Motor struct{}

// Embeber reparte cada palabra normalizada en una dimension por hash y normaliza a norma 1.
func (Motor) Embeber(_ context.Context, texto string) (aplicacion.Embedding, error) {
	v := make([]float32, Dimension)
	for _, p := range palabras(texto) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(p))
		v[h.Sum32()%Dimension]++
	}
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n > 0 {
		for i := range v {
			v[i] = float32(float64(v[i]) / math.Sqrt(n))
		}
	}
	return aplicacion.Embedding{Modelo: Modelo, Vector: v}, nil
}

// palabras pasa a minusculas, quita tildes y descarta palabras de menos de 3 letras.
func palabras(s string) []string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	campos := strings.FieldsFunc(b.String(), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := campos[:0]
	for _, c := range campos {
		if len([]rune(c)) >= 3 {
			out = append(out, c)
		}
	}
	return out
}
