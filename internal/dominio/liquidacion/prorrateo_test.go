package liquidacion

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestNetoDeCorridaEsBrutoMenosLasTresDeducciones(t *testing.T) {
	got := NetoDeCorrida(dec("1000"), dec("200"), dec("100"), dec("50"))
	if !got.Equal(dec("650")) {
		t.Fatalf("netoProc = %s, se esperaba 650", got)
	}
}

func TestProrratearUsaElNetoDeLaCorridaYNoLaSumaDeLineas(t *testing.T) {
	// Corrida: bruto 1000, deducciones 350, neto 650. De ese neto solo se
	// distribuyen 325 (la otra mitad queda retenida: declaracion incompleta,
	// R-04). Ana se lleva los 325.
	//
	// Con el neto de la corrida como denominador, Ana carga la MITAD de las
	// deducciones: 175. Su tasa -- 175/500 -- es el 35%, la misma que aplico
	// la corrida (350/1000). Con la suma de las lineas como denominador,
	// Ana cargaria las 350 enteras y su tasa seria del 51,85%: una cifra que
	// la corrida nunca aplico.
	porTitular, residuo := Prorratear(
		map[string]decimal.Decimal{"tit-ana": dec("325")},
		dec("200"), dec("100"), dec("50"), dec("650"),
	)
	ds := porTitular["tit-ana"]
	if len(ds) != 3 {
		t.Fatalf("%d deducciones, se esperaban 3 itemizadas (RD 13.2)", len(ds))
	}
	if !ds[0].Monto.Equal(dec("100")) {
		t.Fatalf("admin = %s, se esperaba 100 (200 * 325/650)", ds[0].Monto)
	}
	if !ds[1].Monto.Equal(dec("50")) {
		t.Fatalf("social = %s, se esperaba 50", ds[1].Monto)
	}
	if !ds[2].Monto.Equal(dec("25")) {
		t.Fatalf("reserva = %s, se esperaba 25", ds[2].Monto)
	}
	// La mitad retenida de las deducciones queda en el residuo, no desaparece.
	if !residuo.Admin.Equal(dec("100")) || !residuo.Social.Equal(dec("50")) || !residuo.Reserva.Equal(dec("25")) {
		t.Fatalf("residuo = %+v, se esperaba la mitad retenida (100, 50, 25)", residuo)
	}

	o, err := NuevaOrden(DatosOrden{
		ID: "liq-1", ProcesoID: "prc-1", TitularID: "tit-ana",
		Periodo: "2026", Circuito: "nacional", EnviadaDia: "2026-01-01",
		Bruto:       dec("325").Add(dec("175")),
		Deducciones: ds,
	})
	if err != nil {
		t.Fatalf("NuevaOrden: %v", err)
	}
	if !o.Bruto.Equal(dec("500")) || !o.Neto.Equal(dec("325")) {
		t.Fatalf("bruto = %s, neto = %s; se esperaban 500 y 325", o.Bruto, o.Neto)
	}
	// La tasa que se lee en la orden es la de la corrida.
	tasaOrden := dec("175").Div(o.Bruto)
	tasaCorrida := dec("350").Div(dec("1000"))
	if !tasaOrden.Equal(tasaCorrida) {
		t.Fatalf("tasa de la orden = %s, la de la corrida = %s", tasaOrden, tasaCorrida)
	}
}

func TestProrratearNoDevuelveNilNiDivideEntreCero(t *testing.T) {
	porTitular, residuo := Prorratear(
		map[string]decimal.Decimal{"t": dec("10")},
		dec("1"), dec("1"), dec("1"), dec("0"),
	)
	if ds := porTitular["t"]; ds == nil || len(ds) != 0 {
		t.Fatalf("netoProc cero: se esperaba lista vacia no nil, se obtuvo %v", ds)
	}
	if !residuo.Total().Equal(dec("3")) {
		t.Fatalf("con netoProc cero el residuo debe ser el total, got %s", residuo.Total())
	}
	porTitular, residuo = Prorratear(
		map[string]decimal.Decimal{"t": dec("0")},
		dec("1"), dec("1"), dec("1"), dec("650"),
	)
	if ds := porTitular["t"]; ds == nil || len(ds) != 0 {
		t.Fatalf("titular sin neto: se esperaba lista vacia no nil, se obtuvo %v", ds)
	}
	if !residuo.Total().Equal(dec("3")) {
		t.Fatalf("sin netos asignados el residuo debe ser el total, got %s", residuo.Total())
	}
}

