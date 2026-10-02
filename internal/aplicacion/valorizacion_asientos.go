package aplicacion

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Hechos de la valorizacion (RD 9, RD 13.5 "Importe de la Obra").
const (
	HechoRepartoValorizado     = "reparto.valorizado"
	HechoRepartoObraValorizada = "reparto.obra_valorizada"
)

// OrigenDeUso es la procedencia de un uso que pondero: su archivo exacto y como se identifico su obra.
type OrigenDeUso struct {
	UsoID       string
	ReporteID   string
	Fuente      string
	SHA256      string
	ClaveObjeto string
	Escalon     string
	Puntaje     *decimal.Decimal
	Evidencia   string
	ResueltoPor string
	ResueltoEn  *time.Time
}

// AsientoValorizacion es el payload de reparto.valorizado: de donde salio y como cerro la corrida.
type AsientoValorizacion struct {
	ProcesoID     string              `json:"proceso_id"`
	Periodo       string              `json:"periodo"`
	Circuito      string              `json:"circuito"`
	Bolsa         BolsaAsentada       `json:"bolsa"`
	SnapshotID    string              `json:"snapshot_id"`
	Reglamento    string              `json:"reglamento"`
	Deducciones   []DeduccionAsentada `json:"deducciones"`
	Neto          string              `json:"neto"`
	Retenido      string              `json:"retenido"`
	Residuo       string              `json:"residuo"`
	NoDistribuido string              `json:"no_distribuido"`
	ValorPunto    string              `json:"valor_punto"`
	// Netos es el vector de titulares del proceso, ordenado por obra y titular.
	// ExplicarCifra prorratea con el mismo vector que el export (#43).
	Netos                []NetoTitularAsentado  `json:"netos,omitempty"`
	PartesNoDistribuidas []ParteNoDistribuidaAs `json:"partes_no_distribuidas"`
	PorGrupo             []GrupoAsentado        `json:"por_grupo"`
	Reportes             []ReporteAsentado      `json:"reportes"`
}

// NetoTitularAsentado es una linea del vector con el que se prorratean las deducciones.
type NetoTitularAsentado struct {
	ObraID    string `json:"obra_id"`
	TitularID string `json:"titular_id"`
	Importe   string `json:"importe"`
}

// BolsaAsentada es la bolsa que se repartio.
type BolsaAsentada struct {
	ID        string `json:"id"`
	UsuarioID string `json:"usuario_id"`
	Bruto     string `json:"bruto"`
}

// ParteNoDistribuidaAs es un tramo de NoDistribuido con su motivo (RD 16).
type ParteNoDistribuidaAs struct {
	Motivo  string `json:"motivo"`
	Grupo   string `json:"grupo,omitempty"`
	ObraID  string `json:"obra_id,omitempty"`
	Importe string `json:"importe"`
}

// GrupoAsentado es el valor punto de un grupo de canal (RD 9.5).
type GrupoAsentado struct {
	Grupo       string `json:"grupo"`
	Bolsa       string `json:"bolsa"`
	TotalPuntos string `json:"total_puntos"`
	ValorPunto  string `json:"valor_punto"`
	Residuo     string `json:"residuo"`
}

// ReporteAsentado es la version exacta de un archivo crudo que pondero (ADR 0006, pregunta 2).
type ReporteAsentado struct {
	ID          string `json:"id"`
	Fuente      string `json:"fuente"`
	SHA256      string `json:"sha256"`
	ClaveObjeto string `json:"clave_objeto"`
}

// AsientoObraValorizada es el payload de reparto.obra_valorizada: el importe de una obra y su linaje.
type AsientoObraValorizada struct {
	ProcesoID   string                `json:"proceso_id"`
	Periodo     string                `json:"periodo"`
	Puntos      string                `json:"puntos"`
	Importe     string                `json:"importe"`
	Retenida    bool                  `json:"retenida"`
	Motivo      string                `json:"motivo,omitempty"`
	Declaracion *DeclaracionAsentada  `json:"declaracion"`
	Titulares   []TitularAsentado     `json:"titulares"`
	Usos        []IdentificacionDeUso `json:"usos"`
}

// DeclaracionAsentada es la version de la declaracion que fijo los porcentajes.
type DeclaracionAsentada struct {
	Version      int    `json:"version"`
	VigenteDesde string `json:"vigente_desde"`
}

