package liquidacion

import (
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestProrratearConservaLaIdentidadDeLaLinea(t *testing.T) {
	// Bolsa de 10_000: 20% admin, 10% social, 5% reserva, neto 6_500.
	// Ana tiene el 60% del neto: 3_900.
	l := Prorratear(dec("3900"), dec("2000"), dec("1000"), dec("500"), dec("6500"))

	if !l.Neto.Equal(dec("3900")) {
		t.Fatalf("neto = %s, se esperaba 3900", l.Neto)
	}
	if !l.Admin.Equal(dec("1200")) {
		t.Fatalf("admin = %s, se esperaba 1200", l.Admin)
	}
	if !l.Social.Equal(dec("600")) {
		t.Fatalf("social = %s, se esperaba 600", l.Social)
	}
	if !l.Reserva.Equal(dec("300")) {
		t.Fatalf("reserva = %s, se esperaba 300", l.Reserva)
	}
	if !l.Bruto.Equal(dec("6000")) {
		t.Fatalf("bruto = %s, se esperaba 6000", l.Bruto)
	}
	if !l.Neto.Equal(l.Bruto.Sub(l.Admin).Sub(l.Social).Sub(l.Reserva)) {
		t.Fatalf("neto %s != bruto - deducciones", l.Neto)
	}
}

func TestProrratearConNetoDeProcesoCeroDevuelveCeros(t *testing.T) {
	l := Prorratear(dec("0"), dec("80"), dec("20"), dec("0"), dec("0"))
	if !l.Bruto.IsZero() || !l.Neto.IsZero() {
		t.Fatalf("sin neto de proceso no hay proporcion: %+v", l)
	}
}

func TestProrratearRedondeoCierraPorLinea(t *testing.T) {
	// 100 / 3 no es exacto en centavos. La identidad de la linea manda.
	l := Prorratear(dec("10.00"), dec("20"), dec("10"), dec("5"), dec("65"))
	suma := l.Neto.Add(l.Admin).Add(l.Social).Add(l.Reserva)
	if !suma.Equal(l.Bruto) {
		t.Fatalf("bruto %s != neto+deducciones %s", l.Bruto, suma)
	}
}
