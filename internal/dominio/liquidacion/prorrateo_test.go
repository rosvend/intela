package liquidacion

import (
	"errors"
	"testing"
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
	ds := Prorratear(dec("325"), dec("200"), dec("100"), dec("50"), dec("650"))
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
	if ds := Prorratear(dec("10"), dec("1"), dec("1"), dec("1"), dec("0")); ds == nil || len(ds) != 0 {
		t.Fatalf("netoProc cero: se esperaba lista vacia no nil, se obtuvo %v", ds)
	}
	if ds := Prorratear(dec("0"), dec("1"), dec("1"), dec("1"), dec("650")); ds == nil || len(ds) != 0 {
		t.Fatalf("titular sin neto: se esperaba lista vacia no nil, se obtuvo %v", ds)
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
