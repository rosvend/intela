package identificacion

import (
	"fmt"
	"slices"

	"github.com/shopspring/decimal"
)

// Escalones del vocabulario del CHECK de usos.escalon (00001_init.sql).
//
// EscalonExcluido solo vive en Resultado: no es un valor del CHECK y este
// issue no lo persiste (ver D4 de docs/planes/28-cascada-identificacion/01-design.md).
// Una fila excluida no se toca: ni escalon, ni oni, ni obra_id cambian.
const (
	EscalonAlias    = "alias"
	EscalonIDGlobal = "id_global"
	EscalonExcluido = "excluido"
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
}

// Resolver aplica los escalones 0-2 de la cascada del ADR 0007 y devuelve el
// Resultado. Un Resultado con ObraID == "" y Escalon == "" significa "no
// resuelto": la fila queda pendiente, es el insumo del difuso (#32).
//
// Orden, y por que ese orden: el filtro de repertorio corre primero porque una
// fila fuera de repertorio no debe generar ONI ni consumir un sondeo de alias
// o de id global (R-27); el alias manda sobre el id global porque, en la
// operacion real, el caso de uso solo sondea el escalon 2 cuando el 1 no
// pego -pero el orden aqui esta definido igual para que la funcion sea total
// sin importar que traiga Consulta.
func Resolver(e Entrada, c Consulta, excluidas FuentesExcluidas) Resultado {
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
	return Resultado{}
}

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
