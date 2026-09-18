package reparto

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// precisionDinero es la escala de importes en pesos (NUMERIC(18,2)).
const precisionDinero int32 = 2

// precisionPuntos guarda puntos y valor punto con mas escala que el dinero
// para no arrastrar el residuo de la division al importe de cada obra antes
// de redondear.
const precisionPuntos int32 = 8

// redondearDinero aplica la regla declarada del paquete: Round half-away-from-zero
// a dos decimales (shopspring/decimal.Round). El residuo de una asignacion
// proporcional NO se absorbe en la ultima linea: queda en Resultado.Residuo
// (ADR 0005).
func redondearDinero(d decimal.Decimal) decimal.Decimal {
	return d.Round(precisionDinero)
}

// repartirProporcional reparte `neto` entre claves ordenadas segun `pesos`.
// Devuelve importes ya redondeados a dinero y el residuo explicito
// (neto - suma(importes)).
//
// claves debe venir ordenada de forma estable. pesos en cero no reciben nada.
func repartirProporcional(neto decimal.Decimal, claves []string, pesos map[string]decimal.Decimal) (importes map[string]decimal.Decimal, residuo decimal.Decimal) {
	importes = make(map[string]decimal.Decimal, len(claves))
	total := decimal.Zero
	for _, k := range claves {
		if p, ok := pesos[k]; ok && p.GreaterThan(decimal.Zero) {
			total = total.Add(p)
		}
	}
	if total.IsZero() || neto.IsZero() {
		for _, k := range claves {
			importes[k] = decimal.Zero
		}
		return importes, neto
	}

	asignado := decimal.Zero
	for _, k := range claves {
		p := pesos[k]
		if p.LessThanOrEqual(decimal.Zero) {
			importes[k] = decimal.Zero
			continue
		}
		imp := redondearDinero(neto.Mul(p).Div(total))
		importes[k] = imp
		asignado = asignado.Add(imp)
	}
	residuo = neto.Sub(asignado)
	return importes, residuo
}

func pctDe(bruto, pct decimal.Decimal) decimal.Decimal {
	return redondearDinero(bruto.Mul(pct).Div(decimal.NewFromInt(100)))
}

func exigirPositivo(nombre string, v decimal.Decimal) error {
	if v.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: %s", ErrParametroAusente, nombre)
	}
	return nil
}

func exigirNoNegativo(nombre string, v decimal.Decimal) error {
	if v.IsNegative() {
		return fmt.Errorf("%w: %s", ErrParametroAusente, nombre)
	}
	return nil
}
