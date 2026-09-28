package liquidacion

import (
	"slices"

	"github.com/shopspring/decimal"
)

// ResiduoProrrateo es lo que de cada concepto de deduccion NO quedo asignado
// a ninguna orden tras redondear a dos decimales (ADR 0005).
//
// Es exactamente `concepto - suma(asignado)`. Incluye el residuo puro de
// redondeo y, cuando hay RETENIDO, la parte de las deducciones que
// corresponderia al neto no distribuido: esa parte no se reparte a los
// pagados (ver [Prorratear]), y sin este campo desapareceria sin rastro.
//
// Puede ser NEGATIVO: Round(2) (mitad hacia arriba) puede hacer que la suma
// asignada se pase del concepto (p. ej. 0.02 entre tres partes iguales da
// 0.01+0.01+0.01=0.03). Eso es correcto contablemente; no se "arregla" con
// Abs ni se absorbe en la ultima orden.
//
// Quien liquida lo registra: no se absorbe en la ultima orden ni se pierde.
type ResiduoProrrateo struct {
	Admin   decimal.Decimal
	Social  decimal.Decimal
	Reserva decimal.Decimal
}

// Total es la suma de los tres conceptos no asignados.
func (r ResiduoProrrateo) Total() decimal.Decimal {
	return r.Admin.Add(r.Social).Add(r.Reserva)
}

// Prorratear reparte las deducciones de la corrida entre los titulares, en
// proporcion a lo que cada uno se lleva.
//
// # El denominador es el neto de la corrida
//
// netoProc es `bruto - admin - social - reserva`: lo que la corrida dejo para
// repartir. NO es la suma de las lineas de titular, y la diferencia no es
// cosmetica.
//
// Las dos cifras coinciden cuando todo el neto se distribuye, y entonces
// cualquiera de las dos sirve. Dejan de coincidir en cuanto hay RETENIDO --
// una obra con declaracion incompleta (`R-04`) o con un titular sin
// documentos: su importe se calcula pero no se reparte --, y ahi la suma de
// las lineas es MENOR que el neto. Usarla como denominador subiria la
// proporcion de cada titular hasta que las proporciones sumaran 1, es decir
// repartiria entre los pagados las deducciones que le tocaban al retenido: la
// orden mostraria una tasa mayor que la que la corrida aplico de verdad.
//
// Con el neto de la corrida como denominador, la tasa que se lee en la orden
// -- `(admin+social+reserva) / bruto` -- es exactamente la de la corrida,
// haya retenido o no. Esa es la unica forma de que la orden y el cierre del
// periodo cuadren sin que nadie tenga que elegir entre dos cifras.
//
// # Lo que devuelve
//
// Por cada titular, los tres conceptos que RD 13.2 obliga a itemizar, en
// orden fijo. Una lista VACIA -- nunca nil -- cuando no hay nada que
// prorratear para ese titular: netoProc en cero o negativo, o neto del
// titular en cero.
//
// Redondea a dos decimales (escala de `ordenes_pago`). El residuo es
// explicito, igual que en `reparto/redondeo.go` (ADR 0005): no se absorbe en
// la ultima linea ni se descarta. Quien llama lo registra.
//
// Recorre los titulares en orden lexicografico para que el mismo input
// produzca el mismo residuo bit a bit (ADR 0005).
func Prorratear(
	netos map[string]decimal.Decimal,
	admin, social, reserva, netoProc decimal.Decimal,
) (map[string][]Deduccion, ResiduoProrrateo) {
	out := make(map[string][]Deduccion, len(netos))
	claves := make([]string, 0, len(netos))
	for id := range netos {
		claves = append(claves, id)
	}
	slices.Sort(claves)

	if netoProc.LessThanOrEqual(decimal.Zero) {
		for _, id := range claves {
			out[id] = []Deduccion{}
		}
		// Nada se asigno: el residuo es el total de cada concepto.
		return out, ResiduoProrrateo{Admin: admin, Social: social, Reserva: reserva}
	}

	adminAsig := decimal.Zero
	socialAsig := decimal.Zero
	reservaAsig := decimal.Zero
	for _, id := range claves {
		neto := netos[id]
		if neto.IsZero() {
			out[id] = []Deduccion{}
			continue
		}
		a := admin.Mul(neto).Div(netoProc).Round(2)
		s := social.Mul(neto).Div(netoProc).Round(2)
		r := reserva.Mul(neto).Div(netoProc).Round(2)
		out[id] = []Deduccion{
			{Concepto: ConceptoAdministracion, Monto: a},
			{Concepto: ConceptoSocial, Monto: s},
			{Concepto: ConceptoReserva, Monto: r},
		}
		adminAsig = adminAsig.Add(a)
		socialAsig = socialAsig.Add(s)
		reservaAsig = reservaAsig.Add(r)
	}
	return out, ResiduoProrrateo{
		Admin:   admin.Sub(adminAsig),
		Social:  social.Sub(socialAsig),
		Reserva: reserva.Sub(reservaAsig),
	}
}

// NetoDeCorrida es el denominador de [Prorratear]: lo que la corrida dejo
// para repartir.
//
// Vive aqui y no en quien la llama para que la resta este escrita UNA vez. Es
// la misma invariante que `resultados_proceso` comprueba en la base
// (`bruto = admin + social + reserva + neto`), asi que si dos sitios la
// calcularan y uno se desviara, el que se desviara produciria una tasa que la
// corrida nunca aplico.
func NetoDeCorrida(bruto, admin, social, reserva decimal.Decimal) decimal.Decimal {
	return bruto.Sub(admin).Sub(social).Sub(reserva)
}