// TitularAsentado es la parte de un titular en la obra.
//
// DeclaracionVersion es la version sellada en la linea al valorizar. Un
// asiento anterior a #183 no la trae: omitempty la deja fuera y explicar
// cae al asiento de la obra.
type TitularAsentado struct {
	TitularID          string `json:"titular_id"`
	IPI                string `json:"ipi"`
	Porcentaje         string `json:"porcentaje"`
	Importe            string `json:"importe"`
	DeclaracionVersion *int   `json:"declaracion_version,omitempty"`
}

// IdentificacionDeUso es como se reconocio la obra en un uso (ADR 0007).
type IdentificacionDeUso struct {
	UsoID       string `json:"uso_id"`
	ReporteID   string `json:"reporte_id"`
	Escalon     string `json:"escalon"`
	Puntaje     string `json:"puntaje,omitempty"`
	Evidencia   string `json:"evidencia,omitempty"`
	ResueltoPor string `json:"resuelto_por,omitempty"`
	ResueltoEn  string `json:"resuelto_en,omitempty"`
}

// entradaValorizacion reune lo que el motor recibio y devolvio en una corrida.
type entradaValorizacion struct {
	proceso   reparto.ProcesoDeReparto
	bolsa     BolsaPersistida
	snap      reparto.Snapshot
	resultado reparto.Resultado
	vigentes  map[string]VersionDeclaracion
	usos      []UsoDeReparto
	origen    map[string]OrigenDeUso
}

// asientosDeValorizacion arma el asiento de la corrida y uno por obra; un uso sin origen aborta.
func asientosDeValorizacion(e entradaValorizacion) ([]pendiente, error) {
	usosPorObra := make(map[string][]IdentificacionDeUso)
	reportes := make(map[string]ReporteAsentado)
	for _, u := range e.usos {
		o, ok := e.origen[u.Uso.ID]
		if !ok {
			return nil, fmt.Errorf("uso %q: %w", u.Uso.ID, ErrLinajeIncompleto)
		}
		usosPorObra[u.Uso.ObraID] = append(usosPorObra[u.Uso.ObraID], identificacionDe(o))
		reportes[o.ReporteID] = ReporteAsentado{ID: o.ReporteID, Fuente: o.Fuente, SHA256: o.SHA256, ClaveObjeto: o.ClaveObjeto}
	}

	p, r := e.proceso, e.resultado
	corrida := AsientoValorizacion{
		ProcesoID: p.ID, Periodo: p.Periodo, Circuito: string(p.Circuito),
		Bolsa:      BolsaAsentada{ID: e.bolsa.ID, UsuarioID: e.bolsa.UsuarioID, Bruto: e.bolsa.Bruto.StringFixed(2)},
		SnapshotID: r.SnapshotID, Reglamento: r.Reglamento,
		Deducciones: []DeduccionAsentada{
			{Concepto: liquidacion.ConceptoAdministracion, Porcentaje: e.snap.AdminPct.String(), Monto: r.Admin.StringFixed(2)},
			{Concepto: liquidacion.ConceptoSocial, Porcentaje: e.snap.SocialPct.String(), Monto: r.Social.StringFixed(2)},
			{Concepto: liquidacion.ConceptoReserva, Porcentaje: e.snap.ReservaPct.String(), Monto: r.Reserva.StringFixed(2)},
		},
		Neto: r.Neto.StringFixed(2), Retenido: r.Retenido.StringFixed(2), Residuo: r.Residuo.StringFixed(2),
		NoDistribuido: r.NoDistribuido.StringFixed(2), ValorPunto: r.ValorPunto.String(),
		PartesNoDistribuidas: make([]ParteNoDistribuidaAs, 0, len(r.PartesNoDistribuidas)),
		PorGrupo:             make([]GrupoAsentado, 0, len(r.PorGrupo)),
		Reportes:             make([]ReporteAsentado, 0, len(reportes)),
	}
	for _, pn := range r.PartesNoDistribuidas {
		corrida.PartesNoDistribuidas = append(corrida.PartesNoDistribuidas, ParteNoDistribuidaAs{
			Motivo: string(pn.Motivo), Grupo: string(pn.Grupo), ObraID: pn.ObraID, Importe: pn.Importe.StringFixed(2),
		})
	}
	for _, g := range r.PorGrupo {
		corrida.PorGrupo = append(corrida.PorGrupo, GrupoAsentado{
			Grupo: string(g.Grupo), Bolsa: g.Bolsa.StringFixed(2), TotalPuntos: g.TotalPuntos.String(),
			ValorPunto: g.ValorPunto.String(), Residuo: g.Residuo.StringFixed(2),
		})
	}
	for _, rep := range reportes {
		corrida.Reportes = append(corrida.Reportes, rep)
	}
	sort.Slice(corrida.Reportes, func(i, j int) bool { return corrida.Reportes[i].ID < corrida.Reportes[j].ID })

	netos := make([]NetoTitularAsentado, 0, len(r.Titulares))
	for _, t := range r.Titulares {
		netos = append(netos, NetoTitularAsentado{
			ObraID: t.ObraID, TitularID: t.TitularID, Importe: t.Importe.StringFixed(2),
		})
	}
	sort.Slice(netos, func(i, j int) bool {
		if netos[i].ObraID != netos[j].ObraID {
			return netos[i].ObraID < netos[j].ObraID
		}
		return netos[i].TitularID < netos[j].TitularID
	})
	corrida.Netos = netos

	titularesPorObra := make(map[string][]TitularAsentado)
	for _, t := range r.Titulares {
		titularesPorObra[t.ObraID] = append(titularesPorObra[t.ObraID], TitularAsentado{
			TitularID: t.TitularID, IPI: t.IPI, Porcentaje: t.Porcentaje.String(), Importe: t.Importe.StringFixed(2),
			DeclaracionVersion: copiarVersion(t.DeclaracionVersion),
		})
	}

	asientos := []pendiente{{hecho: HechoRepartoValorizado, refTipo: RefProceso, refID: p.ID, payload: corrida}}
	for _, o := range r.Obras {
		obra := AsientoObraValorizada{
			ProcesoID: p.ID, Periodo: p.Periodo,
			Puntos: o.Puntos.String(), Importe: o.Importe.StringFixed(2),
			Retenida: o.Retenida, Motivo: o.Motivo,
			Titulares: titularesPorObra[o.ObraID], Usos: usosPorObra[o.ObraID],
		}
		if obra.Titulares == nil {
			obra.Titulares = []TitularAsentado{}
		}
		if vd, ok := e.vigentes[o.ObraID]; ok {
			obra.Declaracion = &DeclaracionAsentada{Version: vd.Version, VigenteDesde: diaCivil(vd.VigenteDesde)}
		}
		asientos = append(asientos, pendiente{hecho: HechoRepartoObraValorizada, refTipo: RefObra, refID: o.ObraID, payload: obra})
	}
	return asientos, nil
}

