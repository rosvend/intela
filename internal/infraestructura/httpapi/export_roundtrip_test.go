package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"unicode"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/exportacion"
)

// Recorre el camino real: peticion HTTP -> caso de uso -> generador.
// Los totales del panel tienen que ser los mismos que van en el archivo.
// Dos lineas con netos distintos: el total no coincide con ninguna linea,
// asi una mutacion del total (p. ej. prefijar "1") no se camufla.
func TestExportarYPanelCompartenTotales(t *testing.T) {
	repo := &repoLiquidacionHTTP{filas: []aplicacion.FilaLiquidacion{
		{
			Periodo:        "2026-01",
			ObraID:         "obra-completa",
			Titulo:         "La Casa de las Dos Palmas",
			Neto:           decimal.RequireFromString("3900"),
			ProcesoBruto:   decimal.RequireFromString("10000"),
			ProcesoAdmin:   decimal.RequireFromString("2000"),
			ProcesoSocial:  decimal.RequireFromString("1000"),
			ProcesoReserva: decimal.RequireFromString("500"),
			ProcesoNeto:    decimal.RequireFromString("6500"),
		},
		{
			Periodo:        "2026-01",
			ObraID:         "obra-otra",
			Titulo:         "El Rio",
			Neto:           decimal.RequireFromString("2600"),
			ProcesoBruto:   decimal.RequireFromString("10000"),
			ProcesoAdmin:   decimal.RequireFromString("2000"),
			ProcesoSocial:  decimal.RequireFromString("1000"),
			ProcesoReserva: decimal.RequireFromString("500"),
			ProcesoNeto:    decimal.RequireFromString("6500"),
		},
	}}
	svc := aplicacion.ServicioLiquidacion{
		Repo: repo,
		Exportador: exportacion.Combinado{
			XLSX: exportacion.GeneradorExcel{},
			Docs: exportacion.GeneradorPDF{},
		},
	}
	auth := &autenticacionFalsa{usuario: titularAna()}
	h := servidorConReporte(t, auth, svc)

	panel := pedir(t, h, http.MethodGet, "/mis-liquidaciones/obras?periodo=2026-01", "", "tok")
	if panel.Code != http.StatusOK {
		t.Fatalf("panel: %d %s", panel.Code, panel.Body)
	}
	var cuerpo struct {
		Lineas []struct {
			Neto  string `json:"neto"`
			Bruto string `json:"bruto"`
		} `json:"lineas"`
		Totales struct {
			Bruto string `json:"bruto"`
			Admin string `json:"admin"`
			Neto  string `json:"neto"`
		} `json:"totales"`
	}
	if err := json.Unmarshal(panel.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("panel json: %v", err)
	}
	if len(cuerpo.Lineas) != 2 {
		t.Fatalf("lineas del panel = %d", len(cuerpo.Lineas))
	}
	if cuerpo.Totales.Neto != "6500.00" || cuerpo.Totales.Bruto != "10000.00" {
		t.Fatalf("totales del panel = %+v", cuerpo.Totales)
	}
	if cuerpo.Totales.Neto == cuerpo.Lineas[0].Neto || cuerpo.Totales.Neto == cuerpo.Lineas[1].Neto {
		t.Fatal("el total no puede coincidir con una linea: el test no detectaria un total mal sumado")
	}

	pdf := pedir(t, h, http.MethodGet, "/mis-liquidaciones/export?periodo=2026-01&formato=pdf", "", "tok")
	if pdf.Code != http.StatusOK {
		t.Fatalf("pdf: %d %s", pdf.Code, pdf.Body)
	}
	if pdf.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("pdf content-type = %q", pdf.Header().Get("Content-Type"))
	}
	if !strings.Contains(pdf.Header().Get("Content-Disposition"), "liquidacion-2026-01.pdf") {
		t.Fatalf("pdf disposition = %q", pdf.Header().Get("Content-Disposition"))
	}
	texto := textoPDFRoundtrip(pdf.Body.Bytes())
	for _, monto := range []string{cuerpo.Totales.Neto, cuerpo.Totales.Bruto, cuerpo.Lineas[0].Neto, cuerpo.Lineas[1].Neto} {
		if !contieneMontoExacto(texto, monto) {
			t.Fatalf("el PDF no lleva el monto exacto %q (un prefijo tipo 1%sseria falso positivo de Contains)", monto, monto)
		}
	}

	xlsx := pedir(t, h, http.MethodGet, "/mis-liquidaciones/export?periodo=2026-01&formato=xlsx", "", "tok")
	if xlsx.Code != http.StatusOK {
		t.Fatalf("xlsx: %d", xlsx.Code)
	}
	if !strings.Contains(xlsx.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("xlsx content-type = %q", xlsx.Header().Get("Content-Type"))
	}
	f, err := excelize.OpenReader(bytes.NewReader(xlsx.Body.Bytes()))
	if err != nil {
		t.Fatalf("abrir xlsx: %v", err)
	}
	defer func() { _ = f.Close() }()
	const hoja = "Liquidacion"
	for celda, quiero := range map[string]string{
		"G5": cuerpo.Lineas[0].Neto,
		"G6": cuerpo.Lineas[1].Neto,
		"C5": cuerpo.Lineas[0].Bruto,
		"C6": cuerpo.Lineas[1].Bruto,
	} {
		v, err := f.GetCellValue(hoja, celda)
		if err != nil {
			t.Fatalf("%s: %v", celda, err)
		}
		normalizado := strings.ReplaceAll(v, ",", "")
		fragmento := strings.TrimSuffix(quiero, ".00")
		if !strings.Contains(normalizado, fragmento) {
			t.Fatalf("%s = %q, se esperaba %s del panel", celda, v, quiero)
		}
	}
	formula, err := f.GetCellFormula(hoja, "G7")
	if err != nil || !strings.Contains(formula, "SUM(G5:G6)") {
		t.Fatalf("formula neto totales = %q (%v); tiene que sumar las dos lineas", formula, err)
	}
}

// contieneMontoExacto exige el monto como token (no subcadena de un numero
// mas grande). Asi "13900.00" no satisface la busqueda de "3900.00".
func contieneMontoExacto(texto, monto string) bool {
	for i := 0; ; {
		j := strings.Index(texto[i:], monto)
		if j < 0 {
			return false
		}
		j += i
		antesOK := j == 0 || !esDigitoMonto(texto[j-1])
		despues := j + len(monto)
		despuesOK := despues >= len(texto) || !esDigitoMonto(texto[despues])
		if antesOK && despuesOK {
			return true
		}
		i = j + 1
	}
}

func esDigitoMonto(b byte) bool {
	return b >= '0' && b <= '9'
}

func textoPDFRoundtrip(b []byte) string {
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

type repoLiquidacionHTTP struct {
	filas []aplicacion.FilaLiquidacion
}

func (r repoLiquidacionHTTP) FilasDeTitular(_ context.Context, _, periodo string) ([]aplicacion.FilaLiquidacion, error) {
	if periodo == "" {
		return r.filas, nil
	}
	var out []aplicacion.FilaLiquidacion
	for _, f := range r.filas {
		if f.Periodo == periodo {
			out = append(out, f)
		}
	}
	return out, nil
}
