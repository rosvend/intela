package reparto_test

import (
	"testing"

	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestAbrirProcesoEmpiezaEnRecaudoConRevisionUno(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Etapa != reparto.EtapaRecaudo {
		t.Fatalf("etapa = %q, se esperaba %q", p.Etapa, reparto.EtapaRecaudo)
	}
	if p.Revision != 1 {
		t.Fatalf("revision = %d, se esperaba 1", p.Revision)
	}
	if len(p.Firmas) != 0 {
		t.Fatalf("firmas = %v, se esperaba ninguna al abrir", p.Firmas)
	}
}

func procesoEnVerificacion(t *testing.T) reparto.ProcesoDeReparto {
	t.Helper()
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p.Etapa = reparto.EtapaVerificacion
	return p
}

func TestFirmarRechazaFueraDeCompuerta(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = p.Firmar(reparto.RolDistribucion, "actor-1")
	if err == nil {
		t.Fatal("se esperaba error: recaudo no es una etapa con compuerta")
	}
}

func TestFirmarRechazaRolDesconocido(t *testing.T) {
	p := procesoEnVerificacion(t)
	_, err := p.Firmar(reparto.RolAcompuerta("gerencia"), "actor-1")
	if err == nil {
		t.Fatal("se esperaba error: RD 13.5 solo admite distribucion y contabilidad")
	}
}

func TestFirmarAgregaLaFirma(t *testing.T) {
	p := procesoEnVerificacion(t)
	p, err := p.Firmar(reparto.RolDistribucion, "actor-dist")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(p.Firmas) != 1 {
		t.Fatalf("firmas = %v, se esperaba una", p.Firmas)
	}
	if p.Firmas[0].Rol != string(reparto.RolDistribucion) || p.Firmas[0].ActorID != "actor-dist" || p.Firmas[0].SobreRev != p.Revision {
		t.Fatalf("firma = %+v, no coincide con lo firmado", p.Firmas[0])
	}
}

func TestFirmarRechazaMismoRolDosVeces(t *testing.T) {
	p := procesoEnVerificacion(t)
	p, err := p.Firmar(reparto.RolDistribucion, "actor-a")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = p.Firmar(reparto.RolDistribucion, "actor-b")
	if err == nil {
		t.Fatal("se esperaba error: el rol ya firmo esta revision")
	}
}

func TestFirmarRechazaMismoActorParaLosDosRoles(t *testing.T) {
	p := procesoEnVerificacion(t)
	p, err := p.Firmar(reparto.RolDistribucion, "actor-unico")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = p.Firmar(reparto.RolContabilidad, "actor-unico")
	if err == nil {
		t.Fatal("se esperaba error: el mismo actor no puede cubrir los dos roles")
	}
}

func TestAvanzarEtapaFueraDeCompuertaAvanzaLibre(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err = p.AvanzarEtapa()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Etapa != reparto.EtapaDeducciones {
		t.Fatalf("etapa = %q, se esperaba %q", p.Etapa, reparto.EtapaDeducciones)
	}
}

func TestAvanzarEtapaSiguePorTodaLaSecuenciaNacional(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	esperado := []reparto.Etapa{
		reparto.EtapaDeducciones, reparto.EtapaImporteObra, reparto.EtapaImporteTitular,
		reparto.EtapaLiquidacionParcial, reparto.EtapaVerificacion,
	}
	for _, e := range esperado {
		if p.Etapa == reparto.EtapaVerificacion {
			break
		}
		p, err = p.AvanzarEtapa()
		if err != nil {
			t.Fatalf("error inesperado avanzando a %q: %v", e, err)
		}
		if p.Etapa != e {
			t.Fatalf("etapa = %q, se esperaba %q", p.Etapa, e)
		}
	}
}

func TestAvanzarEtapaEnCompuertaSinFirmasFalla(t *testing.T) {
	p := procesoEnVerificacion(t)
	_, err := p.AvanzarEtapa()
	if err == nil {
		t.Fatal("se esperaba error: verificacion sin firmas no avanza")
	}
}

func TestAvanzarEtapaEnCompuertaConUnaFirmaFalla(t *testing.T) {
	p := procesoEnVerificacion(t)
	p, err := p.Firmar(reparto.RolDistribucion, "actor-dist")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = p.AvanzarEtapa()
	if err == nil {
		t.Fatal("se esperaba error: una sola firma no basta")
	}
}

func TestAvanzarEtapaEnCompuertaConLasDosFirmasAvanza(t *testing.T) {
	p := procesoEnVerificacion(t)
	p, err := p.Firmar(reparto.RolDistribucion, "actor-dist")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err = p.Firmar(reparto.RolContabilidad, "actor-conta")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err = p.AvanzarEtapa()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Etapa != reparto.EtapaLiquidacionFinal {
		t.Fatalf("etapa = %q, se esperaba %q", p.Etapa, reparto.EtapaLiquidacionFinal)
	}
}

func TestAvanzarEtapaTerminaEnAuditoria(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	for p.Etapa != reparto.EtapaAuditoria {
		if p.Etapa == reparto.EtapaVerificacion || p.Etapa == reparto.EtapaPagoRegistro {
			p, err = p.Firmar(reparto.RolDistribucion, "actor-dist")
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			p, err = p.Firmar(reparto.RolContabilidad, "actor-conta")
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
		}
		p, err = p.AvanzarEtapa()
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
	}
	_, err = p.AvanzarEtapa()
	if err == nil {
		t.Fatal("se esperaba error: auditoria es terminal")
	}
}

func TestAvanzarEtapaCircuitoInternacionalSaltaValorizacion(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Internacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err = p.AvanzarEtapa() // deducciones
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	p, err = p.AvanzarEtapa() // liquidacion_parcial, nunca importe_obra/importe_titular
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatalf("etapa = %q, el internacional no valoriza por puntos (RD 7.4)", p.Etapa)
	}
}

