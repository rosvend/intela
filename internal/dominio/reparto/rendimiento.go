package reparto

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrRendimientoCircuitoMezclado: RD 10.3 exige inversiones nacionales e
// internacionales segregadas -- "esto permitira identificar con facilidad
// a que tipo de reparto corresponden los rendimientos obtenidos". Un pool
// solo acepta montos de su propio circuito.
var ErrRendimientoCircuitoMezclado = errors.New("el rendimiento no coincide con el circuito del pool")

// PoolRendimiento son los rendimientos financieros de un (circuito, vigencia)
// (RD 10). Vigencia es el ano de la comunicacion publica que se reparte, no
// el ano en que llego el pago (RD 10.1): un string, como periodo en
// procesos, no un time.Time -- el dominio no importa "time".
type PoolRendimiento struct {
	Circuito Circuito
	Vigencia string
	Monto    decimal.Decimal
}

// NuevoPoolRendimiento abre el ledger de un circuito y una vigencia.
func NuevoPoolRendimiento(circuito Circuito, vigencia string, monto decimal.Decimal) (PoolRendimiento, error) {
	if strings.TrimSpace(vigencia) == "" {
		return PoolRendimiento{}, fmt.Errorf("%w: vigencia vacia", ErrRepartoInvalido)
	}
	if err := exigirNoNegativo("rendimiento_monto", monto); err != nil {
		return PoolRendimiento{}, err
	}
	return PoolRendimiento{Circuito: circuito, Vigencia: vigencia, Monto: monto}, nil
}

// AcrecerRendimiento suma un monto nuevo al pool, siempre que venga del mismo
// circuito (RD 10.3). No hay forma de mezclar los dos ledgers por este
// camino: el chequeo se hace en cada operacion, no solo en la documentacion.
func AcrecerRendimiento(pool PoolRendimiento, circuitoOrigen Circuito, monto decimal.Decimal) (PoolRendimiento, error) {
	if circuitoOrigen != pool.Circuito {
		return PoolRendimiento{}, fmt.Errorf("%w: pool es %q, monto viene de %q",
			ErrRendimientoCircuitoMezclado, pool.Circuito, circuitoOrigen)
	}
	if err := exigirNoNegativo("rendimiento_monto", monto); err != nil {
		return PoolRendimiento{}, err
	}
	pool.Monto = pool.Monto.Add(monto)
	return pool, nil
}
