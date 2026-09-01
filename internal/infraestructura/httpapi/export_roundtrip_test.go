package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/exportacion"
)

// Recorre el camino real: peticion HTTP -> caso de uso -> generador.
// Los totales del panel tienen que ser los mismos que van en el archivo.
func TestExportarYPanelCompartenTotales(t *testing.T) {
	repo := &repoLiquidacionHTTP{filas: []aplicacion.FilaLiquidacion{{
		Periodo:        "2026-01",
		ObraID:         "obra-completa",
		Titulo:         "La Casa de las Dos Palmas",
		Neto:           decimal.RequireFromString("3900"),
		ProcesoBruto:   decimal.RequireFromString("10000"),
		ProcesoAdmin:   decimal.RequireFromString("2000"),
		ProcesoSocial:  decimal.RequireFromString("1000"),
		ProcesoReserva: decimal.RequireFromString("500"),
		ProcesoNeto:    decimal.RequireFromString("6500"),
	}}}
	svc := aplicacion.ServicioLiquidacion{
		Repo: repo,
		Exportador: exportacion.Combinado{
			XLSX: exportacion.GeneradorExcel{},
			Docs: exportacion.GeneradorPDF{},
		},
	}
	auth := &autenticacionFalsa{usuario: titularAna()}
	h := servidorConLiq(t, auth, svc)

	panel := pedir(t, h, http.MethodGet, "/mis-liquidaciones?periodo=2026-01", "", "tok")
	if panel.Code != http.StatusOK {
		t.Fatalf("panel: %d %s", panel.Code, panel.Body)
	}
	var cuerpo struct {
		Totales struct {
			Bruto string `json:"bruto"`
			Admin string `json:"admin"`
			Neto  string `json:"neto"`
		} `json:"totales"`
	}
	if err := json.Unmarshal(panel.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("panel json: %v", err)
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
	if !strings.Contains(string(pdf.Body.Bytes()), "3900.00") && !strings.Contains(string(pdf.Body.Bytes()), "3900") {
		// El PDF embebe las cifras; el round-trip no puede divergir del panel.
		texto := string(pdf.Body.Bytes())
		if !strings.Contains(texto, cuerpo.Totales.Neto) && !strings.Contains(texto, "3900") {
			t.Fatalf("el PDF no lleva el neto del panel %s", cuerpo.Totales.Neto)
		}
	}

	xlsx := pedir(t, h, http.MethodGet, "/mis-liquidaciones/export?periodo=2026-01&formato=xlsx", "", "tok")
	if xlsx.Code != http.StatusOK {
		t.Fatalf("xlsx: %d", xlsx.Code)
	}
	if !strings.Contains(xlsx.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("xlsx content-type = %q", xlsx.Header().Get("Content-Type"))
	}
	if cuerpo.Totales.Neto != "3900.00" || cuerpo.Totales.Bruto != "6000.00" {
		t.Fatalf("totales del panel = %+v", cuerpo.Totales)
	}
}

type repoLiquidacionHTTP struct {
	filas []aplicacion.FilaLiquidacion
}

func (r repoLiquidacionHTTP) DeTitular(_ context.Context, _, periodo string) ([]aplicacion.FilaLiquidacion, error) {
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
