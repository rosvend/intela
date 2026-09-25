package anomalias

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Detectar corre los seis detectores y devuelve lo hallado en orden total (docs/planes/37/diseno.md D0).
func Detectar(p Periodo) []Hallazgo {
	hallazgos := make([]Hallazgo, 0)
	hallazgos = append(hallazgos, deteccionONI(p.Usos)...)
	hallazgos = append(hallazgos, duplicadosPorHuella(p.Periodo, p.Entregas)...)
	hallazgos = append(hallazgos, duplicadosPorRegistro(p.Usos)...)
	hallazgos = append(hallazgos, titularesSinPorcentaje(p.Obras)...)
	hallazgos = append(hallazgos, retencionPorDeclaracionIncompleta(p.Obras)...)
	hallazgos = append(hallazgos, tipoObraSinMapear(p.Usos)...)

	slices.SortFunc(hallazgos, func(a, b Hallazgo) int {
		if c := strings.Compare(a.Tipo, b.Tipo); c != 0 {
			return c
		}
		if c := strings.Compare(a.RefTipo, b.RefTipo); c != 0 {
			return c
		}
		if c := strings.Compare(a.RefID, b.RefID); c != 0 {
			return c
		}
		return strings.Compare(a.RefTitular, b.RefTitular)
	})
	return hallazgos
}

// ---------------------------------------------------------------------------
// 1. ONI

