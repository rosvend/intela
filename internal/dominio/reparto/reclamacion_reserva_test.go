package reparto_test

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func nuevaReclamacionValida(t *testing.T) reparto.ReclamacionReserva {
	t.Helper()
	r, err := reparto.NuevaReclamacionReserva("rec-1", "titular-1", "proceso-1", "error en el conteo de emisiones de enero", d("100.00"),
		true /* afiliado antes del periodo */, false /* no es error de declaracion */)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	return r
}

func TestNuevaReclamacionReservaRechazaAfiliacionPosterior(t *testing.T) {
	_, err := reparto.NuevaReclamacionReserva("rec-1", "titular-1", "proceso-1", "detalle", d("100.00"),
		false, false)
	if !errors.Is(err, reparto.ErrReclamacionAfiliacionPosterior) {
		t.Fatalf("error = %v, se esperaba ErrReclamacionAfiliacionPosterior (RD 14.5.5)", err)
	}
}

func TestNuevaReclamacionReservaRechazaErrorDeDeclaracion(t *testing.T) {
	_, err := reparto.NuevaReclamacionReserva("rec-1", "titular-1", "proceso-1", "detalle", d("100.00"),
		true, true)
	if !errors.Is(err, reparto.ErrReclamacionErrorDeDeclaracion) {
		t.Fatalf("error = %v, se esperaba ErrReclamacionErrorDeDeclaracion (RD 14.5.6)", err)
	}
}

func TestNuevaReclamacionReservaRechazaDetalleVacio(t *testing.T) {
	// RD 14.3 exige responder cada reclamo por escrito, individualmente: sin
	// un detalle no hay que responder.
	_, err := reparto.NuevaReclamacionReserva("rec-1", "titular-1", "proceso-1", "  ", d("100.00"),
		true, false)
	if err == nil {
		t.Fatal("se esperaba error con detalle vacio")
	}
}

func TestReclamacionReservaNoEsPagableSinAvales(t *testing.T) {
	r := nuevaReclamacionValida(t)
	if r.Pagable() {
		t.Fatal("una reclamacion recien abierta no puede ser pagable")
	}
}

func TestReclamacionReservaEsPagableConLosDosAvales(t *testing.T) {
	r := nuevaReclamacionValida(t)
	r, err := r.Avalar(reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-revisoria")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if r.Pagable() {
		t.Fatal("con un solo aval no deberia ser pagable (RD 14.5.10-12)")
	}
	r, err = r.Avalar(reparto.RolDistribucionYContabilidad, "actor-contabilidad")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !r.Pagable() {
		t.Fatal("con los dos avales de roles distintos deberia ser pagable")
	}
}

func TestReclamacionReservaUnActorNoCubreLosDosRoles(t *testing.T) {
	r := nuevaReclamacionValida(t)
	r, err := r.Avalar(reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-unico")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = r.Avalar(reparto.RolDistribucionYContabilidad, "actor-unico")
	if err == nil {
		t.Fatal("se esperaba error: el mismo actor no puede cubrir los dos roles")
	}
}

func TestReclamacionReservaRechazaRolDesconocido(t *testing.T) {
	r := nuevaReclamacionValida(t)
	_, err := r.Avalar(reparto.RolAvalReclamacion("contabilidad"), "actor-a")
	if err == nil {
		t.Fatal("se esperaba error: RD 14.5.10-12 solo define dos avales")
	}
}

func TestReclamacionReservaElMismoRolNoAvalaDosVeces(t *testing.T) {
	r := nuevaReclamacionValida(t)
	r, err := r.Avalar(reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-a")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = r.Avalar(reparto.RolRevisoriaFiscalOAuditoriaInterna, "actor-b")
	if err == nil {
		t.Fatal("se esperaba error: el rol ya avalo")
	}
}
