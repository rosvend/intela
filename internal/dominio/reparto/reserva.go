package reparto

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// techoReservaPct es RD 14.1: "hasta un 5%". Es un techo, no la tasa: la tasa
// vigente la aprueba la Asamblea General (RD 14.5.1) y llega por el snapshot.
var techoReservaPct = decimal.NewFromInt(5)

// organoAprobadorReserva es fijo por RD 14.5.1. No es un parametro del
// constructor: asi no hay forma de crear una reserva con otro organo.
const organoAprobadorReserva = "Asamblea General"

// ErrReservaInternacional: RD 14.5.4, la reserva de errores tecnicos no
// aplica al recaudo recibido del extranjero.
var ErrReservaInternacional = errors.New("la reserva de errores tecnicos no aplica al circuito internacional")

// PoolReserva es la reserva de errores tecnicos retenida de una corrida
// (RD 14). Su proveniencia -- de que corrida se tomo -- es dato permanente:
// RD 14.4 exige liberar el remanente proporcional a como se distribuyo
// ESA corrida, incluso anos despues (ver [DistribuirSobreProporciones]).
type PoolReserva struct {
	ProcesoID       string
	Circuito        Circuito
	MontoInicial    decimal.Decimal
	Saldo           decimal.Decimal
	TasaPct         decimal.Decimal
	OrganoAprobador string
}

// NuevaPoolReserva construye la reserva retenida de una corrida. Circuito
// internacional y tasa por fuera del techo del 14.1 son errores tipados, no
// un valor recortado en silencio.
func NuevaPoolReserva(procesoID string, circuito Circuito, montoInicial, tasaPct decimal.Decimal) (PoolReserva, error) {
	if circuito == Internacional {
		return PoolReserva{}, fmt.Errorf("%w: proceso %q", ErrReservaInternacional, procesoID)
	}
	if err := exigirNoNegativo("monto_inicial", montoInicial); err != nil {
		return PoolReserva{}, err
	}
	if err := exigirPositivo("reserva_tasa_pct", tasaPct); err != nil {
		return PoolReserva{}, err
	}
	if tasaPct.GreaterThan(techoReservaPct) {
		return PoolReserva{}, fmt.Errorf("%w: tasa %s%% supera el techo de %s%% (RD 14.1)",
			ErrRepartoInvalido, tasaPct, techoReservaPct)
	}
	return PoolReserva{
		ProcesoID:       procesoID,
		Circuito:        circuito,
		MontoInicial:    montoInicial,
		Saldo:           montoInicial,
		TasaPct:         tasaPct,
		OrganoAprobador: organoAprobadorReserva,
	}, nil
}
