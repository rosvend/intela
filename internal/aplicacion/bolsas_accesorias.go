package aplicacion

import (
	"context"
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
//
// Es de una sola vez: CrearReserva falla si el proceso ya tiene una (N2). Un
// segundo alta no puede pisar tasa ni monto en silencio.
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

// LiberarReservaPrescrita reparte el remanente de una reserva prescrita
// (RD 14.4) sobre las proporciones EXACTAS de la corrida de la que salio,
// sin revalorizar nada -- lee [reparto.LineaTitular] ya persistidas, no
// vuelve a correr el motor. rendimientoAcumulado es el rendimiento que esa
// reserva invertida acumulo mientras estuvo retenida (RD 10.4); cero si no
// aplica.
//
// El consumo del saldo pasa por ActualizarSaldoReserva: el adaptador
// bloquea la fila, entrega el saldo actual, y persiste lo que este metodo
// devuelva -- todo en una transaccion. Sin eso, dos liberaciones
// concurrentes leerian el mismo saldo y repartirian las dos.
//
// El saldo nuevo es el residuo, no cero: si no hay lineas de titular sobre
// las que repartir (una corrida con toda la declaracion incompleta), el
// importe se queda en la reserva en vez de evaporarse.
//
// Que la corrida sea prescrita o no lo decide quien llama (#34/prescripcion,
// fuera de este alcance): este caso de uso solo ejecuta la liberacion.
func (b BolsasAccesorias) LiberarReservaPrescrita(ctx context.Context, procesoID string, rendimientoAcumulado decimal.Decimal) ([]reparto.LineaTitular, decimal.Decimal, error) {
	if rendimientoAcumulado.IsNegative() {
		return nil, decimal.Zero, fmt.Errorf("%w: rendimiento acumulado negativo", reparto.ErrRepartoInvalido)
	}
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoID, err)
	}

	var nuevas []reparto.LineaTitular
	var residuo decimal.Decimal
	err = b.Reservas.ActualizarSaldoReserva(ctx, procesoID, func(saldoActual decimal.Decimal) (decimal.Decimal, error) {
		monto := saldoActual.Add(rendimientoAcumulado)
		var errDist error
		nuevas, residuo, errDist = reparto.DistribuirSobreProporciones(monto, resultado.Titulares)
		if errDist != nil {
			return decimal.Decimal{}, errDist
		}
		return residuo, nil
	})
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoID, err)
	}
	return nuevas, residuo, nil
}

// RegistrarRendimiento acrece el ledger de (circuito, vigencia) en una sola
// sentencia atomica: dos acrecimientos concurrentes se suman, ninguno pisa
// al otro (B2). Crea la fila si es la primera vez.
func (b BolsasAccesorias) RegistrarRendimiento(ctx context.Context, circuito reparto.Circuito, vigencia string, monto decimal.Decimal) error {
	if _, err := reparto.NuevoPoolRendimiento(circuito, vigencia, monto); err != nil {
		return err
	}
	if err := b.Rendimientos.AcrecerRendimiento(ctx, circuito, vigencia, monto); err != nil {
		return fmt.Errorf("registrar rendimiento %s/%s: %w", circuito, vigencia, err)
	}
	return nil
}

// DistribuirRendimiento reparte el pool de rendimiento sobre las
// proporciones de una corrida, sin revalorizar nada (RD 10.1). Consume el
// monto: el nuevo valor del pool es el residuo, asi que una segunda llamada
// no vuelve a repartir lo mismo (B3), igual que LiberarReservaPrescrita.
func (b BolsasAccesorias) DistribuirRendimiento(ctx context.Context, procesoID string, circuito reparto.Circuito, vigencia string) ([]reparto.LineaTitular, decimal.Decimal, error) {
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("distribuir rendimiento sobre %q: %w", procesoID, err)
	}

	var nuevas []reparto.LineaTitular
	var residuo decimal.Decimal
	err = b.Rendimientos.ActualizarMontoRendimiento(ctx, circuito, vigencia, func(montoActual decimal.Decimal) (decimal.Decimal, error) {
		var errDist error
		nuevas, residuo, errDist = reparto.DistribuirSobreProporciones(montoActual, resultado.Titulares)
		if errDist != nil {
			return decimal.Decimal{}, errDist
		}
		return residuo, nil
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

// FirmarReclamacion agrega uno de los dos avales de RD 14.5.10-12. El
// dominio rechaza un actor cubriendo los dos roles o un rol firmando dos
// veces; este caso de uso no repite esa validacion, la propaga.
//
// El estado persistido ('resuelta' con los dos avales) lo recalcula el
// adaptador a partir de lo que de verdad quedo en la base, no de lo que este
// metodo cree que hay: dos avales firmados a la vez no pueden dejar el
// estado desincronizado (B4).
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