// conVersionDeDeclaracion sella en cada linea la version con la que el motor
// la repartio, tomada del mismo mapa de vigentes de esa corrida. Una obra
// ausente del mapa deja la version en nil: inventar 1 atribuiria un split
// que esta corrida no uso (#183).
func conVersionDeDeclaracion(r reparto.Resultado, vigentes map[string]VersionDeclaracion) reparto.Resultado {
	for i := range r.Titulares {
		vd, ok := vigentes[r.Titulares[i].ObraID]
		if !ok {
			continue
		}
		v := vd.Version
		r.Titulares[i].DeclaracionVersion = &v
	}
	return r
}

func copiarVersion(v *int) *int {
	if v == nil {
		return nil
	}
	copia := *v
	return &copia
}

func identificacionDe(o OrigenDeUso) IdentificacionDeUso {
	id := IdentificacionDeUso{
		UsoID: o.UsoID, ReporteID: o.ReporteID, Escalon: o.Escalon,
		Evidencia: o.Evidencia, ResueltoPor: o.ResueltoPor,
	}
	if o.Puntaje != nil && conPuntaje(o.Escalon) {
		id.Puntaje = o.Puntaje.String()
	}
	if o.ResueltoEn != nil {
		id.ResueltoEn = o.ResueltoEn.UTC().Format(time.RFC3339)
	}
	return id
}

// conPuntaje: alias e id global son exactos; solo el difuso y la decision manual llevan puntaje.
func conPuntaje(escalon string) bool {
	return escalon == identificacion.EscalonDifuso || escalon == identificacion.EscalonManual
}

func idsDeUsos(usos []UsoDeReparto) []string {
	ids := make([]string, 0, len(usos))
	for _, u := range usos {
		ids = append(ids, u.Uso.ID)
	}
	return ids
}
