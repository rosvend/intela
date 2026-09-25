package liquidacion

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// EvaluarPlazo aplica R-10 y R-11 contra un dia civil.
//
// hoy es YYYY-MM-DD, el mismo formato que [OrdenDePago.EnviadaDia]. Lo
// produce aplicacion a partir de PuertoReloj; este paquete no pregunta
// que hora es.
//
// A los 15 dias calendario sin respuesta:
//
//   - si el neto supera el umbral (2% SMMLV), pasa a aceptada_por_silencio;
//   - si no, pasa a diferida y el monto se arrastra al siguiente periodo.
//
// Antes del dia 15 no se toca. Un estado distinto de enviada tampoco:
// una respuesta del titular ya cerro la ventana.
func (o OrdenDePago) EvaluarPlazo(hoy string, umbral decimal.Decimal) OrdenDePago {
	if o.Estado != EstadoEnviada {
		return o
	}
	dias, ok := diasEntre(o.EnviadaDia, hoy)
	if !ok || dias < PlazoAceptacionDias {
		return o
	}
	if o.Neto.LessThanOrEqual(umbral) {
		o.Estado = EstadoDiferida
		return o
	}
	o.Estado = EstadoAceptadaPorSilencio
	return o
}

// diasEntre cuenta dias civiles de desde a hasta, ambos YYYY-MM-DD.
//
// No usa time: el paquete esta denegado en dominio. La aritmetica es
// Rata Die (Howard Hinnant), enteros, sin zona horaria.
func diasEntre(desde, hasta string) (int, bool) {
	y1, m1, d1, ok := parseFecha(desde)
	if !ok {
		return 0, false
	}
	y2, m2, d2, ok := parseFecha(hasta)
	if !ok {
		return 0, false
	}
	return rataDie(y2, m2, d2) - rataDie(y1, m1, d1), true
}

func parseFecha(s string) (y, m, d int, ok bool) {
	partes := strings.Split(s, "-")
	if len(partes) != 3 {
		return 0, 0, 0, false
	}
	y, err := strconv.Atoi(partes[0])
	if err != nil {
		return 0, 0, 0, false
	}
	m, err = strconv.Atoi(partes[1])
	if err != nil {
		return 0, 0, 0, false
	}
	d, err = strconv.Atoi(partes[2])
	if err != nil {
		return 0, 0, 0, false
	}
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return 0, 0, 0, false
	}
	return y, m, d, true
}

// rataDie es el numero de dia civil desde 0001-01-01, algoritmo de
// Howard Hinnant. Solo enteros: un float aqui seria el mismo error que
// usar time.Time.Sub y dividir por 24h, que no es un dia calendario
// alrededor de un cambio de horario.
func rataDie(y, m, d int) int {
	if m <= 2 {
		y--
		m += 12
	}
	era := y / 400
	yoe := y - era*400
	doy := (153*(m-3)+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 306
}
