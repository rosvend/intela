package liquidacion

import "github.com/shopspring/decimal"

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
// Los tres conceptos que RD 13.2 obliga a itemizar, en orden fijo. Una lista
// VACIA -- nunca nil -- cuando no hay nada que prorratear: netoProc en cero o
// negativo (una corrida que no dejo nada) o un titular con neto cero. Redondea
// a dos decimales, que es la escala de `ordenes_pago`; el residuo de redondeo
// se queda en la corrida y no se reparte, igual que en reparto/redondeo.go.
func Prorratear(netoTitular, admin, social, reserva, netoProc decimal.Decimal) []Deduccion {
	if netoProc.LessThanOrEqual(decimal.Zero) || netoTitular.IsZero() {
		return []Deduccion{}
	}
	prop := netoTitular.Div(netoProc)
	return []Deduccion{
		{Concepto: ConceptoAdministracion, Monto: admin.Mul(prop).Round(2)},
		{Concepto: ConceptoSocial, Monto: social.Mul(prop).Round(2)},
		{Concepto: ConceptoReserva, Monto: reserva.Mul(prop).Round(2)},
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
