package herramientas

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
)

// MaxCorridasPorConsulta acota lo que vuelve al modelo cuando se lista un periodo.
const MaxCorridasPorConsulta = 20

// ConsultaCorridas es la lectura de estado de corridas; aplicacion.ConsultarEstadoCorrida la cumple.
type ConsultaCorridas interface {
	PorID(ctx context.Context, actor aplicacion.Usuario, procesoID string) (aplicacion.EstadoCorrida, error)
	DePeriodo(ctx context.Context, actor aplicacion.Usuario, periodo string) ([]aplicacion.EstadoCorrida, error)
}

type corridaJSON struct {
	ProcesoID         string   `json:"proceso_id"`
	Periodo           string   `json:"periodo"`
	BolsaID           string   `json:"bolsa_id"`
	Reglamento        string   `json:"reglamento,omitempty"`
	Circuito          string   `json:"circuito"`
	CircuitoExplicado string   `json:"circuito_explicado"`
	Etapa             string   `json:"etapa"`
	EtapaNombre       string   `json:"etapa_nombre"`
	EtapaExplicada    string   `json:"etapa_explicada"`
	Paso              int      `json:"paso"`
	TotalPasos        int      `json:"total_pasos"`
	SiguienteEtapa    string   `json:"siguiente_etapa,omitempty"`
	Compuerta         bool     `json:"compuerta_doble_firma"`
	Revision          int      `json:"revision"`
	FirmasFaltantes   []string `json:"firmas_faltantes"`
	Pendiente         string   `json:"pendiente"`
	UltimoRechazo     string   `json:"ultimo_rechazo,omitempty"`
}

type corridasJSON struct {
	Total    int           `json:"total"`
	Corridas []corridaJSON `json:"corridas"`
}

// EstadoCorrida dice en que etapa de RD 13.5 esta una corrida de reparto y que le falta para avanzar.
func EstadoCorrida(uc ConsultaCorridas) aplicacion.Herramienta {
	return aplicacion.Herramienta{
		Nombre: "estado_corrida",
		Descripcion: "Consulta en qué etapa del proceso de reparto (RD 13.5) está una corrida y qué le falta para avanzar: " +
			"circuito nacional o internacional, etapa actual y siguiente, firmas pendientes de la compuerta y último rechazo. " +
			"Con proceso_id devuelve esa corrida; sin él, las corridas del periodo (AAAA o AAAA-MM; vacío = todas), la más reciente primero. " +
			"Solo lee: no puede firmar, avanzar ni rechazar una corrida.",
		Esquema: json.RawMessage(`{"type":"object","properties":{` +
			`"proceso_id":{"type":"string","description":"Identificador de la corrida, p. ej. proc-bolsa-1-1."},` +
			`"periodo":{"type":"string","description":"AAAA o AAAA-MM. Se ignora si viene proceso_id."}` +
			`},"additionalProperties":false}`),
		Ejecutar: func(ctx context.Context, actor aplicacion.Usuario, args json.RawMessage) (any, error) {
			var a struct {
				ProcesoID string `json:"proceso_id"`
				Periodo   string `json:"periodo"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, fmt.Errorf("%w: %v", aplicacion.ErrArgumentosInvalidos, err)
			}
			if a.ProcesoID != "" {
				e, err := uc.PorID(ctx, actor, a.ProcesoID)
				if err != nil {
					return nil, err
				}
				return aCorridaJSON(e), nil
			}
			es, err := uc.DePeriodo(ctx, actor, a.Periodo)
			if err != nil {
				return nil, err
			}
			out := corridasJSON{Total: len(es), Corridas: []corridaJSON{}}
			for _, e := range es[:min(len(es), MaxCorridasPorConsulta)] {
				out.Corridas = append(out.Corridas, aCorridaJSON(e))
			}
			return out, nil
		},
	}
}

func aCorridaJSON(e aplicacion.EstadoCorrida) corridaJSON {
	return corridaJSON{
		ProcesoID: e.ProcesoID, Periodo: e.Periodo, BolsaID: e.BolsaID, Reglamento: e.Reglamento,
		Circuito: e.Circuito, CircuitoExplicado: e.CircuitoExplicado,
		Etapa: e.Etapa, EtapaNombre: e.EtapaNombre, EtapaExplicada: e.EtapaExplicada,
		Paso: e.Paso, TotalPasos: e.TotalPasos, SiguienteEtapa: e.SiguienteEtapa,
		Compuerta: e.Compuerta, Revision: e.Revision, FirmasFaltantes: e.FirmasFaltantes,
		Pendiente: e.Pendiente, UltimoRechazo: e.UltimoRechazo,
	}
}
