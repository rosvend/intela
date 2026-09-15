package normalizacion

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// taquillaEnBase deja la metrica de cine en la moneda base del snapshot.
//
// No es un importe a sumar: la taquilla pondera la bolsa de RD 9.2. Si la
// moneda no se reconoce o no tiene tasa, la fila va a revision CON el valor
// original. Ponerla a cero y dejarla pasar haria que una pelicula en dolares
// no puntuara, en silencio. Multiplicar por la tasa de otra moneda (p. ej.
// EUR × TRM del dolar) es peor: pondera RD 9.2 con una cifra inexplicable.
func taquillaEnBase(valor decimal.Decimal, moneda string, p Parametros) (decimal.Decimal, *Revision) {
	moneda = normalizarCodigoMoneda(moneda)
	base := normalizarCodigoMoneda(p.MonedaBase)

	if valor.IsZero() && moneda == "" {
		return decimal.Zero, nil
	}
	if moneda == "" {
		return decimal.Zero, &Revision{
			Codigo:  CodigoMonedaDesconocida,
			Campo:   "moneda",
			Detalle: "taquilla " + valor.String() + " sin moneda: no se pone a cero",
		}
	}
	if base == "" || moneda == base {
		if base == "" && moneda != "" {
			return decimal.Zero, &Revision{
				Codigo:  CodigoParametroAusente,
				Campo:   "moneda_base",
				Detalle: "taquilla en " + moneda + " y no hay moneda base en el snapshot",
			}
		}
		return valor, nil
	}
	tasa, ok := tasaDe(moneda, p.Tasas)
	if !ok {
		return decimal.Zero, &Revision{
			Codigo:  CodigoMonedaDesconocida,
			Campo:   "moneda",
			Detalle: "moneda " + moneda + " sin tasa en el snapshot: no se pone a cero",
		}
	}
	if !tasa.GreaterThan(decimal.Zero) {
		return decimal.Zero, &Revision{
			Codigo:  CodigoParametroAusente,
			Campo:   "tasa",
			Detalle: "taquilla en " + moneda + " y la tasa a " + base + " no es positiva",
		}
	}
	return valor.Mul(tasa), nil
}

func tasaDe(moneda string, tasas map[string]decimal.Decimal) (decimal.Decimal, bool) {
	if len(tasas) == 0 {
		return decimal.Zero, false
	}
	for codigo, tasa := range tasas {
		if normalizarCodigoMoneda(codigo) == moneda {
			return tasa, true
		}
	}
	return decimal.Zero, false
}

func normalizarCodigoMoneda(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}
