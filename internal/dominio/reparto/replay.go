package reparto

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// DistribuirSobreProporciones reparte `monto` sobre las proporciones de una
// corrida ya cerrada, sin revalorizar nada (RD 14.4, RD 10.1). Es la unica
// operacion que libera una reserva o reparte un rendimiento: ambos casos leen
// las lineas persistidas de un [Resultado] pasado y aplican el mismo peso que
// ya recibieron, contra el monto nuevo.
//
// El peso de cada linea es su Importe ORIGINAL, no su Porcentaje: el
// porcentaje es relativo a su propia obra, y lo que RD 14.4 pide es la
// proporcion en que se distribuyo el recaudo completo del que salio la
// reserva. Porcentaje se conserva intacto en la salida por trazabilidad
// (RD 16): dice de que declaracion vino la linea, no cuanto le toca ahora.
//
// El residuo de redondeo es explicito (ADR 0005), igual que en el motor: no
// se absorbe en la ultima linea.
func DistribuirSobreProporciones(monto decimal.Decimal, originales []LineaTitular) ([]LineaTitular, decimal.Decimal, error) {
	if monto.IsNegative() {
		return nil, decimal.Zero, fmt.Errorf("%w: monto a distribuir negativo", ErrRepartoInvalido)
	}

	claves := make([]string, len(originales))
	pesos := make(map[string]decimal.Decimal, len(originales))
	porClave := make(map[string]LineaTitular, len(originales))
	for i, o := range originales {
		clave := fmt.Sprintf("%s\x00%s\x00%d", o.ObraID, o.TitularID, i)
		claves[i] = clave
		pesos[clave] = o.Importe
		porClave[clave] = o
	}

	importes, residuo := repartirProporcional(monto, claves, pesos)

	nuevas := make([]LineaTitular, len(originales))
	for i, clave := range claves {
		base := porClave[clave]
		nuevas[i] = LineaTitular{
			ObraID:     base.ObraID,
			TitularID:  base.TitularID,
			IPI:        base.IPI,
			Porcentaje: base.Porcentaje,
			Importe:    importes[clave],
		}
	}
	return nuevas, residuo, nil
}
