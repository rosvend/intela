package liquidacion

import (
	"sort"

	"github.com/shopspring/decimal"
)

// Linea es la participacion de un titular en una obra, con las deducciones
// del proceso ya prorrateadas sobre su neto.
//
// La identidad que hay que conservar, la misma que exige resultados_proceso:
//
//	Neto == Bruto - Admin - Social - Reserva
//
// Admin es R-06 (gastos administrativos), Social es R-06 (bienestar social)
// y Reserva es R-07 (errores tecnicos). El motor de reparto las descuenta
// de la bolsa ANTES de partir por obra; aqui solo se reparte esa resta
// sobre la linea del titular, para que el reporte pueda mostrar bruto,
// cada deduccion y neto por obra sin recalcular la corrida.
type Linea struct {
	Bruto   decimal.Decimal
	Admin   decimal.Decimal
	Social  decimal.Decimal
	Reserva decimal.Decimal
	Neto    decimal.Decimal
}

var centavo = decimal.RequireFromString("0.01")

// ProrratearLinea asigna las deducciones de un proceso a UNA linea de titular.
//
// Cuando solo se conoce esta linea, el remanente del neto del proceso
// (netoProc - neto) entra como cubeta residual para que el mayor-resto
// coincida con lo que [ProrratearProceso] asignaria si el resto fuera una
// sola partida. Para reconciliar Σ Admin = admin del proceso entre TODOS
// los titulares hay que llamar a [ProrratearProceso] con sus netos.
//
// El bruto se reconstruye desde el neto y las deducciones ya redondeadas
// para que la identidad de la linea cierre al centavo.
func ProrratearLinea(neto, adminProc, socialProc, reservaProc, netoProc decimal.Decimal) Linea {
	if netoProc.IsZero() {
		return Linea{}
	}
	partes := []decimal.Decimal{neto}
	if resto := netoProc.Sub(neto); resto.IsPositive() {
		partes = append(partes, resto)
	}
	return ProrratearProceso(partes, adminProc, socialProc, reservaProc)[0]
}

// ProrratearProceso reparte admin/social/reserva del proceso entre todas las
// lineas (netos) del mismo. La suma de cada concepto cuadra con el total
// del proceso al centavo: el residuo de redondeo se asigna por mayor resto
// fraccionario (Hamilton), con desempate por indice estable — no se pierde
// en silencio ni se absorbe en "la ultima linea" (ADR 0005, RD 16).
//
// Cada Linea conserva Neto == Bruto - Admin - Social - Reserva.
func ProrratearProceso(netos []decimal.Decimal, admin, social, reserva decimal.Decimal) []Linea {
	if len(netos) == 0 {
		return nil
	}
	admins := repartirExacto(admin, netos)
	sociales := repartirExacto(social, netos)
	reservas := repartirExacto(reserva, netos)
	out := make([]Linea, len(netos))
	for i, neto := range netos {
		out[i] = Linea{
			Neto:    neto,
			Admin:   admins[i],
			Social:  sociales[i],
			Reserva: reservas[i],
			Bruto:   neto.Add(admins[i]).Add(sociales[i]).Add(reservas[i]),
		}
	}
	return out
}

// repartirExacto asigna total entre pesos con mayor resto, de modo que
// suma(resultado) == total. Pesos no positivos reciben cero.
func repartirExacto(total decimal.Decimal, pesos []decimal.Decimal) []decimal.Decimal {
	out := make([]decimal.Decimal, len(pesos))
	sumaPesos := decimal.Zero
	for _, p := range pesos {
		if p.IsPositive() {
			sumaPesos = sumaPesos.Add(p)
		}
	}
	if total.IsZero() || sumaPesos.IsZero() {
		return out
	}

	type candidato struct {
		i    int
		frac decimal.Decimal
	}
	candidatos := make([]candidato, 0, len(pesos))
	asignado := decimal.Zero
	for i, p := range pesos {
		if !p.IsPositive() {
			continue
		}
		exacto := total.Mul(p).Div(sumaPesos)
		base := exacto.RoundDown(2)
		out[i] = base
		asignado = asignado.Add(base)
		candidatos = append(candidatos, candidato{i: i, frac: exacto.Sub(base)})
	}

	sort.SliceStable(candidatos, func(a, b int) bool {
		cmp := candidatos[a].frac.Cmp(candidatos[b].frac)
		if cmp != 0 {
			return cmp > 0
		}
		return candidatos[a].i < candidatos[b].i
	})

	falta := total.Sub(asignado)
	for _, c := range candidatos {
		if !falta.IsPositive() {
			break
		}
		out[c.i] = out[c.i].Add(centavo)
		falta = falta.Sub(centavo)
	}
	return out
}
