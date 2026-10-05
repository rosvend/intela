package reparto_test

import (
	"slices"
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestEtapasDeSeparaLosDosCircuitos(t *testing.T) {
	nac := reparto.EtapasDe(reparto.Nacional)
	inter := reparto.EtapasDe(reparto.Internacional)
	if len(nac) != 9 || len(inter) != 8 {
		t.Fatalf("nacional %d etapas, internacional %d; RD 13.5 trae 9 y 8", len(nac), len(inter))
	}
	if slices.Contains(inter, reparto.EtapaImporteObra) || !slices.Contains(inter, reparto.EtapaFeesInError) {
		t.Fatalf("internacional = %v: no valoriza (RD 7.4) y si tiene Fees in Error (RD 13.7)", inter)
	}
	if slices.Contains(nac, reparto.EtapaFeesInError) {
		t.Fatalf("nacional = %v: Fees in Error es solo del internacional", nac)
	}
}

func TestEtapasDeDevuelveUnaCopia(t *testing.T) {
	reparto.EtapasDe(reparto.Nacional)[0] = reparto.EtapaAuditoria
	if reparto.EtapasDe(reparto.Nacional)[0] != reparto.EtapaRecaudo {
		t.Fatal("mutar el resultado cambio la secuencia del circuito")
	}
}

func TestFirmasFaltantesFueraDeCompuertaEsVacio(t *testing.T) {
	p, _ := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if p.EnCompuerta() {
		t.Fatal("recaudo no es compuerta")
	}
	if f := p.FirmasFaltantes(); len(f) != 0 {
		t.Fatalf("faltantes = %v, fuera de compuerta no falta ninguna", f)
	}
}

func TestFirmasFaltantesCuentaSoloLaRevisionActual(t *testing.T) {
	p := procesoEnVerificacion(t)
	if !p.EnCompuerta() {
		t.Fatal("verificacion es compuerta")
	}
	if f := p.FirmasFaltantes(); !slices.Equal(f, []reparto.RolAcompuerta{reparto.RolDistribucion, reparto.RolContabilidad}) {
		t.Fatalf("faltantes = %v, se esperaban las dos", f)
	}
	p, _ = p.Firmar(reparto.RolDistribucion, "actor-dist")
	if f := p.FirmasFaltantes(); !slices.Equal(f, []reparto.RolAcompuerta{reparto.RolContabilidad}) {
		t.Fatalf("faltantes = %v, se esperaba solo contabilidad", f)
	}
	p.Revision++
	if f := p.FirmasFaltantes(); len(f) != 2 {
		t.Fatalf("faltantes = %v: una firma de otra revision no cuenta", f)
	}
}
