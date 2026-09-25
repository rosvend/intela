package liquidacion

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestProrratearConservaLaIdentidadDeLaLinea(t *testing.T) {
	// Bolsa de 10_000: 20% admin, 10% social, 5% reserva, neto 6_500.
	// Ana tiene el 60% del neto: 3_900.
	l := ProrratearLinea(dec("3900"), dec("2000"), dec("1000"), dec("500"), dec("6500"))

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
	l := ProrratearLinea(dec("0"), dec("80"), dec("20"), dec("0"), dec("0"))
	if !l.Bruto.IsZero() || !l.Neto.IsZero() {
		t.Fatalf("sin neto de proceso no hay proporcion: %+v", l)
	}
}

func TestProrratearRedondeoCierraPorLinea(t *testing.T) {
	// 100 / 3 no es exacto en centavos. La identidad de la linea manda.
	l := ProrratearLinea(dec("10.00"), dec("20"), dec("10"), dec("5"), dec("65"))
	suma := l.Neto.Add(l.Admin).Add(l.Social).Add(l.Reserva)
	if !suma.Equal(l.Bruto) {
		t.Fatalf("bruto %s != neto+deducciones %s", l.Bruto, suma)
	}
}

func TestProrratearProcesoCuadraContraTotalesDelProceso(t *testing.T) {
	// Reproduccion del hallazgo de revision: 7 titulares con participacion
	// no uniforme sobre neto 10_000. Redondear cada uno por separado deja
	// Σ Admin = 1999.98; el mayor-resto tiene que devolver 2000.00.
	netos := []decimal.Decimal{
		dec("2500.01"),
		dec("1800.00"),
		dec("1500.00"),
		dec("1200.00"),
		dec("999.99"),
		dec("1000.00"),
		dec("1000.00"),
	}
	sumaNetos := decimal.Zero
	for _, n := range netos {
		sumaNetos = sumaNetos.Add(n)
	}
	if !sumaNetos.Equal(dec("10000")) {
		t.Fatalf("fixture: suma netos = %s", sumaNetos)
	}

	admin, social, reserva := dec("2000"), dec("1000"), dec("500")
	lineas := ProrratearProceso(netos, admin, social, reserva)
	if len(lineas) != len(netos) {
		t.Fatalf("lineas = %d", len(lineas))
	}

	var sumaAdmin, sumaSocial, sumaReserva decimal.Decimal
	for i, l := range lineas {
		if !l.Neto.Equal(netos[i]) {
			t.Fatalf("linea[%d].neto = %s", i, l.Neto)
		}
		if !l.Neto.Equal(l.Bruto.Sub(l.Admin).Sub(l.Social).Sub(l.Reserva)) {
			t.Fatalf("linea[%d]: identidad rota bruto=%s neto=%s", i, l.Bruto, l.Neto)
		}
		sumaAdmin = sumaAdmin.Add(l.Admin)
		sumaSocial = sumaSocial.Add(l.Social)
		sumaReserva = sumaReserva.Add(l.Reserva)
	}
	if !sumaAdmin.Equal(admin) {
		t.Fatalf("Σ admin = %s, se esperaba %s (RD 16 / ADR 0005)", sumaAdmin, admin)
	}
	if !sumaSocial.Equal(social) {
		t.Fatalf("Σ social = %s, se esperaba %s", sumaSocial, social)
	}
	if !sumaReserva.Equal(reserva) {
		t.Fatalf("Σ reserva = %s, se esperaba %s", sumaReserva, reserva)
	}
}

func TestProrratearProcesoEsDeterminista(t *testing.T) {
	netos := []decimal.Decimal{dec("3333.34"), dec("3333.33"), dec("3333.33")}
	a := ProrratearProceso(netos, dec("100"), dec("50"), dec("25"))
	b := ProrratearProceso(netos, dec("100"), dec("50"), dec("25"))
	for i := range a {
		if !a[i].Admin.Equal(b[i].Admin) || !a[i].Social.Equal(b[i].Social) || !a[i].Reserva.Equal(b[i].Reserva) {
			t.Fatalf("no determinista en [%d]: %+v vs %+v", i, a[i], b[i])
		}
	}
}
