package anticipos

import "github.com/shopspring/decimal"

// Los dos topes de R-31. Constantes y no parametros normativos: son texto del
// Reglamento de Anticipos V7, no una cifra que cambie por ano. Lo que si cambia
// por ano es el SMMLV, y ese entra como dato (ADR 0004).
const (
	// PorcentajeDerechos es RA 3.1.g: "no podra superar el 25% de los Derechos
	// de Autor generados a su favor en los procesos de reparto ordinarios de los
	// ultimos dos anos".
	PorcentajeDerechos = 25

	// SalariosMinimos es RA 3.1.h: "no podra ser superior a los cinco (5)
	// salarios minimos mensuales legales vigentes".
	SalariosMinimos = 5

	// VentanaAnios es la ventana de RA 3.1.g: "los ultimos dos anos". Como se
	// cuenta (por la fecha de la orden de pago) lo fija el ADR 0027.
	VentanaAnios = 2
)

// Tope es el cupo de R-31: el MENOR de los dos topes simultaneos.
//
// derechos es lo que el socio recibio en los repartos ordinarios de la ventana
// (ADR 0027 dice de donde sale); smmlv, el vigente en la fecha de la decision.
//
// Los dos topes se truncan al centavo (RoundDown) y no se redondean a la
// mitad: el reglamento los da como techo ("no podra superar", "no podra ser
// superior"), y redondear hacia arriba dejaria otorgar un centavo por encima.
// El de 5 SMMLV solo tiene centavos que truncar si el SMMLV trae mas de dos
// decimales (`parametros.valor` es NUMERIC(18,6)). Unos derechos negativos -no
// los produce ninguna linea de reparto, cuyo importe es >= 0- no dan un cupo
// negativo.
func Tope(derechos, smmlv decimal.Decimal) decimal.Decimal {
	if derechos.IsNegative() {
		derechos = decimal.Zero
	}
	porDerechos := derechos.
		Mul(decimal.NewFromInt(PorcentajeDerechos)).
		Div(decimal.NewFromInt(100)).
		RoundDown(2)
	porSalarios := smmlv.Mul(decimal.NewFromInt(SalariosMinimos)).RoundDown(2)
	return decimal.Min(porDerechos, porSalarios)
}
