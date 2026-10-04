package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Explicador es lo que HTTP necesita de ExplicarCifra.
type Explicador interface {
	Explicar(ctx context.Context, actor aplicacion.Usuario, ref string) (aplicacion.Explicacion, error)
}

var _ Explicador = aplicacion.ExplicarCifra{}

type corridaJSON struct {
	ProcesoID string `json:"proceso_id"`
	Periodo   string `json:"periodo"`
	Circuito  string `json:"circuito"`
}

type recaudoJSON struct {
	Convenio string `json:"convenio"`
	Tarifa   string `json:"tarifa"`
	Factura  string `json:"factura"`
}

type bolsaLinajeJSON struct {
	ID        string       `json:"id"`
	UsuarioID string       `json:"usuario_id"`
	Bruto     string       `json:"bruto"`
	Recaudo   *recaudoJSON `json:"recaudo"`
}

type reporteLinajeJSON struct {
	ID          string `json:"id"`
	Fuente      string `json:"fuente"`
	SHA256      string `json:"sha256"`
	ClaveObjeto string `json:"clave_objeto"`
}

type obraLinajeJSON struct {
	ID      string `json:"id"`
	Titulo  string `json:"titulo"`
	Escalon string `json:"escalon"`
	Puntaje string `json:"puntaje"`
	Puntos  string `json:"puntos"`
}

type identificacionJSON struct {
	UsoID       string `json:"uso_id"`
	ReporteID   string `json:"reporte_id"`
	Escalon     string `json:"escalon"`
	Puntaje     string `json:"puntaje,omitempty"`
	Evidencia   string `json:"evidencia,omitempty"`
	ResueltoPor string `json:"resuelto_por,omitempty"`
	ResueltoEn  string `json:"resuelto_en,omitempty"`
}

type factorJSON struct {
	Nombre string `json:"nombre"`
	Valor  string `json:"valor"`
	Origen string `json:"origen"`
}

type terminoJSON struct {
	Factores []factorJSON `json:"factores"`
	Producto string       `json:"producto"`
}

type valorizacionJSON struct {
	UsoID    string        `json:"uso_id"`
	Formula  string        `json:"formula"`
	Puntos   string        `json:"puntos"`
	Terminos []terminoJSON `json:"terminos"`
}

type reglaJSON struct {
	SnapshotID string `json:"snapshot_id"`
	Reglamento string `json:"reglamento"`
}

type splitJSON struct {
	TitularID  string `json:"titular_id"`
	IPI        string `json:"ipi"`
	Porcentaje string `json:"porcentaje"`
	Version    *int   `json:"version"`
}

type deduccionLinajeJSON struct {
	Concepto   string `json:"concepto"`
	Porcentaje string `json:"porcentaje"`
	Monto      string `json:"monto"`
}

type firmaLinajeJSON struct {
	Rol           string `json:"rol"`
	ActorID       string `json:"actor_id"`
	SobreRevision int    `json:"sobre_revision"`
	Etapa         string `json:"etapa"`
	Cuando        string `json:"cuando"`
}

// explicacionJSON es el contrato de GET /explicar/{ref}; el dinero viaja como cadena (ADR 0010).
type explicacionJSON struct {
	Ref            string                `json:"ref"`
	TitularID      string                `json:"titular_id,omitempty"`
	Neto           string                `json:"neto"`
	Bruto          string                `json:"bruto"`
	Retenida       bool                  `json:"retenida"`
	Motivo         string                `json:"motivo,omitempty"`
	Corrida        corridaJSON           `json:"corrida"`
	Bolsa          bolsaLinajeJSON       `json:"bolsa"`
	Reporte        reporteLinajeJSON     `json:"reporte"`
	Reportes       []reporteLinajeJSON   `json:"reportes"`
	Obra           obraLinajeJSON        `json:"obra"`
	Identificacion []identificacionJSON  `json:"identificacion"`
	Valorizacion   []valorizacionJSON    `json:"valorizacion"`
	Regla          reglaJSON             `json:"regla"`
	Split          *splitJSON            `json:"split"`
	Deducciones    []deduccionLinajeJSON `json:"deducciones"`
	Firmas         []firmaLinajeJSON     `json:"firmas"`
	Faltantes      []string              `json:"faltantes"`
}

func aReporteLinajeJSON(r aplicacion.ReporteAsentado) reporteLinajeJSON {
	return reporteLinajeJSON{ID: r.ID, Fuente: r.Fuente, SHA256: r.SHA256, ClaveObjeto: r.ClaveObjeto}
}

