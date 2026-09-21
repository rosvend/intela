package reparto_test

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestNuevaPoolReservaCircuitoNacionalFijaAsambleaGeneralComoOrgano(t *testing.T) {
	p, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, d("5000.00"), d("5"))
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.OrganoAprobador != "Asamblea General" {
		t.Fatalf("organo aprobador = %q, se esperaba %q", p.OrganoAprobador, "Asamblea General")
	}
	if !p.Saldo.Equal(d("5000.00")) {
		t.Fatalf("saldo inicial = %s, se esperaba igual al monto inicial", p.Saldo)
	}
	if p.ProcesoID != "proceso-1" {
		t.Fatalf("proceso_id = %q, se esperaba la proveniencia de la corrida", p.ProcesoID)
	}
}

func TestNuevaPoolReservaRechazaCircuitoInternacional(t *testing.T) {
	_, err := reparto.NuevaPoolReserva("proceso-1", reparto.Internacional, d("100.00"), d("5"))
	if !errors.Is(err, reparto.ErrReservaInternacional) {
		t.Fatalf("error = %v, se esperaba ErrReservaInternacional (RD 14.5.4)", err)
	}
}

func TestNuevaPoolReservaRechazaTasaSobreElTecho(t *testing.T) {
	_, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, d("100.00"), d("5.01"))
	if !errors.Is(err, reparto.ErrRepartoInvalido) {
		t.Fatalf("error = %v, se esperaba ErrRepartoInvalido: la tasa supera el techo de 5%% (RD 14.1)", err)
	}
}

func TestNuevaPoolReservaTasaAusenteEsError(t *testing.T) {
	_, err := reparto.NuevaPoolReserva("proceso-1", reparto.Nacional, d("100.00"), d("0"))
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("error = %v, se esperaba ErrParametroAusente: tasa cero es ausente, no cero legitimo (ADR 0004)", err)
	}
}
