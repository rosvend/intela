package exportacion

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/rosvend/intela/internal/aplicacion"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func liquidacionEjemplo() aplicacion.Liquidacion {
	return aplicacion.Liquidacion{
		TitularID: "tit-ana",
		Periodo:   "2026-01",
		Lineas: []aplicacion.LineaLiquidacion{{
			Periodo: "2026-01",
			ObraID:  "obra-completa",
			Titulo:  "La Casa de las Dos Palmas",
			Bruto:   dec("6000.00"),
			Admin:   dec("1200.00"),
			Social:  dec("600.00"),
			Reserva: dec("300.00"),
			Neto:    dec("3900.00"),
		}},
		Totales: aplicacion.TotalesLiquidacion{
			Bruto:   dec("6000.00"),
			Admin:   dec("1200.00"),
			Social:  dec("600.00"),
			Reserva: dec("300.00"),
			Neto:    dec("3900.00"),
		},
	}
}

func TestGeneradorExcelIncluyeBrutoDeduccionesYNeto(t *testing.T) {
	archivo, err := GeneradorExcel{}.Generar(liquidacionEjemplo())
	if err != nil {
		t.Fatalf("Generar: %v", err)
	}
	if archivo.Nombre != "liquidacion-2026-01.xlsx" {
		t.Fatalf("nombre = %q", archivo.Nombre)
	}
	if archivo.TipoMIME != tipoXLSX {
		t.Fatalf("mime = %q", archivo.TipoMIME)
	}

	f, err := excelize.OpenReader(bytes.NewReader(archivo.Contenido))
	if err != nil {
		t.Fatalf("el xlsx no se puede abrir: %v", err)
	}
	defer func() { _ = f.Close() }()

	if v, _ := f.GetCellValue(hojaLiquidacion, "A4"); v != "Periodo" {
		t.Fatalf("A4 = %q, se esperaba el encabezado Periodo", v)
	}
	if v, _ := f.GetCellValue(hojaLiquidacion, "B5"); v != "La Casa de las Dos Palmas" {
		t.Fatalf("obra = %q", v)
	}
	for celda, fragmento := range map[string]string{
		"C5": "6000",
		"D5": "1200",
		"E5": "600",
		"F5": "300",
		"G5": "3900",
	} {
		v, err := f.GetCellValue(hojaLiquidacion, celda)
		if err != nil {
			t.Fatalf("%s: %v", celda, err)
		}
		if !strings.Contains(strings.ReplaceAll(v, ",", ""), fragmento) {
			t.Fatalf("%s = %q, se esperaba %s", celda, v, fragmento)
		}
	}

	formula, err := f.GetCellFormula(hojaLiquidacion, "G6")
	if err != nil || !strings.Contains(formula, "SUM(G5:G5)") {
		t.Fatalf("formula neto = %q (%v)", formula, err)
	}

	hay, _, err := f.GetCellHyperLink(hojaLiquidacion, "B5")
	if err != nil {
		t.Fatalf("GetCellHyperLink: %v", err)
	}
	if hay {
		t.Fatal("el archivo no puede llevar hipervinculos: tiene que valer offline")
	}
}

func TestGeneradorExcelFiltraPorPeriodoEnElNombre(t *testing.T) {
	liq := liquidacionEjemplo()
	liq.Periodo = ""
	archivo, err := GeneradorExcel{}.Generar(liq)
	if err != nil {
		t.Fatalf("Generar: %v", err)
	}
	if archivo.Nombre != "liquidacion.xlsx" {
		t.Fatalf("nombre = %q", archivo.Nombre)
	}
}

func TestGeneradorPDFEsUnPDFConLasCifrasEmbebidas(t *testing.T) {
	archivo, err := GeneradorPDF{}.Generar(liquidacionEjemplo())
	if err != nil {
		t.Fatalf("Generar: %v", err)
	}
	if archivo.Nombre != "liquidacion-2026-01.pdf" {
		t.Fatalf("nombre = %q", archivo.Nombre)
	}
	if archivo.TipoMIME != tipoPDF {
		t.Fatalf("mime = %q", archivo.TipoMIME)
	}
	if !bytes.HasPrefix(archivo.Contenido, []byte("%PDF")) {
		t.Fatalf("no empieza por %%PDF: %q", archivo.Contenido[:min(16, len(archivo.Contenido))])
	}

	texto := textoPDF(archivo.Contenido)
	for _, quiero := range []string{"Liquidacion", "Bruto", "Admin", "Social", "Reserva", "Neto", "3900.00", "6000.00", "La Casa"} {
		if !strings.Contains(texto, quiero) {
			t.Fatalf("el PDF no embebida %q. Texto extraido: %s", quiero, recortar(texto, 500))
		}
	}
	if strings.Contains(texto, "https://") || strings.Contains(string(archivo.Contenido), "http://localhost") {
		t.Fatal("el PDF no puede llevar enlaces vivos: tiene que valer offline")
	}
	if !bytes.Contains(archivo.Contenido, []byte("/Type /Page")) && !bytes.Contains(archivo.Contenido, []byte("/Type/Page")) {
		t.Fatal("el PDF no declara ninguna pagina")
	}
}

func textoPDF(b []byte) string {
	var bld strings.Builder
	run := 0
	for _, c := range b {
		if c >= 32 && c < 127 && unicode.IsPrint(rune(c)) {
			bld.WriteByte(c)
			run++
			continue
		}
		if run > 0 {
			bld.WriteByte(' ')
			run = 0
		}
	}
	return bld.String()
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
