package aplicacion

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// BolsasAccesorias orquesta la reserva de errores tecnicos (RD 14) y los rendimientos financieros (RD 10). Sin ruta HTTP todavia: eso lo cablea #34.
type BolsasAccesorias struct {
	Resultados    RepositorioResultados
	Reservas      RepositorioReservas
	Rendimientos  RepositorioRendimientos
	Reclamaciones RepositorioReclamacionesReserva
}

// RegistrarReserva abre el pool de reserva de una corrida con Resultado.Reserva y tasaPct (RD 14.5.1). De una sola vez: CrearReserva falla si el proceso ya tiene una.
func (b BolsasAccesorias) RegistrarReserva(ctx context.Context, procesoID string, circuito reparto.Circuito, tasaPct decimal.Decimal) (reparto.PoolReserva, error) {
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoID)
	if err != nil {
		return reparto.PoolReserva{}, fmt.Errorf("registrar reserva de %q: %w", procesoID, err)
	}
	pool, err := reparto.NuevaPoolReserva(procesoID, circuito, resultado.Reserva, tasaPct)
	if err != nil {
		return reparto.PoolReserva{}, err
	}
	if err := b.Reservas.CrearReserva(ctx, pool); err != nil {
		return reparto.PoolReserva{}, fmt.Errorf("crear reserva de %q: %w", procesoID, err)
	}
	return pool, nil
}

// LiberarReservaPrescrita reparte el remanente de una reserva (RD 14.4) sobre las proporciones exactas de su corrida, mas rendimientoAUsar (RD 10.4) descontado de (nacional, vigenciaRendimiento). El saldo nuevo es el residuo, no cero: sin titulares a quien repartir, el importe se queda en la reserva.
func (b BolsasAccesorias) LiberarReservaPrescrita(ctx context.Context, procesoID, vigenciaRendimiento string, rendimientoAUsar decimal.Decimal) ([]reparto.LineaTitular, decimal.Decimal, error) {
	if rendimientoAUsar.IsNegative() {
		return nil, decimal.Zero, fmt.Errorf("%w: rendimiento acumulado negativo", reparto.ErrRepartoInvalido)
	}
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoID, err)
	}

	var nuevas []reparto.LineaTitular
	var residuo decimal.Decimal
	err = b.Reservas.LiberarSaldoReserva(ctx, procesoID, vigenciaRendimiento, rendimientoAUsar,
		func(saldoActual decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
			monto := saldoActual.Add(rendimientoAUsar)
			var errDist error
			nuevas, residuo, errDist = reparto.DistribuirSobreProporciones(monto, resultado.Titulares)
			if errDist != nil {
				return decimal.Decimal{}, nil, errDist
			}
			return residuo, nuevas, nil
		})
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoID, err)
	}
	return nuevas, residuo, nil
}

// RegistrarRendimiento acrece el ledger de (circuito, vigencia) atomicamente, creando la fila si es la primera vez.
func (b BolsasAccesorias) RegistrarRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia string, monto decimal.Decimal) error {
	if _, err := reparto.NuevoPoolRendimiento(circuito, vigencia, monto); err != nil {
		return err
	}
	if err := b.Rendimientos.AcrecerRendimiento(ctx, circuito, vigencia, monto); err != nil {
		return fmt.Errorf("registrar rendimiento %s/%s: %w", circuito, vigencia, err)
	}
	return nil
}

// DistribuirRendimiento reparte el pool sobre las proporciones de una corrida (RD 10.1), consumiendo el monto y persistiendo las lineas en la misma transaccion.
func (b BolsasAccesorias) DistribuirRendimiento(ctx context.Context, procesoID string, circuito reparto.Circuito, vigencia string) ([]reparto.LineaTitular, decimal.Decimal, error) {
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("distribuir rendimiento sobre %q: %w", procesoID, err)
	}

	var nuevas []reparto.LineaTitular
	var residuo decimal.Decimal
	err = b.Rendimientos.ActualizarMontoRendimiento(ctx, circuito, vigencia, procesoID,
		func(montoActual decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
			var errDist error
			nuevas, residuo, errDist = reparto.DistribuirSobreProporciones(montoActual, resultado.Titulares)
			if errDist != nil {
				return decimal.Decimal{}, nil, errDist
			}
			return residuo, nuevas, nil
		})
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("distribuir rendimiento %s/%s: %w", circuito, vigencia, err)
	}
	return nuevas, residuo, nil
}

// AbrirReclamacion valida elegibilidad (RD 14.5.5, RD 14.5.6) y persiste el
// reclamo. afiliadoAntesDelPeriodo y esErrorDeDeclaracion los resuelve quien
// llama, contra el padron de afiliacion y el motivo declarado.
func (b BolsasAccesorias) AbrirReclamacion(
	ctx context.Context,
	id, titularID, procesoOrigenID, detalle string,
	montoSolicitado decimal.Decimal,
	afiliadoAntesDelPeriodo, esErrorDeDeclaracion bool,
) (reparto.ReclamacionReserva, error) {
	r, err := reparto.NuevaReclamacionReserva(id, titularID, procesoOrigenID, detalle, montoSolicitado,
		afiliadoAntesDelPeriodo, esErrorDeDeclaracion)
	if err != nil {
		return reparto.ReclamacionReserva{}, err
	}
	if err := b.Reclamaciones.GuardarReclamacion(ctx, r); err != nil {
		return reparto.ReclamacionReserva{}, fmt.Errorf("abrir reclamacion %q: %w", id, err)
	}
	return r, nil
}

// FirmarReclamacion agrega un aval de RD 14.5.10-12; propaga el rechazo del dominio si el actor o el rol ya estan cubiertos.
func (b BolsasAccesorias) FirmarReclamacion(ctx context.Context, id string, rol reparto.RolAvalReclamacion, actorID string) (reparto.ReclamacionReserva, error) {
	r, err := b.Reclamaciones.ReclamacionPorID(ctx, id)
	if err != nil {
		return reparto.ReclamacionReserva{}, fmt.Errorf("firmar reclamacion %q: %w", id, err)
	}
	r, err = r.Avalar(rol, actorID)
	if err != nil {
		return reparto.ReclamacionReserva{}, err
	}
	if err := b.Reclamaciones.GuardarReclamacion(ctx, r); err != nil {
		return reparto.ReclamacionReserva{}, fmt.Errorf("guardar aval de %q: %w", id, err)
	}
	return r, nil
}
