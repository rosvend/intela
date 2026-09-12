package liquidacion

import (
	"errors"
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

func ordenPrueba(t *testing.T, bruto string, deducciones []Deduccion) OrdenDePago {
	t.Helper()
	o, err := NuevaOrden("liq-1", "prc-1", "tit-ana", "2026", "2026-01-01", dec(bruto), deducciones)
	if err != nil {
		t.Fatalf("NuevaOrden: %v", err)
	}
	return o
}

func TestNuevaOrdenSeparaBrutoDeduccionesYNeto(t *testing.T) {
	deducciones := []Deduccion{
		{Concepto: ConceptoAdministracion, Monto: dec("200.00")},
		{Concepto: ConceptoSocial, Monto: dec("100.00")},
		{Concepto: ConceptoReserva, Monto: dec("50.00")},
	}
	o := ordenPrueba(t, "1000.00", deducciones)

	if !o.Bruto.Equal(dec("1000.00")) {
		t.Fatalf("Bruto = %s, se esperaba 1000.00", o.Bruto)
	}
	if len(o.Deducciones) != 3 {
		t.Fatalf("deducciones = %d, se esperaban 3 itemizadas, no un neto colapsado", len(o.Deducciones))
	}
	if o.Deducciones[0].Concepto != ConceptoAdministracion || !o.Deducciones[0].Monto.Equal(dec("200.00")) {
		t.Fatalf("primera deduccion = %+v", o.Deducciones[0])
	}
	if o.Deducciones[1].Concepto != ConceptoSocial || !o.Deducciones[1].Monto.Equal(dec("100.00")) {
		t.Fatalf("segunda deduccion = %+v", o.Deducciones[1])
	}
	if o.Deducciones[2].Concepto != ConceptoReserva || !o.Deducciones[2].Monto.Equal(dec("50.00")) {
		t.Fatalf("tercera deduccion = %+v", o.Deducciones[2])
	}
	if !o.Neto.Equal(dec("650.00")) {
		t.Fatalf("Neto = %s, se esperaba 650.00 (1000-200-100-50)", o.Neto)
	}
	if o.Estado != EstadoEnviada {
		t.Fatalf("Estado = %q, una orden recien emitida esta enviada", o.Estado)
	}
}

func TestNuevaOrdenSinDeduccionesNoDejaElCorteNulo(t *testing.T) {
	o, err := NuevaOrden("liq-1", "prc-1", "tit-ana", "2026", "2026-01-01", dec("100"), nil)
	if err != nil {
		t.Fatalf("NuevaOrden: %v", err)
	}
	if o.Deducciones == nil {
		t.Fatal("Deducciones nil se serializa a null; tiene que ser una lista vacia")
	}
	if !o.Neto.Equal(dec("100")) {
		t.Fatalf("Neto = %s", o.Neto)
	}
}

func TestNuevaOrdenRechazaBrutoNegativo(t *testing.T) {
	_, err := NuevaOrden("liq-1", "prc-1", "tit-ana", "2026", "2026-01-01", dec("-1"), nil)
	if !errors.Is(err, ErrBrutoNegativo) {
		t.Fatalf("se esperaba ErrBrutoNegativo, se obtuvo %v", err)
	}
}

func TestNuevaOrdenRechazaDeduccionNegativa(t *testing.T) {
	_, err := NuevaOrden("liq-1", "prc-1", "tit-ana", "2026", "2026-01-01",
		dec("100"), []Deduccion{{Concepto: ConceptoAdministracion, Monto: dec("-1")}})
	if !errors.Is(err, ErrDeduccionNegativa) {
		t.Fatalf("se esperaba ErrDeduccionNegativa, se obtuvo %v", err)
	}
}

func TestNuevaOrdenRechazaNetoNegativo(t *testing.T) {
	_, err := NuevaOrden("liq-1", "prc-1", "tit-ana", "2026", "2026-01-01",
		dec("100"), []Deduccion{{Concepto: ConceptoAdministracion, Monto: dec("200")}})
	if !errors.Is(err, ErrNetoNegativo) {
		t.Fatalf("se esperaba ErrNetoNegativo, se obtuvo %v", err)
	}
}

func TestUmbralMenorCuantiaEsElDosPorCiento(t *testing.T) {
	// SMMLV 1_300_000 -> 2% = 26_000. Exacto, sin float.
	got := UmbralMenorCuantia(dec("1300000"))
	if !got.Equal(dec("26000")) {
		t.Fatalf("umbral = %s, se esperaba 26000", got)
	}
}

func TestEsPagableExigeAceptacionYDocumentos(t *testing.T) {
	completos := Documentos{RUT: true, CertificacionBancaria: true}
	incompletos := Documentos{RUT: true, CertificacionBancaria: false}

	casos := []struct {
		name    string
		estado  Estado
		docs    Documentos
		neto    string
		pagable bool
	}{
		{name: "aceptada con RUT y banco", estado: EstadoAceptada, docs: completos, neto: "100", pagable: true},
		{name: "aceptada por silencio con RUT y banco", estado: EstadoAceptadaPorSilencio, docs: completos, neto: "100", pagable: true},
		{name: "aceptada sin certificacion bancaria", estado: EstadoAceptada, docs: incompletos, neto: "100", pagable: false},
		{name: "enviada con documentos", estado: EstadoEnviada, docs: completos, neto: "100", pagable: false},
		{name: "diferida con documentos", estado: EstadoDiferida, docs: completos, neto: "100", pagable: false},
		{name: "aceptada con neto cero", estado: EstadoAceptada, docs: completos, neto: "0", pagable: false},
	}
	for _, tt := range casos {
		t.Run(tt.name, func(t *testing.T) {
			o := ordenPrueba(t, tt.neto, nil)
			o.Estado = tt.estado
			if got := o.EsPagable(tt.docs); got != tt.pagable {
				t.Fatalf("EsPagable = %v, se esperaba %v", got, tt.pagable)
			}
		})
	}
}

func TestDocumentosCompletosPideLosDosDeR12(t *testing.T) {
	if (Documentos{}).Completos() {
		t.Fatal("sin nada no estan completos")
	}
	if (Documentos{RUT: true}).Completos() {
		t.Fatal("solo RUT no basta: RD 13.1.6 pide tambien la certificacion bancaria")
	}
	if (Documentos{CertificacionBancaria: true}).Completos() {
		t.Fatal("solo la certificacion bancaria no basta")
	}
	if !(Documentos{RUT: true, CertificacionBancaria: true}).Completos() {
		t.Fatal("RUT y certificacion bancaria tienen que bastar")
	}
}

func TestRegistrarRespuesta(t *testing.T) {
	o := ordenPrueba(t, "100", nil)

	pagar := o.RegistrarRespuesta(true)
	if pagar.Estado != EstadoAceptada {
		t.Fatalf("quiere pago: Estado = %q", pagar.Estado)
	}

	objetar := o.RegistrarRespuesta(false)
	if objetar.Estado != EstadoObjetada {
		t.Fatalf("objeta: Estado = %q", objetar.Estado)
	}

	// Ya aceptada: una segunda respuesta no la mueve.
	otra := pagar.RegistrarRespuesta(false)
	if otra.Estado != EstadoAceptada {
		t.Fatalf("una orden ya aceptada no se objeta: %q", otra.Estado)
	}
}

func TestIncorporarArrastreSumaElNetoSinRededucir(t *testing.T) {
	actual := ordenPrueba(t, "1000.00", []Deduccion{
		{Concepto: ConceptoAdministracion, Monto: dec("200.00")},
	})
	anterior, err := NuevaOrden("liq-prev", "prc-0", "tit-ana", "2025", "2025-01-01",
		dec("80.00"), nil)
	if err != nil {
		t.Fatalf("NuevaOrden: %v", err)
	}
	anterior.Estado = EstadoDiferida

	got := actual.IncorporarArrastre(anterior)
	if !got.Bruto.Equal(dec("1080.00")) {
		t.Fatalf("Bruto = %s, se esperaba 1080 (1000+80, sin volver a deducir)", got.Bruto)
	}
	if !got.Neto.Equal(dec("880.00")) {
		t.Fatalf("Neto = %s, se esperaba 880 (800+80)", got.Neto)
	}
	if len(got.Arrastres) != 1 || got.Arrastres[0] != "liq-prev" {
		t.Fatalf("Arrastres = %v", got.Arrastres)
	}
	if !got.Deducciones[0].Monto.Equal(dec("200.00")) {
		t.Fatal("las deducciones del periodo actual no se tocan")
	}

	// Una enviada no se arrastra: solo las diferidas.
	viva := ordenPrueba(t, "50", nil)
	igual := actual.IncorporarArrastre(viva)
	if !igual.Neto.Equal(actual.Neto) {
		t.Fatal("arrastrar una enviada no debe cambiar el neto")
	}
}

func TestMarcarAcumuladaSoloCierraUnaDiferida(t *testing.T) {
	o := ordenPrueba(t, "10", nil)
	o.Estado = EstadoDiferida
	if o.MarcarAcumulada().Estado != EstadoAcumulada {
		t.Fatal("una diferida incorporada pasa a acumulada")
	}
	enviada := ordenPrueba(t, "10", nil)
	if enviada.MarcarAcumulada().Estado != EstadoEnviada {
		t.Fatal("marcar acumulada no puede cerrar una enviada")
	}
}
