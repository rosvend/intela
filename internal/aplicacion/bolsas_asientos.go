package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Hechos de las bolsas accesorias. El linaje de cada uno nombra dos corridas:
// la que fijo las proporciones y la que paga (#177, RD 14.4, RD 10.1).
const (
	HechoReservaLiberada          = "reserva.liberada"
	HechoRendimientosDistribuidos = "rendimientos.distribuidos"
)

// Prefijos de ref de una cifra que no sale de la valorizacion de una sola
// corrida. La forma es prefijo:origen:destino:obra:titular.
const (
	prefijoRefReserva     = "reserva"
	prefijoRefRendimiento = "rendimiento"
)

// CorridaAsentada es una corrida tal como queda en el asiento: lo que
// ExplicarCifra necesita para devolverla sin leer procesos.
type CorridaAsentada struct {
	ProcesoID string `json:"proceso_id"`
	Periodo   string `json:"periodo"`
	Circuito  string `json:"circuito"`
}

// LineaDosCorridas es la parte de un titular en el reparto accesorio.
type LineaDosCorridas struct {
	ObraID     string `json:"obra_id"`
	TitularID  string `json:"titular_id"`
	IPI        string `json:"ipi"`
	Porcentaje string `json:"porcentaje"`
	Importe    string `json:"importe"`
}

// asientoReservaLiberada es el payload de reserva.liberada.
type asientoReservaLiberada struct {
	Origen              CorridaAsentada    `json:"origen"`
	Destino             CorridaAsentada    `json:"destino"`
	VigenciaRendimiento string             `json:"vigencia_rendimiento,omitempty"`
	Lineas              []LineaDosCorridas `json:"lineas"`
}

// asientoRendimientosDistribuidos es el payload de rendimientos.distribuidos.
type asientoRendimientosDistribuidos struct {
	Origen   CorridaAsentada    `json:"origen"`
	Destino  CorridaAsentada    `json:"destino"`
	Circuito string             `json:"circuito"`
	Vigencia string             `json:"vigencia"`
	Lineas   []LineaDosCorridas `json:"lineas"`
}

// refAccesoria es una cifra liberada o un rendimiento ya repartido.
type refAccesoria struct {
	hecho, origen, destino, obra, titular string
}

// FormarRefReservaLiberada arma la ref de una linea pagada al liberar una reserva.
func FormarRefReservaLiberada(origen, destino, obra, titular string) string {
	return formarRefAccesoria(prefijoRefReserva, origen, destino, obra, titular)
}

// FormarRefRendimiento arma la ref de una linea de rendimiento distribuido.
func FormarRefRendimiento(origen, destino, obra, titular string) string {
	return formarRefAccesoria(prefijoRefRendimiento, origen, destino, obra, titular)
}

func formarRefAccesoria(prefijo, origen, destino, obra, titular string) string {
	return strings.Join([]string{prefijo, origen, destino, obra, titular}, ":")
}

func parsearRefAccesoria(ref string) (refAccesoria, bool) {
	partes := strings.Split(ref, ":")
	if len(partes) != 5 {
		return refAccesoria{}, false
	}
	var hecho string
	switch partes[0] {
	case prefijoRefReserva:
		hecho = HechoReservaLiberada
	case prefijoRefRendimiento:
		hecho = HechoRendimientosDistribuidos
	default:
		return refAccesoria{}, false
	}
	for _, p := range partes[1:] {
		if strings.TrimSpace(p) == "" {
			return refAccesoria{}, false
		}
	}
	return refAccesoria{
		hecho: hecho, origen: partes[1], destino: partes[2], obra: partes[3], titular: partes[4],
	}, true
}

func exigirDosCorridas(origen, destino string) error {
	if strings.TrimSpace(origen) == "" || strings.TrimSpace(destino) == "" {
		return fmt.Errorf("%w: faltan la corrida de origen o la de destino", reparto.ErrRepartoInvalido)
	}
	if origen == destino {
		return fmt.Errorf("%w: la corrida de origen y la de destino son la misma", reparto.ErrRepartoInvalido)
	}
	return nil
}