func TestRechazarGateRechazaFueraDeCompuerta(t *testing.T) {
	p, err := reparto.AbrirProceso("proc-1", "2026-01", reparto.Nacional, "bolsa-1", "snap-1", "IX")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	_, err = p.RechazarGate("no cuadra")
	if err == nil {
		t.Fatal("se esperaba error: recaudo no es una etapa con compuerta")
	}
}

func TestRechazarGateRetrocedeUnaEtapaYSubeRevision(t *testing.T) {
	p := procesoEnVerificacion(t)
	p, err := p.Firmar(reparto.RolDistribucion, "actor-dist")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	revisionAntes := p.Revision
	p, err = p.RechazarGate("faltan soportes")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if p.Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatalf("etapa = %q, se esperaba retroceder a %q, no a un estado terminal", p.Etapa, reparto.EtapaLiquidacionParcial)
	}
	if p.Revision != revisionAntes+1 {
		t.Fatalf("revision = %d, se esperaba %d", p.Revision, revisionAntes+1)
	}
	if p.RechazoMotivo != "faltan soportes" {
		t.Fatalf("rechazoMotivo = %q, se esperaba el motivo dado", p.RechazoMotivo)
	}
	if len(p.Firmas) != 0 {
		t.Fatalf("firmas = %v, las firmas de la revision anterior no deberian contar", p.Firmas)
	}
}

func TestRechazarGateExigeMotivo(t *testing.T) {
	p := procesoEnVerificacion(t)
	_, err := p.RechazarGate("  ")
	if err == nil {
		t.Fatal("se esperaba error: un rechazo sin motivo no es explicable")
	}
}

func TestAbrirProcesoRechazaCamposVacios(t *testing.T) {
	casos := []struct {
		nombre                                       string
		id, periodo, bolsaID, snapshotID, reglamento string
	}{
		{"id vacio", "", "2026-01", "bolsa-1", "snap-1", "IX"},
		{"periodo vacio", "proc-1", "", "bolsa-1", "snap-1", "IX"},
		{"bolsaID vacio", "proc-1", "2026-01", "", "snap-1", "IX"},
	}
	for _, c := range casos {
		_, err := reparto.AbrirProceso(c.id, c.periodo, reparto.Nacional, c.bolsaID, c.snapshotID, c.reglamento)
		if err == nil {
			t.Fatalf("%s: se esperaba error", c.nombre)
		}
	}
}
