package identificacion

import (
	"fmt"
	"slices"

	"github.com/shopspring/decimal"
)

// Escalones del vocabulario del CHECK de usos.escalon (00001_init.sql).
//
// EscalonExcluido se persiste desde la migracion 00007 (ver D4 de
// docs/planes/28-cascada-identificacion/01-design.md): la fila queda sin obra
// y con oni=false, porque fuera de repertorio no es lo mismo que no
// identificada (criterio 4 de #28).
//
// EscalonPendiente es el estado con que la ingesta siembra cada fila, y al
// que vuelve una fila excluida cuya fuente deja de estarlo y no resuelve.
const (
	EscalonPendiente = "pendiente"
	EscalonAlias     = "alias"
	EscalonIDGlobal  = "id_global"
	EscalonExcluido  = "excluido"
	EscalonDifuso    = "difuso"
	EscalonONI       = "oni"
)

// IDGlobal identifica cual de los tres identificadores globales caso en el
// escalon 2.
type IDGlobal string

const (
	IDA  IDGlobal = "ida"
	EIDR IDGlobal = "eidr"
	IMDB IDGlobal = "imdb"
)

// OrdenIDGlobal es el orden fijo en que se sondea el escalon 2. Determinismo
// (ADR 0005): dos corridas sobre el mismo dato prueban los identificadores en
// el mismo orden y paran en el mismo primero que case.
var OrdenIDGlobal = []IDGlobal{IDA, EIDR, IMDB}

// FuentesExcluidas es el conjunto de fuentes fuera de repertorio (R-27, RD
// 9.5). Vacio significa que nada se excluye: es el valor de arranque en
// produccion, hasta que exista el dato de politica (ver D4 del diseno).
type FuentesExcluidas []string

// Excluye dice si fuente esta fuera de repertorio.
func (f FuentesExcluidas) Excluye(fuente string) bool {
	return slices.Contains(f, fuente)
}

// Consulta trae lo que respondieron los sondeos de datos. Resolver no hace
// E/S: el caso de uso consulta los puertos y rellena esto antes de llamarla.
type Consulta struct {
	AliasObraID    string   // lo que devolvio Alias(); "" = sin alias
	IDGlobalObraID string   // obra del primer id global que caso; "" = ninguno
	IDGlobalCual   IDGlobal // cual de los tres caso; "" si no se sondeo

	// Candidatos del escalon 3, ya puntuados. Vacio = el motor no propuso nada.
	Candidatos []Candidato
}

// Umbrales son los dos cortes del escalon 3, en la escala 0-1 del motor.
//
// Dos y no uno porque hay tres desenlaces: asignar, pedir revision humana y
// declarar que no se parece a nada. Son parametros normativos, no constantes
// (ADR 0004); arrancan conservadores a proposito. Ver D2 de
// docs/planes/32-difuso/diseno.md.
type Umbrales struct {
	Match decimal.Decimal // >= Match: asignacion automatica
	Banda decimal.Decimal // >= Banda y < Match: cola manual con los candidatos
}

// Resolver aplica la cascada completa del ADR 0007 y devuelve el Resultado.
//
// El repertorio corta primero (R-27), luego alias, luego id global, luego
// difuso: un parecido de 0.99 no desbanca a una igualdad. Lo que no resuelve
// nadie sale con EscalonONI, que es un estado del modelo y no un fallo
// (RD 13.8).
func Resolver(e Entrada, c Consulta, excluidas FuentesExcluidas, u Umbrales) Resultado {
	if excluidas.Excluye(e.Fuente) {
		return Resultado{
			Escalon:   EscalonExcluido,
			Evidencia: fmt.Sprintf("fuera de repertorio: %s", e.Fuente),
		}
	}
	if c.AliasObraID != "" {
		return Resultado{
			ObraID:    c.AliasObraID,
			Escalon:   EscalonAlias,
			Puntaje:   decimal.NewFromInt(1),
			Evidencia: fmt.Sprintf("alias %s %s=%s -> %s", e.Fuente, e.TipoID, e.ValorID, c.AliasObraID),
		}
	}
	if c.IDGlobalObraID != "" && c.IDGlobalCual != "" {
		return Resultado{
			ObraID:    c.IDGlobalObraID,
			Escalon:   EscalonIDGlobal,
			Puntaje:   decimal.NewFromInt(1),
			Evidencia: fmt.Sprintf("%s %s -> %s", c.IDGlobalCual, valorGlobal(e, c.IDGlobalCual), c.IDGlobalObraID),
		}
	}
	return difuso(e, c.Candidatos, u)
}

// difuso aplica los dos cortes a los candidatos ya puntuados. No calcula
// similitud -eso es del adaptador- ni mira creditos (R-02).
func difuso(e Entrada, candidatos []Candidato, u Umbrales) Resultado {
	mejor, hay := mejorCandidato(candidatos)

	switch {
	case hay && mejor.Puntaje.GreaterThanOrEqual(u.Match):
		return Resultado{
			ObraID:  mejor.ObraID,
			Escalon: EscalonDifuso,
			Puntaje: mejor.Puntaje,
			Evidencia: fmt.Sprintf("difuso %q ~ %s (%s)",
				e.Titulo, mejor.ObraID, puntajeLegible(mejor.Puntaje)),
		}

	// Banda ambigua: no asigna, pero adjunta lo que un humano puede revisar.
	case hay && mejor.Puntaje.GreaterThanOrEqual(u.Banda):
		return Resultado{
			Escalon: EscalonONI,
			ONI:     true,
			Evidencia: fmt.Sprintf("banda ambigua: %d candidatos, mejor %s (%s) bajo umbral %s",
				len(candidatos), mejor.ObraID, puntajeLegible(mejor.Puntaje), puntajeLegible(u.Match)),
			Candidatos: candidatos,
		}

	// Por debajo del piso no se adjunta nada: un 0.02 no es material de revision.
	default:
		return Resultado{
			Escalon:   EscalonONI,
			ONI:       true,
			Evidencia: fmt.Sprintf("sin candidato sobre %s", puntajeLegible(u.Banda)),
		}
	}
}

// mejorCandidato elige por puntaje y desempata por ObraID ascendente: sin
// criterio propio, dos corridas podrian pagar a obras distintas (ADR 0005).
func mejorCandidato(candidatos []Candidato) (Candidato, bool) {
	var mejor Candidato
	hay := false
	for _, c := range candidatos {
		switch {
		case !hay,
			c.Puntaje.GreaterThan(mejor.Puntaje),
			c.Puntaje.Equal(mejor.Puntaje) && c.ObraID < mejor.ObraID:
			mejor, hay = c, true
		}
	}
	return mejor, hay
}

// puntajeLegible fija cinco decimales, la precision de usos.puntaje, para que
// la evidencia sea comparable entre corridas.
func puntajeLegible(d decimal.Decimal) string { return d.StringFixed(5) }

// valorGlobal devuelve el identificador de e que corresponde a cual, para que
// la evidencia nombre el valor exacto que caso.
func valorGlobal(e Entrada, cual IDGlobal) string {
	switch cual {
	case IDA:
		return e.IDA
	case EIDR:
		return e.EIDR
	case IMDB:
		return e.IMDB
	default:
		return ""
	}
}
