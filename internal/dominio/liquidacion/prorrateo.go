package liquidacion

import "github.com/shopspring/decimal"

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

// Prorratear asigna las deducciones de un proceso a una linea de titular.
//
// neto es lo que ya le toca al titular (resultados_titular.importe). El
// resto son los totales del proceso. Si el neto del proceso es cero no hay
// proporcion que aplicar: se devuelve la linea vacia. Liquidacion no
// recalcula el reparto (ADR 0003); solo deshace la resta para mostrarla.
//
// El bruto se reconstruye desde el neto y las deducciones ya redondeadas
// para que la identidad de la linea cierre al centavo.
func Prorratear(neto, adminProc, socialProc, reservaProc, netoProc decimal.Decimal) Linea {
	if netoProc.IsZero() {
		return Linea{}
	}
	admin := neto.Mul(adminProc).Div(netoProc).Round(2)
	social := neto.Mul(socialProc).Div(netoProc).Round(2)
	reserva := neto.Mul(reservaProc).Div(netoProc).Round(2)
	bruto := neto.Add(admin).Add(social).Add(reserva)
	return Linea{
		Bruto:   bruto,
		Admin:   admin,
		Social:  social,
		Reserva: reserva,
		Neto:    neto,
	}
}
