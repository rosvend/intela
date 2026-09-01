package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

type liquidacionesFalsa struct {
	liq     aplicacion.Liquidacion
	archivo aplicacion.Archivo
	err     error

	vistoActor   aplicacion.Usuario
	vistoPeriodo string
	vistoFormato string
}

func (l *liquidacionesFalsa) Consultar(_ context.Context, actor aplicacion.Usuario, periodo string) (aplicacion.Liquidacion, error) {
	l.vistoActor = actor
	l.vistoPeriodo = periodo
	return l.liq, l.err
}

func (l *liquidacionesFalsa) Exportar(_ context.Context, actor aplicacion.Usuario, periodo, formato string) (aplicacion.Archivo, error) {
	l.vistoActor = actor
	l.vistoPeriodo = periodo
	l.vistoFormato = formato
	return l.archivo, l.err
}

func titularAna() aplicacion.Usuario {
	return aplicacion.Usuario{ID: "usr-ana", Email: "ana@redes.co", Nombre: "Ana", Rol: aplicacion.RolTitular, TitularID: "tit-ana"}
}

func servidorConLiq(t *testing.T, auth Autenticacion, liq Liquidaciones) http.Handler {
	t.Helper()
	return Nueva(nil, auth, liq, Opciones{}).Router()
}

func TestConsultarLiquidacionesDevuelveElPanel(t *testing.T) {
	liq := aplicacion.Liquidacion{
		TitularID: "tit-ana",
		Periodo:   "2026-01",
		Lineas: []aplicacion.LineaLiquidacion{{
			Periodo: "2026-01", ObraID: "obra-1", Titulo: "La Casa",
			Bruto: decimal.RequireFromString("6000"), Admin: decimal.RequireFromString("1200"),
			Social: decimal.RequireFromString("600"), Reserva: decimal.RequireFromString("300"),
			Neto: decimal.RequireFromString("3900"),
		}},
		Totales: aplicacion.TotalesLiquidacion{
			Bruto: decimal.RequireFromString("6000"), Neto: decimal.RequireFromString("3900"),
			Admin: decimal.RequireFromString("1200"), Social: decimal.RequireFromString("600"),
			Reserva: decimal.RequireFromString("300"),
		},
	}
	fake := &liquidacionesFalsa{liq: liq}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorConLiq(t, auth, fake), http.MethodGet, "/mis-liquidaciones?periodo=2026-01", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if fake.vistoPeriodo != "2026-01" {
		t.Fatalf("periodo = %q", fake.vistoPeriodo)
	}
	if fake.vistoActor.TitularID != "tit-ana" {
		t.Fatal("el titular tiene que salir de la sesion")
	}
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, `"neto":"3900.00"`) || !strings.Contains(cuerpo, `"bruto":"6000.00"`) {
		t.Fatalf("el panel no lleva bruto/neto: %s", cuerpo)
	}
}

func TestExportarLiquidacionPDFCabecerasYCuerpo(t *testing.T) {
	fake := &liquidacionesFalsa{archivo: aplicacion.Archivo{
		Nombre:    "liquidacion-2026-01.pdf",
		TipoMIME:  "application/pdf",
		Contenido: []byte("%PDF-1.4 embebido 3900.00"),
	}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorConLiq(t, auth, fake), http.MethodGet,
		"/mis-liquidaciones/export?periodo=2026-01&formato=pdf", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "liquidacion-2026-01.pdf") {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("cuerpo = %q", rec.Body)
	}
	if fake.vistoFormato != "pdf" || fake.vistoPeriodo != "2026-01" {
		t.Fatalf("visto formato=%q periodo=%q", fake.vistoFormato, fake.vistoPeriodo)
	}
}

func TestExportarLiquidacionExcelCabeceras(t *testing.T) {
	fake := &liquidacionesFalsa{archivo: aplicacion.Archivo{
		Nombre:    "liquidacion-2026-01.xlsx",
		TipoMIME:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Contenido: []byte("PK"),
	}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorConLiq(t, auth, fake), http.MethodGet,
		"/mis-liquidaciones/export?formato=xlsx", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestExportarFormatoInvalidoEs400(t *testing.T) {
	fake := &liquidacionesFalsa{err: aplicacion.ErrFormatoInvalido}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorConLiq(t, auth, fake), http.MethodGet,
		"/mis-liquidaciones/export?formato=csv", "", "tok")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestExportarPeriodoInvalidoEs400(t *testing.T) {
	fake := &liquidacionesFalsa{err: aplicacion.ErrPeriodoInvalido}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorConLiq(t, auth, fake), http.MethodGet,
		"/mis-liquidaciones/export?periodo=enero&formato=pdf", "", "tok")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d", rec.Code)
	}
}

func TestMisLiquidacionesSinSesionEs401(t *testing.T) {
	h := servidorConLiq(t, &autenticacionFalsa{}, &liquidacionesFalsa{})
	for _, ruta := range []string{"/mis-liquidaciones", "/mis-liquidaciones/export?formato=pdf"} {
		rec := pedir(t, h, http.MethodGet, ruta, "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s dio %d, se esperaba 401", ruta, rec.Code)
		}
	}
}

func TestMisLiquidacionesConOtroRolEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	h := servidorConLiq(t, auth, &liquidacionesFalsa{})

	rec := pedir(t, h, http.MethodGet, "/mis-liquidaciones", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
}
