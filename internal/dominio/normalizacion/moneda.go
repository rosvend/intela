package normalizacion

import (
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// taquillaEnBase deja la metrica de cine en la moneda base del snapshot.
//
// No es un importe a sumar: la taquilla pondera la bolsa de RD 9.2. Si la
// moneda no se reconoce, la fila va a revision CON el valor original. Ponerla
// a cero y dejarla pasar haria que una pelicula en dolares no puntuara, en
// silencio.
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
	if !monedaReconocida(moneda, p.MonedasReconocidas) {
		return decimal.Zero, &Revision{
			Codigo:  CodigoMonedaDesconocida,
			Campo:   "moneda",
			Detalle: "moneda " + moneda + " no reconocida: no se pone a cero",
		}
	}
	if base == "" || moneda == base {
		return valor, nil
	}
	if !p.TRM.GreaterThan(decimal.Zero) {
		return decimal.Zero, &Revision{
			Codigo:  CodigoParametroAusente,
			Campo:   "trm",
			Detalle: "taquilla en " + moneda + " y no hay TRM en el snapshot para pasarla a " + base,
		}
	}
	return valor.Mul(p.TRM), nil
}

func monedaReconocida(moneda string, reconocidas []string) bool {
	if len(reconocidas) == 0 {
		return false
	}
	for _, r := range reconocidas {
		if normalizarCodigoMoneda(r) == moneda {
			return true
		}
	}
	return false
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
