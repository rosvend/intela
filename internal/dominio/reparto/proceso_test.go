package reparto

import "testing"

func TestInternacionalNoTieneValorizacion(t *testing.T) {
	p := Proceso{Circuito: Internacional, Etapa: EtapaDeducciones}
	p, err := Avanzar(p, "sistema")
	if err != nil {
		t.Fatal(err)
	}
	if p.Etapa != EtapaLiquidacionParcial {
		t.Fatalf("internacional no debe valorizar, etapa=%s", p.Etapa)
	}
}

func TestCompuertaExigeDobleFirma(t *testing.T) {
	p := Proceso{Circuito: Nacional, Etapa: EtapaVerificacion, Revision: 1}
	if _, err := Avanzar(p, "admin"); err == nil {
		t.Fatal("esperaba error")
	}
	p, _ = Firmar(p, "distribucion", "u1")
	p, _ = Firmar(p, "contabilidad", "u2")
	p, err := Avanzar(p, "admin")
	if err != nil || p.Etapa != EtapaLiquidacionFinal {
		t.Fatalf("%v %+v", err, p)
	}
}

func TestRechazoRetrocede(t *testing.T) {
	p := Proceso{Circuito: Nacional, Etapa: EtapaVerificacion, Revision: 1}
	p = Rechazar(p, "cifras")
	if p.Etapa != EtapaLiquidacionParcial || p.Revision != 2 {
		t.Fatalf("%+v", p)
	}
}
