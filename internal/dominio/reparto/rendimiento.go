package reparto

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// vigenciaValida es el ano de la comunicacion publica (RD 10.1): cuatro
// digitos, nada mas.
var vigenciaValida = regexp.MustCompile(`^[0-9]{4}$`)

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
	if !slices.Contains(recaudo.Circuitos(), circuito) {
		return PoolRendimiento{}, fmt.Errorf("%w: circuito %q desconocido", ErrRepartoInvalido, circuito)
	}
	if !vigenciaValida.MatchString(vigencia) {
		return PoolRendimiento{}, fmt.Errorf("%w: vigencia %q, se esperan cuatro digitos (RD 10.1)", ErrRepartoInvalido, vigencia)
	}
	if err := exigirNoNegativo("rendimiento_monto", monto); err != nil {
		return PoolRendimiento{}, err
	}
	return PoolRendimiento{Circuito: circuito, Vigencia: vigencia, Monto: monto}, nil
}