// deteccionONI alerta cada fila con escalon ONI; nunca por la bandera `oni`, y `excluido` no cuenta (D1).
func deteccionONI(usos []Uso) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.Escalon != identificacion.EscalonONI {
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoONI,
			RefTipo: RefUso,
			RefID:   u.ID,
			Detalle: fmt.Sprintf(
				"la cascada no reconocio %q (fuente %q, entrega %q): queda en la cola manual y su parte se retiene",
				u.Titulo, u.Fuente, u.ReporteID),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 2. Duplicado por huella de archivo

// duplicadosPorHuella alerta las entregas del periodo cuyos bytes ya llegaron bajo otra fuente, de cualquier periodo (D2).
func duplicadosPorHuella(periodo string, entregas []Entrega) []Hallazgo {
	porHuella := make(map[string][]Entrega, len(entregas))
	for _, e := range entregas {
		if e.SHA256 == "" {
			continue
		}
		porHuella[e.SHA256] = append(porHuella[e.SHA256], e)
	}

	out := make([]Hallazgo, 0)
	for _, e := range entregas {
		if e.Periodo != periodo {
			continue
		}
		colisiones := porHuella[e.SHA256]
		if len(colisiones) < 2 {
			continue
		}
		otras := make([]string, 0, len(colisiones)-1)
		for _, c := range colisiones {
			if c.ID == e.ID {
				continue
			}
			otras = append(otras, fmt.Sprintf("%s (fuente %q, periodo %q)", c.ID, c.Fuente, c.Periodo))
		}
		out = append(out, Hallazgo{
			Tipo:    TipoDuplicadoArchivo,
			RefTipo: RefReporte,
			RefID:   e.ID,
			// El detalle explica el mecanismo; no afirma nada fijo sobre el periodo del par.
			Detalle: fmt.Sprintf(
				"la entrega %q (fuente %q) trae los mismos bytes que %s; sha256 %s. El UNIQUE (sha256, fuente) de `reportes` no lo impide: solo cierra la puerta a la MISMA fuente reenviando los mismos bytes, y el periodo no entra en esa clave",
				e.ID, e.Fuente, strings.Join(otras, ", "), e.SHA256),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 3. Duplicado por registro logico

// duplicadosPorRegistro alerta la fila que repite (fuente, clave de registro) de otra del periodo; clave vacia no compara (D3).
func duplicadosPorRegistro(usos []Uso) []Hallazgo {
	type primera struct{ usoID, reporteID string }

	vistas := make(map[string]primera, len(usos))
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.ClaveRegistro == "" {
			continue
		}
		clave := u.Fuente + "\x00" + u.ClaveRegistro
		antes, repetida := vistas[clave]
		if !repetida {
			vistas[clave] = primera{usoID: u.ID, reporteID: u.ReporteID}
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoDuplicadoRegistro,
			RefTipo: RefUso,
			RefID:   u.ID,
			Detalle: fmt.Sprintf(
				"el registro %s de la fuente %q ya venia en el uso %q (entrega %q); esta fila llego en la entrega %q y contaria el mismo hecho dos veces",
				u.ClaveRegistro, u.Fuente, antes.usoID, antes.reporteID, u.ReporteID),
		})
	}
	return out
}

// SinClaveDeRegistro cuenta las filas que el detector de duplicados no pudo cotejar: el punto ciego (D3).
func SinClaveDeRegistro(usos []Uso) int {
	n := 0
	for _, u := range usos {
		if u.ClaveRegistro == "" {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// 4. Titulares sin porcentaje

// titularesSinPorcentaje nombra al coautor sin parte, o la parte sin IPI, de una obra con declaracion abierta (D4).
func titularesSinPorcentaje(obras []Obra) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, o := range obras {
		if !o.Declarada {
			continue
		}

		declarados := make(map[string]bool, len(o.Declaracion.Partes))
		for _, p := range o.Declaracion.Partes {
			declarados[p.IPI] = true
		}

		// Caso 1: coautor del catalogo sin parte.
		for _, ipi := range o.CoautoresIPI {
			if ipi == "" || declarados[ipi] {
				continue
			}
			out = append(out, Hallazgo{
				Tipo:       TipoTitularSinPorcentaje,
				RefTipo:    RefObra,
				RefID:      o.ID,
				RefTitular: PrefijoIPI + ipi,
				Detalle: fmt.Sprintf(
					"el coautor con IPI %s figura en el catalogo de la obra %q y no tiene parte en la declaracion vigente: su porcentaje no esta declarado y sin el no se le puede pagar (R-03)",
					ipi, o.ID),
			})
		}

		// Caso 2: parte sin IPI.
		for _, p := range o.Declaracion.Partes {
			if p.IPI != "" {
				continue
			}
			out = append(out, Hallazgo{
				Tipo:       TipoTitularSinPorcentaje,
				RefTipo:    RefObra,
				RefID:      o.ID,
				RefTitular: PrefijoTitular + p.TitularID,
				Detalle: fmt.Sprintf(
					"la parte del titular %q en la obra %q declara %s%% y no trae IPI: el porcentaje esta, pero no dice a quien se le paga",
					p.TitularID, o.ID, p.Porcentaje.String()),
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 5. Retencion por declaracion incompleta (R-04 / RD 13.1.3)

// retencionPorDeclaracionIncompleta alerta cada obra cuya declaracion no esta Completa(): R-04 retiene el total (D5).
func retencionPorDeclaracionIncompleta(obras []Obra) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, o := range obras {
		if o.Declarada && o.Declaracion.Completa() {
			continue
		}

		detalle := fmt.Sprintf(
			"la obra %q no tiene ninguna Declaracion de Obra%s: se retiene el total de lo que le corresponda en este periodo (R-04, RD 13.1.3)",
			o.ID, aQuienReclamar(o))
		if o.Declarada {
			detalle = fmt.Sprintf(
				"la declaracion vigente de la obra %q no esta completa: %s. Se retiene el TOTAL de esa obra, no se reparte la parte declarada (R-04, RD 13.1.3)",
				o.ID, motivoIncompleta(o.Declaracion))
		}

		out = append(out, Hallazgo{
			Tipo:    TipoReservaDeclaracionIncompleta,
			RefTipo: RefObra,
			RefID:   o.ID,
			Detalle: detalle,
		})
	}
	return out
}

// motivoIncompleta narra el motivo real por el que Completa() dijo que no, en su mismo orden.
func motivoIncompleta(d repertorio.Declaracion) string {
	if len(d.Partes) == 0 {
		return "la version vigente esta abierta y no tiene ninguna parte declarada"
	}

	sinIPI := make([]string, 0)
	noPositivas := make([]string, 0)
	suma := decimal.Zero
	for _, p := range d.Partes {
		if p.IPI == "" {
			sinIPI = append(sinIPI, p.TitularID)
		}
		if p.Porcentaje.LessThanOrEqual(decimal.Zero) {
			noPositivas = append(noPositivas, p.TitularID)
		}
		suma = suma.Add(p.Porcentaje)
	}

	motivos := make([]string, 0, 3)
	if len(sinIPI) > 0 {
		motivos = append(motivos, fmt.Sprintf(
			"%d de %d parte(s) no traen IPI (titular(es) %s), asi que no dicen a quien se le paga",
			len(sinIPI), len(d.Partes), strings.Join(sinIPI, ", ")))
	}
	if len(noPositivas) > 0 {
		motivos = append(motivos, fmt.Sprintf(
			"%d parte(s) no son positivas (titular(es) %s)",
			len(noPositivas), strings.Join(noPositivas, ", ")))
	}
	if !suma.Equal(decimal.NewFromInt(100)) {
		motivos = append(motivos, fmt.Sprintf(
			"lo declarado suma %s%% en %d parte(s) y R-04 exige 100 exactos",
			suma.String(), len(d.Partes)))
	}
	if len(motivos) == 0 {
		// Inalcanzable si solo se llama con una declaracion que Completa() rechazo.
		return "no se pudo determinar el motivo"
	}
	return strings.Join(motivos, "; ")
}

// aQuienReclamar nombra los IPI del catalogo de una obra sin ninguna declaracion (vacio si no hay).
func aQuienReclamar(o Obra) string {
	ipis := make([]string, 0, len(o.CoautoresIPI))
	for _, ipi := range o.CoautoresIPI {
		if ipi == "" {
			continue
		}
		ipis = append(ipis, ipi)
	}
	if len(ipis) == 0 {
		return ""
	}
	return fmt.Sprintf(" y en el catalogo figura(n) %d coautor(es) a quien(es) reclamarla (IPI %s)",
		len(ipis), strings.Join(ipis, ", "))
}

// ---------------------------------------------------------------------------
// 6. tipo_obra sin mapear (RD 9.1.1)

// tipoObraSinMapear alerta la fila identificada sin tipo_obra en una modalidad cuyo motor lo lee (D6).
func tipoObraSinMapear(usos []Uso) []Hallazgo {
	out := make([]Hallazgo, 0)
	for _, u := range usos {
		if u.ObraID == "" || u.TipoObra != "" {
			continue
		}
		if !ModalidadPonderaPorTipoObra(u.Modalidad) {
			continue
		}
		out = append(out, Hallazgo{
			Tipo:    TipoTipoObraSinMapear,
			RefTipo: RefUso,
			RefID:   u.ID,
			// Preaviso: hoy la fila puede no llegar al motor (sin canal_id), por eso "en cuanto entre".
			Detalle: fmt.Sprintf(
				"el uso %q de la obra %q (fuente %q, modalidad %q) no trae tipo_obra: RD 9.1.1 pondera por cuatro categorias y el motor de %s aborta la corrida entera (ErrRepartoInvalido) en cuanto esta fila entre en una",
				u.ID, u.ObraID, u.Fuente, u.Modalidad, u.Modalidad),
		})
	}
	return out
}
