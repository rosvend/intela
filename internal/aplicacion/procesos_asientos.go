package aplicacion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Hechos del modulo de proceso (RD 13.5); el prefijo agrupa en la vista de auditoria.
const (
	HechoProcesoAbierto            = "proceso.abierto"
	HechoProcesoEtapaAvanzada      = "proceso.etapa_avanzada"
	HechoFirmaRegistrada           = "firma.registrada"
	HechoProcesoCompuertaRechazada = "proceso.gate_rechazado"
)

// RefProceso es el ref_tipo de los asientos de una corrida.
const RefProceso = "proceso"

// AsientoProceso es el payload de toda transicion de un ProcesoDeReparto.
type AsientoProceso struct {
	Periodo       string         `json:"periodo"`
	Circuito      string         `json:"circuito"`
	BolsaID       string         `json:"bolsa_id"`
	SnapshotID    string         `json:"snapshot_id"`
	Reglamento    string         `json:"reglamento"`
	Etapa         string         `json:"etapa"`
	EtapaAnterior string         `json:"etapa_anterior,omitempty"`
	Revision      int            `json:"revision"`
	Motivo        string         `json:"motivo,omitempty"`
	Firma         *FirmaAsentada `json:"firma,omitempty"`
	// CriticasAceptadasTalCual: cuantas criticas del periodo estaban cerradas sin corregir el dato
	// cuando la compuerta de anomalias dejo pasar ESTA transicion (#164). Ausente si la transicion
	// no pasa por la compuerta; un cero explicito si paso y no habia ninguna.
	CriticasAceptadasTalCual *int `json:"criticas_aceptadas_tal_cual,omitempty"`
}

// FirmaAsentada es una firma de compuerta: quien, en que rol y sobre que revision (ADR 0008).
type FirmaAsentada struct {
	Rol           string `json:"rol"`
	ActorID       string `json:"actor_id"`
	SobreRevision int    `json:"sobre_revision"`
}

func asientoProceso(p reparto.ProcesoDeReparto, anterior reparto.Etapa) AsientoProceso {
	a := AsientoProceso{
		Periodo: p.Periodo, Circuito: string(p.Circuito), BolsaID: p.BolsaID,
		SnapshotID: p.SnapshotID, Reglamento: p.Reglamento,
		Etapa: string(p.Etapa), Revision: p.Revision, Motivo: p.RechazoMotivo,
	}
	if anterior != "" && anterior != p.Etapa {
		a.EtapaAnterior = string(anterior)
	}
	return a
}

// pendiente es un asiento a escribir dentro de la unidad de su transicion.
type pendiente struct {
	hecho, refTipo, refID string
	payload               any
}

// transicion escribe los datos y sus asientos en una sola unidad: si un asiento falla, nada queda (ADR 0006).
func (uc Procesos) transicion(ctx context.Context, actorID string, escribir func(context.Context) ([]pendiente, error)) error {
	if uc.Unidad == nil || uc.Bitacora == nil || uc.Reloj == nil {
		return fmt.Errorf("procesos mal cableado: faltan Unidad, Bitacora o Reloj")
	}
	return uc.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		asientos, err := escribir(ctx)
		if err != nil {
			return err
		}
		cuando := uc.Reloj.Ahora()
		for _, a := range asientos {
			payload, err := json.Marshal(a.payload)
			if err != nil {
				return fmt.Errorf("serializar asiento %q: %w", a.hecho, err)
			}
			if err := uc.Bitacora.Asentar(ctx, Asiento{
				Hecho: a.hecho, RefTipo: a.refTipo, RefID: a.refID,
				ActorID: actorID, Payload: payload, Cuando: cuando,
			}); err != nil {
				return fmt.Errorf("asentar %q: %w", a.hecho, err)
			}
		}
		return nil
	})
}

func pendienteDeProceso(hecho string, p reparto.ProcesoDeReparto, payload AsientoProceso) pendiente {
	return pendiente{hecho: hecho, refTipo: RefProceso, refID: p.ID, payload: payload}
}