func (b BolsasAccesorias) enUnidad(ctx context.Context, fn func(context.Context) error) error {
	switch {
	case b.Unidad == nil:
		return errors.New("bolsas accesorias mal cableadas: falta UnidadDeTrabajo")
	case b.Bitacora == nil:
		return errors.New("bolsas accesorias mal cableadas: falta BitacoraAuditoria")
	case b.Reloj == nil:
		return errors.New("bolsas accesorias mal cableadas: falta Reloj")
	case b.Corridas == nil:
		return errors.New("bolsas accesorias mal cableadas: falta LecturaDeCorridas")
	}
	return b.Unidad.EnUnidad(ctx, fn)
}

func (b BolsasAccesorias) dosCorridas(ctx context.Context, origenID, destinoID string) (CorridaAsentada, CorridaAsentada, error) {
	origen, err := b.Corridas.ProcesoPorID(ctx, origenID)
	if err != nil {
		return CorridaAsentada{}, CorridaAsentada{}, fmt.Errorf("corrida de origen %q: %w", origenID, err)
	}
	destino, err := b.Corridas.ProcesoPorID(ctx, destinoID)
	if err != nil {
		return CorridaAsentada{}, CorridaAsentada{}, fmt.Errorf("corrida de destino %q: %w", destinoID, err)
	}
	return corridaAsentadaDe(origen), corridaAsentadaDe(destino), nil
}

func corridaAsentadaDe(p ProcesoVista) CorridaAsentada {
	return CorridaAsentada{ProcesoID: p.ID, Periodo: p.Periodo, Circuito: string(p.Circuito)}
}

func lineasAsentadas(lineas []reparto.LineaTitular) []LineaDosCorridas {
	out := make([]LineaDosCorridas, 0, len(lineas))
	for _, l := range lineas {
		out = append(out, LineaDosCorridas{
			ObraID: l.ObraID, TitularID: l.TitularID, IPI: l.IPI,
			Porcentaje: l.Porcentaje.String(), Importe: l.Importe.StringFixed(2),
		})
	}
	return out
}

func (b BolsasAccesorias) asentarDosCorridas(ctx context.Context, hecho, refID, actorID string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializar asiento %q: %w", hecho, err)
	}
	if err := b.Bitacora.Asentar(ctx, Asiento{
		Hecho: hecho, RefTipo: RefProceso, RefID: refID,
		ActorID: actorID, Payload: raw, Cuando: b.Reloj.Ahora(),
	}); err != nil {
		return fmt.Errorf("asentar %q: %w", hecho, err)
	}
	return nil
}

// lineaDeAsiento busca en el payload la linea de esta obra y titular.
// Un importe en cero no tapa uno positivo anterior: redistribuir un pool
// ya vacio deja lineas en cero y no borra el pago que ya se asento.
func lineaDeAsiento(payload []byte, obra, titular string) (LineaDosCorridas, CorridaAsentada, CorridaAsentada, bool) {
	var cuerpo struct {
		Origen  CorridaAsentada    `json:"origen"`
		Destino CorridaAsentada    `json:"destino"`
		Lineas  []LineaDosCorridas `json:"lineas"`
	}
	if err := json.Unmarshal(payload, &cuerpo); err != nil {
		return LineaDosCorridas{}, CorridaAsentada{}, CorridaAsentada{}, false
	}
	for _, l := range cuerpo.Lineas {
		if l.ObraID == obra && l.TitularID == titular {
			return l, cuerpo.Origen, cuerpo.Destino, true
		}
	}
	return LineaDosCorridas{}, CorridaAsentada{}, CorridaAsentada{}, false
}

func importePositivo(s string) bool {
	d, err := decimal.NewFromString(s)
	return err == nil && d.IsPositive()
}
