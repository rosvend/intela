package herramientas

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ConsultaONI es la lectura de la cola ONI; aplicacion.ConsultarONI la cumple.
type ConsultaONI interface {
	Ejecutar(ctx context.Context, actor aplicacion.Usuario, f aplicacion.FiltroONI) (aplicacion.ColaONI, error)
}

type oniJSON struct {
	UsoID             string `json:"uso_id"`
	Titulo            string `json:"titulo"`
	TituloOriginal    string `json:"titulo_original,omitempty"`
	Fuente            string `json:"fuente"`
	Modalidad         string `json:"modalidad,omitempty"`
	Periodo           string `json:"periodo"`
	IDsFuente         string `json:"ids_fuente,omitempty"`
	ReporteID         string `json:"reporte_id"`
	DetectadaEn       string `json:"detectada_en"`
	CandidatosDudosos int    `json:"candidatos_dudosos"`
}

type colaONIJSON struct {
	PendientesTotal int       `json:"pendientes_total"`
	Obras           []oniJSON `json:"obras"`
}

// ListarONI lee la cola de obras no identificadas (RD 13.8) que espera identificacion.
func ListarONI(uc ConsultaONI) aplicacion.Herramienta {
	return aplicacion.Herramienta{
		Nombre: "listar_oni",
		Descripcion: "Lista la cola de obras no identificadas (ONI, RD 13.8): usos reportados que no se pudieron asociar a una obra del catálogo. " +
			"pendientes_total es cuántas hay en total con esos filtros; obras trae las más antiguas primero, hasta limite (por defecto 20, máximo 100). " +
			"Filtros opcionales: periodo del reporte (AAAA o AAAA-MM) y fuente (canal o plataforma que reportó). No trae importes.",
		Esquema: json.RawMessage(`{"type":"object","properties":{` +
			`"periodo":{"type":"string","description":"AAAA o AAAA-MM."},` +
			`"fuente":{"type":"string","description":"Fuente exacta del reporte, p. ej. caracol."},` +
			`"limite":{"type":"integer","description":"Cuántas obras devolver, de 1 a 100."}` +
			`},"additionalProperties":false}`),
		Ejecutar: func(ctx context.Context, actor aplicacion.Usuario, args json.RawMessage) (any, error) {
			var a struct {
				Periodo string `json:"periodo"`
				Fuente  string `json:"fuente"`
				Limite  int    `json:"limite"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, fmt.Errorf("%w: %v", aplicacion.ErrArgumentosInvalidos, err)
			}
			cola, err := uc.Ejecutar(ctx, actor, aplicacion.FiltroONI{Periodo: a.Periodo, Fuente: a.Fuente, Limite: a.Limite})
			if err != nil {
				return nil, err
			}
			out := colaONIJSON{PendientesTotal: cola.Pendientes, Obras: []oniJSON{}}
			for _, o := range cola.Obras {
				out.Obras = append(out.Obras, oniJSON{
					UsoID: o.UsoID, Titulo: o.Titulo, TituloOriginal: o.TituloOriginal, Fuente: o.Fuente,
					Modalidad: o.Modalidad, Periodo: o.Periodo, IDsFuente: o.IDsFuente, ReporteID: o.ReporteID,
					DetectadaEn: o.DetectadaEn.UTC().Format("2006-01-02"), CandidatosDudosos: o.Candidatos,
				})
			}
			return out, nil
		},
	}
}