func aValorizacionJSON(v aplicacion.ValorizacionDeUso) valorizacionJSON {
	out := valorizacionJSON{UsoID: v.UsoID, Formula: v.Formula, Puntos: v.Puntos, Terminos: make([]terminoJSON, 0, len(v.Terminos))}
	for _, t := range v.Terminos {
		tj := terminoJSON{Producto: t.Producto, Factores: make([]factorJSON, 0, len(t.Factores))}
		for _, f := range t.Factores {
			tj.Factores = append(tj.Factores, factorJSON{Nombre: f.Nombre, Valor: f.Valor, Origen: f.Origen})
		}
		out.Terminos = append(out.Terminos, tj)
	}
	return out
}

func aExplicacionJSON(x aplicacion.Explicacion) explicacionJSON {
	out := explicacionJSON{
		Ref: x.Ref, TitularID: x.TitularID,
		Neto: x.Neto.StringFixed(2), Bruto: x.Bruto.StringFixed(2),
		Retenida: x.Retenida, Motivo: x.Motivo,
		Corrida: corridaJSON{ProcesoID: x.Corrida.ProcesoID, Periodo: x.Corrida.Periodo, Circuito: x.Corrida.Circuito},
		Bolsa:   bolsaLinajeJSON{ID: x.Bolsa.ID, UsuarioID: x.Bolsa.UsuarioID, Bruto: x.Bolsa.Bruto.StringFixed(2)},
		Reporte: aReporteLinajeJSON(x.Reporte),
		Obra:    obraLinajeJSON{ID: x.Obra.ID, Titulo: x.Obra.Titulo, Escalon: x.Obra.Escalon, Puntaje: x.Obra.Puntaje, Puntos: x.Obra.Puntos},
		Regla:   reglaJSON{SnapshotID: x.Regla.SnapshotID, Reglamento: x.Regla.Reglamento},

		Reportes:       make([]reporteLinajeJSON, 0, len(x.Reportes)),
		Identificacion: make([]identificacionJSON, 0, len(x.Identificacion)),
		Valorizacion:   make([]valorizacionJSON, 0, len(x.Valorizacion)),
		Deducciones:    make([]deduccionLinajeJSON, 0, len(x.Deducciones)),
		Firmas:         make([]firmaLinajeJSON, 0, len(x.Firmas)),
		Faltantes:      append(make([]string, 0, len(x.Faltantes)), x.Faltantes...),
	}
	if r := x.Bolsa.Recaudo; r != nil {
		out.Bolsa.Recaudo = &recaudoJSON{Convenio: r.Convenio, Tarifa: r.Tarifa, Factura: r.Factura}
	}
	if s := x.Split; s != nil {
		out.Split = &splitJSON{TitularID: s.TitularID, IPI: s.IPI, Porcentaje: s.Porcentaje.String(), Version: s.Version}
	}
	for _, r := range x.Reportes {
		out.Reportes = append(out.Reportes, aReporteLinajeJSON(r))
	}
	for _, u := range x.Identificacion {
		out.Identificacion = append(out.Identificacion, identificacionJSON(u))
	}
	for _, v := range x.Valorizacion {
		out.Valorizacion = append(out.Valorizacion, aValorizacionJSON(v))
	}
	for _, d := range x.Deducciones {
		out.Deducciones = append(out.Deducciones, deduccionLinajeJSON{Concepto: d.Concepto, Porcentaje: d.Porcentaje.String(), Monto: d.Monto.StringFixed(2)})
	}
	for _, f := range x.Firmas {
		out.Firmas = append(out.Firmas, firmaLinajeJSON{
			Rol: f.Rol, ActorID: f.ActorID, SobreRevision: f.SobreRevision, Etapa: f.Etapa,
			Cuando: f.Cuando.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}

// explicarCifra sirve el linaje de una cifra; el alcance del titular lo decide el caso de uso.
func (a *API) explicarCifra(w http.ResponseWriter, r *http.Request) {
	if a.explicar == nil {
		escribirError(w, http.StatusServiceUnavailable,
			"explicar una cifra no esta configurado en esta instalacion")
		return
	}
	usuario, hay := UsuarioDe(r.Context())
	if !hay {
		noAutenticado(w, "sesion invalida o expirada")
		return
	}
	// chi entrega el segmento todavia codificado cuando trae %3A (el frontend usa encodeURIComponent).
	ref, err := url.PathUnescape(chi.URLParam(r, "ref"))
	if err != nil {
		escribirError(w, http.StatusNotFound, "no hay linaje asentado para esa cifra")
		return
	}
	x, err := a.explicar.Explicar(r.Context(), usuario, ref)
	switch {
	case err == nil:
		escribirJSON(w, http.StatusOK, aExplicacionJSON(x))
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "no hay linaje asentado para esa cifra")
	case errors.Is(err, aplicacion.ErrNoAutorizado):
		escribirError(w, http.StatusForbidden, aplicacion.ErrNoAutorizado.Error())
	default:
		a.log.ErrorContext(r.Context(), "explicar cifra", "error", err)
		escribirError(w, http.StatusInternalServerError, "no se pudo explicar la cifra")
	}
}
