package reparto_test

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestDistribuirSobreProporcionesRepartePorImporteOriginalConResiduoExplicito(t *testing.T) {
	original := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: d("100"), Importe: d("100.00")},
		{ObraID: "obra-2", TitularID: "titular-b", IPI: "222", Porcentaje: d("100"), Importe: d("100.00")},
		{ObraID: "obra-3", TitularID: "titular-c", IPI: "333", Porcentaje: d("100"), Importe: d("100.00")},
	}

	nuevas, residuo, err := reparto.DistribuirSobreProporciones(d("100.00"), original)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if len(nuevas) != 3 {
		t.Fatalf("se esperaban 3 lineas, hubo %d", len(nuevas))
	}

	suma := decimal.Zero
	for i, n := range nuevas {
		if n.ObraID != original[i].ObraID || n.TitularID != original[i].TitularID {
			t.Fatalf("la linea %d cambio de identidad: %+v", i, n)
		}
		if !n.Importe.Equal(d("33.33")) {
			t.Fatalf("linea %d: importe = %s, se esperaba 33.33", i, n.Importe)
		}
		suma = suma.Add(n.Importe)
	}

	if !residuo.Equal(d("0.01")) {
		t.Fatalf("residuo = %s, se esperaba 0.01", residuo)
	}
	if !suma.Add(residuo).Equal(d("100.00")) {
		t.Fatalf("suma(importes) + residuo = %s, no cierra contra el monto", suma.Add(residuo))
	}
}

func TestDistribuirSobreProporcionesNoRevaloriza(t *testing.T) {
	original := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: d("30"), Importe: d("300.00")},
		{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: d("70"), Importe: d("700.00")},
	}

	nuevas, residuo, err := reparto.DistribuirSobreProporciones(d("1000.00"), original)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !nuevas[0].Importe.Equal(d("300.00")) || !nuevas[1].Importe.Equal(d("700.00")) {
		t.Fatalf("importes = %s, %s; se esperaba la misma proporcion 30/70 aplicada al nuevo monto",
			nuevas[0].Importe, nuevas[1].Importe)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero cuando la proporcion cierra exacto", residuo)
	}
	// El porcentaje declarado original queda intacto: esto es un replay, no un
	// nuevo calculo de participacion.
	if !nuevas[0].Porcentaje.Equal(d("30")) || !nuevas[1].Porcentaje.Equal(d("70")) {
		t.Fatalf("el porcentaje declarado no debe cambiar en un replay")
	}
}

func TestDistribuirSobreProporcionesPesaPorImporteNoPorPorcentaje(t *testing.T) {
	// Los dos titulares declaran el mismo Porcentaje (100, cada uno de su
	// propia obra), pero importes muy distintos. Si el peso fuera Porcentaje,
	// el reparto saldria 30/30; el reglamento pide repartir como se distribuyo
	// el recaudo original, que es lo que dice Importe.
	original := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: d("100"), Importe: d("100.00")},
		{ObraID: "obra-2", TitularID: "titular-b", IPI: "222", Porcentaje: d("100"), Importe: d("500.00")},
	}

	nuevas, residuo, err := reparto.DistribuirSobreProporciones(d("60.00"), original)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !nuevas[0].Importe.Equal(d("10.00")) || !nuevas[1].Importe.Equal(d("50.00")) {
		t.Fatalf("importes = %s, %s; se esperaba 10.00/50.00 (proporcion 100/500 del Importe original), "+
			"no 30.00/30.00 (lo que saldria ponderando por el Porcentaje declarado, igual en los dos)",
			nuevas[0].Importe, nuevas[1].Importe)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero", residuo)
	}
}

func TestDistribuirSobreProporcionesMontoNegativoEsError(t *testing.T) {
	original := []reparto.LineaTitular{
		{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: d("100"), Importe: d("100.00")},
	}

	_, _, err := reparto.DistribuirSobreProporciones(d("-1.00"), original)
	if err == nil {
		t.Fatal("se esperaba error con monto negativo")
	}
}

func TestDistribuirSobreProporcionesSinLineasDejaTodoElMontoComoResiduo(t *testing.T) {
	nuevas, residuo, err := reparto.DistribuirSobreProporciones(d("50.00"), nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(nuevas) != 0 {
		t.Fatalf("se esperaban cero lineas, hubo %d", len(nuevas))
	}
	if !residuo.Equal(d("50.00")) {
		t.Fatalf("residuo = %s, se esperaba 50.00 (nada que proporcionar)", residuo)
	}
}
