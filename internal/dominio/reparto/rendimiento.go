package reparto

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// vigenciaValida es el ano de la comunicacion publica (RD 10.1): cuatro
// digitos, nada mas.
var vigenciaValida = regexp.MustCompile(`^[0-9]{4}$`)

// ErrCircuitoCruzado: RD 10.3 separa las inversiones del recaudo nacional y
// las del internacional para saber a que reparto corresponde cada
// rendimiento. Un rendimiento se reparte sobre una corrida de su circuito y
// se paga en otra del mismo.
var ErrCircuitoCruzado = errors.New("el rendimiento no se reparte fuera de su circuito")

// ExigirMismoCircuito rechaza repartir o pagar un rendimiento en una corrida de otro circuito (RD 10.3).
func ExigirMismoCircuito(rendimiento, origen, destino Circuito) error {
	if origen != rendimiento || destino != rendimiento {
		return fmt.Errorf("%w: rendimiento %q, origen %q, destino %q", ErrCircuitoCruzado, rendimiento, origen, destino)
	}
	return nil
}

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
