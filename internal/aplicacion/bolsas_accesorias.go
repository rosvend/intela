package aplicacion

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// BolsasAccesorias orquesta la reserva de errores tecnicos (RD 14) y los rendimientos financieros (RD 10).
//
// Liberar y distribuir asientan en la misma unidad que el movimiento (#177).
// La ruta HTTP que los dispara sigue siendo de #34: cablearlos aqui no abre
// un endpoint.
type BolsasAccesorias struct {
	Resultados    RepositorioResultados
	Reservas      RepositorioReservas
	Rendimientos  RepositorioRendimientos
	Reclamaciones RepositorioReclamacionesReserva
	// Corridas aporta periodo y circuito al asiento. Sin eso ExplicarCifra
	// tendria que reconstruirlos, y el ADR 0006 lo prohibe.
	Corridas LecturaDeCorridas
	Bitacora BitacoraAuditoria
	Unidad   UnidadDeTrabajo
	Reloj    Reloj
}

// LecturaDeCorridas es lo unico que la liberacion lee de un proceso.
type LecturaDeCorridas interface {
	ProcesoPorID(ctx context.Context, id string) (ProcesoVista, error)
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

// LiberarReservaPrescrita reparte el remanente de una reserva (RD 14.4) sobre las proporciones exactas de su corrida de origen, mas rendimientoAUsar (RD 10.4) descontado de (nacional, vigenciaRendimiento). El saldo que devuelve es el persistido. El residuo de redondeo del rendimiento no sale del ledger: sin titulares, ese importe se queda en rendimientos y la reserva no lo absorbe.
//
// procesoDestinoID es la corrida en la que ese dinero se paga. El linaje
// apunta a las dos (#177). La liberacion y el asiento reserva.liberada son
// un solo hecho: si el asiento falla, no se libera nada.
func (b BolsasAccesorias) LiberarReservaPrescrita(
	ctx context.Context,
	procesoOrigenID, procesoDestinoID, vigenciaRendimiento string,
	rendimientoAUsar decimal.Decimal,
	actorID string,
) ([]reparto.LineaTitular, decimal.Decimal, error) {
	if rendimientoAUsar.IsNegative() {
		return nil, decimal.Zero, fmt.Errorf("%w: rendimiento acumulado negativo", reparto.ErrRepartoInvalido)
	}
	if err := exigirDosCorridas(procesoOrigenID, procesoDestinoID); err != nil {
		return nil, decimal.Zero, err
	}
	if err := exigirActor(actorID, fmt.Sprintf("liberar la reserva de %q", procesoOrigenID)); err != nil {
		return nil, decimal.Zero, err
	}
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoOrigenID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("liberar reserva de %q: %w", procesoOrigenID, err)
	}

	var nuevas []reparto.LineaTitular
	var residuo decimal.Decimal
	err = b.enUnidad(ctx, func(ctx context.Context) error {
		origen, destino, errCorr := b.dosCorridas(ctx, procesoOrigenID, procesoDestinoID)
		if errCorr != nil {
			return errCorr
		}
		errLib := b.Reservas.LiberarSaldoReserva(ctx, procesoOrigenID, vigenciaRendimiento, rendimientoAUsar,
			func(saldoActual decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
				monto := saldoActual.Add(rendimientoAUsar)
				var errDist error
				nuevas, residuo, errDist = reparto.DistribuirSobreProporciones(monto, resultado.Titulares)
				if errDist != nil {
					return decimal.Decimal{}, nil, errDist
				}
				return residuo, nuevas, nil
			})
		if errLib != nil {
			return fmt.Errorf("liberar reserva de %q: %w", procesoOrigenID, errLib)
		}
		return b.asentarDosCorridas(ctx, HechoReservaLiberada, procesoDestinoID, actorID, asientoReservaLiberada{
			Origen: origen, Destino: destino, VigenciaRendimiento: vigenciaRendimiento,
			Lineas: lineasAsentadas(nuevas),
		})
	})
	if err != nil {
		return nil, decimal.Zero, err
	}
	// El repositorio resta del saldo el residuo de la segunda particion, que
	// fn no ve. Releer deja el valor devuelto igual al persistido.
	if pool, errLectura := b.Reservas.ReservaPorProceso(ctx, procesoOrigenID); errLectura == nil {
		residuo = pool.Saldo
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

// DistribuirRendimiento reparte el pool sobre las proporciones de la corrida de origen (RD 10.1) y lo paga en la de destino. El asiento rendimientos.distribuidos y el debito del ledger son un solo hecho (#177).
func (b BolsasAccesorias) DistribuirRendimiento(
	ctx context.Context,
	procesoOrigenID, procesoDestinoID string,
	circuito reparto.Circuito,
	vigencia, actorID string,
) ([]reparto.LineaTitular, decimal.Decimal, error) {
	if err := exigirDosCorridas(procesoOrigenID, procesoDestinoID); err != nil {
		return nil, decimal.Zero, err
	}
	if err := exigirActor(actorID, fmt.Sprintf("distribuir el rendimiento %s/%s", circuito, vigencia)); err != nil {
		return nil, decimal.Zero, err
	}
	resultado, err := b.Resultados.ResultadoPorProceso(ctx, procesoOrigenID)
	if err != nil {
		return nil, decimal.Zero, fmt.Errorf("distribuir rendimiento sobre %q: %w", procesoOrigenID, err)
	}

	var nuevas []reparto.LineaTitular
	var residuo decimal.Decimal
	err = b.enUnidad(ctx, func(ctx context.Context) error {
		origen, destino, errCorr := b.dosCorridas(ctx, procesoOrigenID, procesoDestinoID)
		if errCorr != nil {
			return errCorr
		}
		errDist := b.Rendimientos.ActualizarMontoRendimiento(ctx, circuito, vigencia, procesoOrigenID,
			func(montoActual decimal.Decimal) (decimal.Decimal, []reparto.LineaTitular, error) {
				var errRep error
				nuevas, residuo, errRep = reparto.DistribuirSobreProporciones(montoActual, resultado.Titulares)
				if errRep != nil {
					return decimal.Decimal{}, nil, errRep
				}
				return residuo, nuevas, nil
			})
		if errDist != nil {
			return fmt.Errorf("distribuir rendimiento %s/%s: %w", circuito, vigencia, errDist)
		}
		return b.asentarDosCorridas(ctx, HechoRendimientosDistribuidos, procesoDestinoID, actorID, asientoRendimientosDistribuidos{
			Origen: origen, Destino: destino, Circuito: string(circuito), Vigencia: vigencia,
			Lineas: lineasAsentadas(nuevas),
		})
	})
	if err != nil {
		return nil, decimal.Zero, err
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
