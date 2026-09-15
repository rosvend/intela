package liquidacion

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestEvaluarPlazoSilencioDia14VsDia15(t *testing.T) {
	// Enviada el 1 de enero. El dia 14 (15 de enero) sigue enviada; el
	// dia 15 (16 de enero) se acepta por silencio. El umbral va por
	// debajo del neto para que cuente R-10 y no R-11.
	o := ordenPrueba(t, "100000", nil)
	umbral := dec("26000")

	casos := []struct {
		hoy    string
		estado Estado
	}{
		{hoy: "2026-01-01", estado: EstadoEnviada},
		{hoy: "2026-01-14", estado: EstadoEnviada},
		{hoy: "2026-01-15", estado: EstadoEnviada},
		{hoy: "2026-01-16", estado: EstadoAceptadaPorSilencio},
		{hoy: "2026-01-31", estado: EstadoAceptadaPorSilencio},
	}
	for _, tt := range casos {
		t.Run(tt.hoy, func(t *testing.T) {
			got := o.EvaluarPlazo(tt.hoy, umbral)
			if got.Estado != tt.estado {
				t.Fatalf("el %s: Estado = %q, se esperaba %q", tt.hoy, got.Estado, tt.estado)
			}
		})
	}
}

func TestEvaluarPlazoMenorCuantiaDifiereEnVezDeAceptar(t *testing.T) {
	// Neto 1000, umbral 26000: R-11 manda. Sin respuesta a los 15 dias
	// se difiere; no se acepta por silencio.
	o := ordenPrueba(t, "1000", nil)
	umbral := UmbralMenorCuantia(dec("1300000"))

	alDia14 := o.EvaluarPlazo("2026-01-15", umbral)
	if alDia14.Estado != EstadoEnviada {
		t.Fatalf("dia 14: Estado = %q, todavia no vence", alDia14.Estado)
	}

	alDia15 := o.EvaluarPlazo("2026-01-16", umbral)
	if alDia15.Estado != EstadoDiferida {
		t.Fatalf("dia 15 bajo umbral: Estado = %q, se esperaba diferida (R-11)", alDia15.Estado)
	}
}

func TestEvaluarPlazoNoTocaUnaOrdenYaRespondida(t *testing.T) {
	o := ordenPrueba(t, "100000", nil).RegistrarRespuesta(true)
	got := o.EvaluarPlazo("2026-01-16", dec("1"))
	if got.Estado != EstadoAceptada {
		t.Fatalf("una aceptada no pasa a silencio: %q", got.Estado)
	}
}

func TestEvaluarPlazoNetoIgualAlUmbralDifiere(t *testing.T) {
	// RD 13.3: "igual o menor" al 2%. El borde es diferida, no silencio.
	umbral := UmbralMenorCuantia(dec("1300000"))
	o := ordenPrueba(t, "26000", nil)
	got := o.EvaluarPlazo("2026-01-16", umbral)
	if got.Estado != EstadoDiferida {
		t.Fatalf("neto = umbral: Estado = %q, se esperaba diferida", got.Estado)
	}
}

func TestAcumulacionR11EnDosPeriodos(t *testing.T) {
	smmlv := dec("1300000")
	umbral := UmbralMenorCuantia(smmlv)

	// Periodo 1: 1000, bajo umbral, sin respuesta -> diferida.
	p1 := ordenPrueba(t, "1000", nil)
	p1.ID = "liq-p1"
	p1 = p1.EvaluarPlazo("2026-01-16", umbral)
	if p1.Estado != EstadoDiferida {
		t.Fatalf("periodo 1: Estado = %q", p1.Estado)
	}

	// Periodo 2: 20000 propios + 1000 arrastrados = 21000, sigue bajo
	// umbral. Se informa, y a los 15 dias se vuelve a diferir.
	p2, err := NuevaOrden("liq-p2", "prc-2", "tit-ana", "2026-2", "2026-07-01",
		dec("20000"), nil)
	if err != nil {
		t.Fatalf("NuevaOrden p2: %v", err)
	}
	p2 = p2.IncorporarArrastre(p1)
	p1 = p1.MarcarAcumulada()
	if p1.Estado != EstadoAcumulada {
		t.Fatal("el periodo 1 tiene que quedar acumulado para no arrastrarse dos veces")
	}
	if !p2.Neto.Equal(dec("21000")) {
		t.Fatalf("periodo 2 neto = %s, se esperaba 21000", p2.Neto)
	}
	p2 = p2.EvaluarPlazo("2026-07-16", umbral)
	if p2.Estado != EstadoDiferida {
		t.Fatalf("21000 sigue bajo 26000: Estado = %q, se esperaba diferida", p2.Estado)
	}

	// Periodo 3: 6000 propios + 21000 arrastrados = 27000, supera el
	// umbral. Sin respuesta, se acepta por silencio y se puede pagar.
	p3, err := NuevaOrden("liq-p3", "prc-3", "tit-ana", "2026-3", "2027-01-01",
		dec("6000"), nil)
	if err != nil {
		t.Fatalf("NuevaOrden p3: %v", err)
	}
	p3 = p3.IncorporarArrastre(p2)
	p2 = p2.MarcarAcumulada()
	if !p3.Neto.Equal(dec("27000")) {
		t.Fatalf("periodo 3 neto = %s, se esperaba 27000", p3.Neto)
	}

	antes := p3.EvaluarPlazo("2027-01-15", umbral)
	if antes.Estado != EstadoEnviada {
		t.Fatalf("dia 14 del periodo 3: %q", antes.Estado)
	}
	p3 = p3.EvaluarPlazo("2027-01-16", umbral)
	if p3.Estado != EstadoAceptadaPorSilencio {
		t.Fatalf("27000 > 26000 a los 15 dias: Estado = %q, se esperaba aceptada_por_silencio", p3.Estado)
	}
	if !p3.EsPagable(Documentos{RUT: true, CertificacionBancaria: true}) {
		t.Fatal("superado el umbral, aceptada por silencio y con documentos: tiene que ser pagable")
	}
	if p3.EsPagable(Documentos{RUT: true}) {
		t.Fatal("sin certificacion bancaria no es pagable aunque supere el umbral (R-12)")
	}
}

func TestDiasEntreCruzaMesYBisiesto(t *testing.T) {
	casos := []struct {
		desde, hasta string
		dias         int
	}{
		{desde: "2026-01-01", hasta: "2026-01-01", dias: 0},
		{desde: "2026-01-01", hasta: "2026-01-16", dias: 15},
		{desde: "2026-01-31", hasta: "2026-02-15", dias: 15},
		{desde: "2024-02-28", hasta: "2024-03-14", dias: 15}, // 2024 es bisiesto
		{desde: "2025-02-28", hasta: "2025-03-15", dias: 15}, // 2025 no
	}
	for _, tt := range casos {
		got, ok := diasEntre(tt.desde, tt.hasta)
		if !ok {
			t.Fatalf("diasEntre(%s, %s) no parseo", tt.desde, tt.hasta)
		}
		if got != tt.dias {
			t.Fatalf("diasEntre(%s, %s) = %d, se esperaba %d", tt.desde, tt.hasta, got, tt.dias)
		}
	}
}

func TestEvaluarPlazoFechaInvalidaNoTransiciona(t *testing.T) {
	o := ordenPrueba(t, "100000", nil)
	got := o.EvaluarPlazo("no-es-una-fecha", decimal.Zero)
	if got.Estado != EstadoEnviada {
		t.Fatalf("una fecha basura no puede aceptar por silencio: %q", got.Estado)
	}
}
