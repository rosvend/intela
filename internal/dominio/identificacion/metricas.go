package identificacion

import "github.com/shopspring/decimal"

// Evaluacion es una fila del conjunto etiquetado: lo que decidio la cascada
// contra lo que un humano dijo. ObraEsperada vacia = no deberia resolverse.
type Evaluacion struct {
	ObraEsperada string
	Resultado    Resultado
}

// Reporte son los recuentos sobre el conjunto etiquetado. Las categorias
// cierran contra Total, y hay una prueba que lo vigila.
type Reporte struct {
	Total       int
	Automaticas int // la cascada asigno obra, por el escalon que fuera
	Aciertos    int // de las automaticas, las que dieron con la obra etiquetada
	Fallos      int // de las automaticas, las que dieron con otra
	ABanda      int // ONI con candidatos: hay algo que un humano puede mirar
	AONI        int // ONI sin candidatos: no se parecio a nada
	Excluidas   int // fuera de repertorio (R-27): ni se intento identificar
	Manuales    int // decision de una persona (#39): no es de la cascada, no cuenta como automatica
}

// Metricas cuenta un conjunto ya resuelto: pura, probable sin base de datos.
func Metricas(evs []Evaluacion) Reporte {
	var r Reporte
	r.Total = len(evs)

	for _, ev := range evs {
		switch {
		case ev.Resultado.Escalon == EscalonExcluido:
			r.Excluidas++
		// Antes de mirar la obra: una resolucion manual TAMBIEN trae ObraID, y
		// contarla como automatica inflaria KR-2 con trabajo humano.
		case ev.Resultado.Escalon == EscalonManual:
			r.Manuales++
		case ev.Resultado.ObraID != "":
			r.Automaticas++
			if ev.Resultado.ObraID == ev.ObraEsperada {
				r.Aciertos++
			} else {
				r.Fallos++
			}
		case len(ev.Resultado.Candidatos) > 0:
			r.ABanda++
		default:
			r.AONI++
		}
	}
	return r
}

// TasaAutoAsociacionPct es KR-2 (">= 90%"), en unidad 0-100. La unidad va en
// el nombre: confundirla con una fraccion es un error de factor 100.
//
// El denominador es Total - Excluidas: las filas fuera de repertorio (R-27) no
// se intentan identificar, y contarlas como "no auto-asociadas" castigaria a la
// cascada por algo que no le toca. Las manuales SI estan en el denominador: la
// cascada no las resolvio, y no suben el numerador.
func (r Reporte) TasaAutoAsociacionPct() decimal.Decimal {
	return porcentaje(r.Automaticas, r.Total-r.Excluidas)
}

// PrecisionPct es KR-2 (">= 95%"), medido SOLO sobre lo que se asigno solo:
// sobre el total premiaria a un umbral tan alto que no asigna nada.
func (r Reporte) PrecisionPct() decimal.Decimal {
	return porcentaje(r.Aciertos, r.Automaticas)
}

// porcentaje devuelve cero sin denominador: nada auto-asociado es un resultado
// legitimo, no un panico.
func porcentaje(parte, total int) decimal.Decimal {
	if total == 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(parte)).
		Mul(decimal.NewFromInt(100)).
		Div(decimal.NewFromInt(int64(total)))
}