// TestProrratearResiduoExplicitoConSieteTitulares es el caso de la revision:
// bruto 1000 repartido entre 7 titulares deja centavos que antes desaparecian.
// Con el residuo explicito, suma(brutos reconstruidos) + residuo = bruto.
// Los montos por titular y el residuo se fijan EXPLICITOS: asignado+residuo
// == concepto es tautologico si solo se comprueba esa identidad.
func TestProrratearResiduoExplicitoConSieteTitulares(t *testing.T) {
	bruto := dec("1000.00")
	admin, social, reserva := dec("200.00"), dec("100.00"), dec("50.00")
	netoProc := NetoDeCorrida(bruto, admin, social, reserva) // 650

	netos := map[string]decimal.Decimal{
		"t1": dec("92.86"),
		"t2": dec("92.86"),
		"t3": dec("92.86"),
		"t4": dec("92.86"),
		"t5": dec("92.86"),
		"t6": dec("92.86"),
		"t7": dec("92.84"), // 650 - 6*92.86
	}
	sumaNetos := dec("0")
	for _, n := range netos {
		sumaNetos = sumaNetos.Add(n)
	}
	if !sumaNetos.Equal(netoProc) {
		t.Fatalf("fixture: suma netos = %s, netoProc = %s", sumaNetos, netoProc)
	}

	porTitular, residuo := Prorratear(netos, admin, social, reserva, netoProc)

	// Montos esperados con Round(2). t1..t6 identicos; t7 difiere en social.
	esperados := map[string][3]string{
		"t1": {"28.57", "14.29", "7.14"},
		"t2": {"28.57", "14.29", "7.14"},
		"t3": {"28.57", "14.29", "7.14"},
		"t4": {"28.57", "14.29", "7.14"},
		"t5": {"28.57", "14.29", "7.14"},
		"t6": {"28.57", "14.29", "7.14"},
		"t7": {"28.57", "14.28", "7.14"},
	}
	for id, want := range esperados {
		ds := porTitular[id]
		if len(ds) != 3 {
			t.Fatalf("%s: %d deducciones", id, len(ds))
		}
		if !ds[0].Monto.Equal(dec(want[0])) || !ds[1].Monto.Equal(dec(want[1])) || !ds[2].Monto.Equal(dec(want[2])) {
			t.Fatalf("%s: admin/social/reserva = %s/%s/%s, se esperaban %v",
				id, ds[0].Monto, ds[1].Monto, ds[2].Monto, want)
		}
	}
	if !residuo.Admin.Equal(dec("0.01")) || !residuo.Social.Equal(dec("-0.02")) || !residuo.Reserva.Equal(dec("0.02")) {
		t.Fatalf("residuo = %+v, se esperaba admin=0.01 social=-0.02 reserva=0.02", residuo)
	}
	if !residuo.Total().Equal(dec("0.01")) {
		t.Fatalf("residuo.Total = %s, se esperaba 0.01", residuo.Total())
	}

	sumaBrutos := dec("0")
	for id, ds := range porTitular {
		brutoTit := netos[id]
		for _, d := range ds {
			brutoTit = brutoTit.Add(d.Monto)
		}
		sumaBrutos = sumaBrutos.Add(brutoTit)
	}
	if !sumaBrutos.Add(residuo.Total()).Equal(bruto) {
		t.Fatalf("suma(brutos)=%s + residuo=%s = %s != bruto %s",
			sumaBrutos, residuo.Total(), sumaBrutos.Add(residuo.Total()), bruto)
	}
}

// TestProrratearResiduoPuedeSerNegativo fija que Round(2) puede pasarse del
// concepto: 0.02 entre tres partes iguales asigna 0.01*3=0.03 y el residuo
// queda en -0.01. No se rectifica con Abs.
func TestProrratearResiduoPuedeSerNegativo(t *testing.T) {
	porTitular, residuo := Prorratear(
		map[string]decimal.Decimal{"a": dec("1"), "b": dec("1"), "c": dec("1")},
		dec("0.02"), dec("0"), dec("0"), dec("3"),
	)
	for _, id := range []string{"a", "b", "c"} {
		if !porTitular[id][0].Monto.Equal(dec("0.01")) {
			t.Fatalf("%s admin = %s, se esperaba 0.01", id, porTitular[id][0].Monto)
		}
	}
	if !residuo.Admin.Equal(dec("-0.01")) {
		t.Fatalf("residuo.Admin = %s, se esperaba -0.01 (no Abs)", residuo.Admin)
	}
	if residuo.Admin.IsPositive() || residuo.Admin.IsZero() {
		t.Fatal("el residuo negativo es el comportamiento documentado de Round(2)")
	}
}

func TestNuevaOrdenExigeCircuito(t *testing.T) {
	_, err := NuevaOrden(DatosOrden{
		ID: "liq-1", ProcesoID: "prc-1", TitularID: "tit-ana",
		Periodo: "2026", EnviadaDia: "2026-01-01", Bruto: dec("100"),
	})
	if err == nil {
		t.Fatal("una orden sin circuito no se puede colocar en (titular, periodo, circuito)")
	}
	if !errors.Is(err, ErrCircuitoAusente) {
		t.Fatalf("se esperaba ErrCircuitoAusente, se obtuvo %v", err)
	}
}

func TestNuevaOrdenDejaLaListaDeProcesosConLaCorridaDeReferencia(t *testing.T) {
	o, err := NuevaOrden(DatosOrden{
		ID: "liq-1", ProcesoID: "prc-1", TitularID: "tit-ana",
		Periodo: "2026", Circuito: "nacional", EnviadaDia: "2026-01-01",
		Bruto: dec("100"),
	})
	if err != nil {
		t.Fatalf("NuevaOrden: %v", err)
	}
	if len(o.Procesos) != 1 || o.Procesos[0] != "prc-1" {
		t.Fatalf("Procesos = %v, se esperaba la lista de uno", o.Procesos)
	}
}
