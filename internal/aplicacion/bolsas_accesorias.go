package aplicacion

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// BolsasAccesorias orquesta las dos bolsas que salen de la distribucion
// principal y regresan despues: la reserva de errores tecnicos (RD 14) y los
// rendimientos financieros (RD 10). Los dos comparten la misma operacion de
// dominio -- [reparto.DistribuirSobreProporciones] -- que reparte un monto
// nuevo sobre las lineas de titular de una corrida ya cerrada, sin
// revalorizar nada.
//
// No conoce RepositorioProcesos ni la maquina de estados de RD 13.5 (#34):
// recibe circuito y proceso de origen como los da quien ya orquesto la
// corrida, y solo lee/escribe lo que esta issue necesita.
type BolsasAccesorias struct {
	Resultados    RepositorioResultados
	Reservas      RepositorioReservas
	Rendimientos  RepositorioRendimientos
	Reclamaciones RepositorioReclamacionesReserva
}

// RegistrarReserva abre el pool de reserva de una corrida ya cerrada, con el
// monto que [reparto.deducciones] ya calculo en Resultado.Reserva (R-07).
// tasaPct es la tasa vigente al momento de esa corrida (RD 14.5.1); quien
// orquesta la corrida ya la tiene del mismo snapshot que uso para calcularla.
func (b BolsasAccesorias) RegistrarReserva(ctx context.Context, procesoID string, circuito reparto.Circuito, tasaPct decimal.Decimal) (reparto.PoolReserva, error) {
	resultado, err := b.Resultados.PorProceso(ctx, procesoID)
	if err != nil {
		return reparto.PoolReserva{}, fmt.Errorf("registrar reserva de %q: %w", procesoID, err)
	}
	pool, err := reparto.NuevaPoolReserva(procesoID, circuito, resultado.Reserva, tasaPct)
	if err != nil {
		return reparto.PoolReserva{}, err
	}
	if err := b.Reservas.Guardar(ctx, pool); err != nil {
		return reparto.PoolReserva{}, fmt.Errorf("guardar reserva de %q: %w", procesoID, err)
	}
	return pool, nil
}

// LiberarReservaPrescrita reparte el remanente de una reserva prescrita
// (RD 14.4) sobre las proporciones EXACTAS de la corrida de la que salio,
// sin revalorizar nada -- lee [reparto.LineaTitular] ya persistidas, no
// vuelve a correr el motor. rendimientoAcumulado es el rendimiento que esa
// reserva invertida acumulo mientras estuvo retenida (RD 10.4); cero si no
// aplica.
//
// Que la corrida sea prescrita o no lo decide quien llama (#34/prescripcion,
// fuera de este alcance): este caso de uso solo ejecuta la liberacion.
func (b BolsasAccesorias) LiberarReservaPrescrita(ctx context.Context, procesoID string, rendimientoAcumulado decimal.Decimal) ([]reparto.LineaTitular, decimal.Decimal, error) {
	pool, err := b.Reservas.PorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoID, err)
	}
	resultado, err := b.Resultados.PorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoID, err)
	}

	monto := pool.Saldo.Add(rendimientoAcumulado)
	nuevas, residuo, err := reparto.DistribuirSobreProporciones(monto, resultado.Titulares)
	if err != nil {
		return nil, decimal.Zero, err
	}

	pool.Saldo = decimal.Zero
	if err := b.Reservas.Guardar(ctx, pool); err != nil {
		return nil, decimal.Zero, fmt.Errorf("guardar reserva liberada de %q: %w", procesoID, err)
	}
	return nuevas, residuo, nil
}

// RegistrarRendimiento abre el ledger de (circuito, vigencia) si es la
// primera vez, o lo acrece si ya existia. RD 10.3 impide que un monto del
// circuito equivocado entre por aqui (lo aplica [reparto.AcrecerRendimiento]).
func (b BolsasAccesorias) RegistrarRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia string, monto decimal.Decimal) (reparto.PoolRendimiento, error) {
	pool, err := b.Rendimientos.PorCircuitoYVigencia(ctx, circuito, vigencia)
	switch {
	case errors.Is(err, ErrNoEncontrado):
		pool, err = reparto.NuevoPoolRendimiento(circuito, vigencia, monto)
		if err != nil {
			return reparto.PoolRendimiento{}, err
		}
	case err != nil:
		return reparto.PoolRendimiento{}, fmt.Errorf("registrar rendimiento %s/%s: %w", circuito, vigencia, err)
	default:
		pool, err = reparto.AcrecerRendimiento(pool, circuito, monto)
		if err != nil {
			return reparto.PoolRendimiento{}, err
		}
	}
	if err := b.Rendimientos.Guardar(ctx, pool); err != nil {
		return reparto.PoolRendimiento{}, fmt.Errorf("guardar rendimiento %s/%s: %w", circuito, vigencia, err)
	}
	return pool, nil
}

// DistribuirRendimiento reparte el pool de rendimiento sobre las
// proporciones de una corrida, sin revalorizar nada (RD 10.1).
func (b BolsasAccesorias) DistribuirRendimiento(ctx context.Context, procesoID string, circuito reparto.Circuito, vigencia string) ([]reparto.LineaTitular, decimal.Decimal, error) {
	pool, err := b.Rendimientos.PorCircuitoYVigencia(ctx, circuito, vigencia)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("distribuir rendimiento %s/%s: %w", circuito, vigencia, err)
	}
	resultado, err := b.Resultados.PorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("distribuir rendimiento sobre %q: %w", procesoID, err)
	}
	return reparto.DistribuirSobreProporciones(pool.Monto, resultado.Titulares)
}

// AbrirReclamacion valida elegibilidad (RD 14.5.5, RD 14.5.6) y persiste el
// reclamo. afiliadoAntesDelPeriodo y esErrorDeDeclaracion los resuelve quien
// llama, contra el padron de afiliacion y el motivo declarado.
func (b BolsasAccesorias) AbrirReclamacion(
	ctx context.Context,
	id, titularID, procesoOrigenID string,
	montoSolicitado decimal.Decimal,
	afiliadoAntesDelPeriodo, esErrorDeDeclaracion bool,
) (reparto.ReclamacionReserva, error) {
	r, err := reparto.NuevaReclamacionReserva(id, titularID, procesoOrigenID, montoSolicitado,
		afiliadoAntesDelPeriodo, esErrorDeDeclaracion)
	if err != nil {
		return reparto.ReclamacionReserva{}, err
	}
	if err := b.Reclamaciones.Guardar(ctx, r); err != nil {
		return reparto.ReclamacionReserva{}, fmt.Errorf("abrir reclamacion %q: %w", id, err)
	}
	return r, nil
}

// FirmarReclamacion agrega uno de los dos avales de RD 14.5.10-12. El
// dominio rechaza un actor cubriendo los dos roles o un rol firmando dos
// veces; este caso de uso no repite esa validacion, la propaga.
func (b BolsasAccesorias) FirmarReclamacion(ctx context.Context, id string, rol reparto.RolAvalReclamacion, actorID string) (reparto.ReclamacionReserva, error) {
	r, err := b.Reclamaciones.PorID(ctx, id)
	if err != nil {
		return reparto.ReclamacionReserva{}, fmt.Errorf("firmar reclamacion %q: %w", id, err)
	}
	r, err = r.Avalar(rol, actorID)
	if err != nil {
		return reparto.ReclamacionReserva{}, err
	}
	if err := b.Reclamaciones.Guardar(ctx, r); err != nil {
		return reparto.ReclamacionReserva{}, fmt.Errorf("guardar aval de %q: %w", id, err)
	}
	return r, nil
}
