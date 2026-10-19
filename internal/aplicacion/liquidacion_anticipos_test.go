package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// Pruebas del gancho de R-33 (RA 2.1) en la emision: la liquidacion le pide al
// modulo de anticipos cuanto descontar de cada orden y lo deja como un renglon
// mas del desglose. La aritmetica del saldo se prueba en el dominio
// (anticipos.Anticipo.Recuperar) y la cadena entera contra Postgres
// (postgres/anticipos_integracion_test.go); aqui se prueba el cableado: que se
// pregunta por cada orden, con lo que la orden iba a girar, antes del aviso, y
// que un fallo del descuento no deja una orden emitida.

func deduccionDe(o liquidacion.OrdenDePago, concepto string) (decimal.Decimal, bool) {
	for _, d := range o.Deducciones {
		if d.Concepto == concepto {
			return d.Monto, true
		}
	}
	return decimal.Zero, false
}

// TestGenerarLiquidacionDescuentaElAnticipoDelNetoDeLaOrden: Ana debe 300 de
// su anticipo y su orden iba a girar 390. La orden sale con el renglon
// `anticipo` de 300 y neto 90; el bruto no cambia, porque el anticipo no altera
// lo que el socio genero en el periodo. Beto, sin anticipo, sale como siempre.
func TestGenerarLiquidacionDescuentaElAnticipoDelNetoDeLaOrden(t *testing.T) {
	repo := repoDosTitulares()
	e := montar(repo, envio())
	e.anticipos.saldos["tit-ana"] = liqDec("300")

	vistas, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1")
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	ordenes := porTitular(vistas)
	ana, beto := ordenes["tit-ana"].Orden, ordenes["tit-beto"].Orden

	descuento, hay := deduccionDe(ana, liquidacion.ConceptoAnticipo)
	if !hay || !descuento.Equal(liqDec("300")) {
		t.Fatalf("deducciones de Ana = %+v, se esperaba el renglon anticipo de 300", ana.Deducciones)
	}
	if !ana.Neto.Equal(liqDec("90")) {
		t.Fatalf("neto de Ana = %s, se esperaba 390 - 300 = 90", ana.Neto)
	}
	if !ana.Bruto.Equal(liqDec("600")) {
		t.Fatalf("bruto de Ana = %s; el anticipo no toca el bruto del periodo", ana.Bruto)
	}
	if _, hay := deduccionDe(beto, liquidacion.ConceptoAnticipo); hay || !beto.Neto.Equal(liqDec("260")) {
		t.Fatalf("Beto sin anticipo salio con %+v y neto %s", beto.Deducciones, beto.Neto)
	}

	// Se pregunta por CADA orden, con su id y con lo que iba a girar.
	llamadas := map[string]llamadaRecuperar{}
	for _, l := range e.anticipos.llamadas {
		llamadas[l.TitularID] = l
	}
	if l := llamadas["tit-ana"]; l.OrdenID != ana.ID || !l.Disponible.Equal(liqDec("390")) {
		t.Fatalf("llamada de Ana = %+v, se esperaba la orden %s con 390 disponibles", l, ana.ID)
	}
	if l := llamadas["tit-beto"]; l.OrdenID != beto.ID || !l.Disponible.Equal(liqDec("260")) {
		t.Fatalf("llamada de Beto = %+v", l)
	}

	// Lo que el titular ve (RD 13.2): el aviso lleva el neto ya descontado y
	// dice por que, y el asiento de emision lleva el renglon.
	var cuerpoAna string
	for _, a := range e.avisos.enviados {
		if a.Dest == "tit-ana" {
			cuerpoAna = a.Cuerpo
		}
	}
	if !strings.Contains(cuerpoAna, "neto 90.00") || !strings.Contains(cuerpoAna, "Descuento de anticipo (R-33): 300.00") {
		t.Fatalf("aviso de Ana = %q; tiene que mostrar el neto descontado y el renglon del anticipo", cuerpoAna)
	}
	var emision AsientoOrden
	for _, a := range e.libro.asientos {
		if a.Hecho == HechoLiquidacionEmitida && a.RefID == ana.ID {
			if err := json.Unmarshal(a.Payload, &emision); err != nil {
				t.Fatalf("payload de la emision: %v", err)
			}
		}
	}
	asentado := false
	for _, d := range emision.Deducciones {
		if d.Concepto == liquidacion.ConceptoAnticipo && d.Monto == "300.00" {
			asentado = true
		}
	}
	if !asentado || emision.Neto != "90.00" {
		t.Fatalf("asiento de emision = %+v; el libro tiene que llevar el renglon del anticipo", emision)
	}
}

// TestGenerarLiquidacionDescuentaElAnticipoTambienDelArrastre: lo disponible
// es el neto de la orden DESPUES de incorporar las diferidas de R-11. Tambien
// son derechos del socio que no se le han girado (ADR 0027), y dejarlos fuera
// le pagaria entero el arrastre a quien debe un anticipo.
func TestGenerarLiquidacionDescuentaElAnticipoTambienDelArrastre(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true, CertificacionBancaria: true}},
	}
	diferida, err := liquidacion.NuevaOrden(liquidacion.DatosOrden{
		ID: "liq-2026-01-nacional-tit-ana", ProcesoID: "prc-01",
		TitularID: "tit-ana", Periodo: "2026-01", Circuito: "nacional",
		EnviadaDia: "2026-01-01", Bruto: liqDec("50"),
	})
	if err != nil {
		t.Fatalf("diferida: %v", err)
	}
	diferida.Estado = liquidacion.EstadoDiferida
	repo.ordenes = append(repo.ordenes, diferida)
	repo.sembrar(metaLista("prc-02", "2026-02", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("400"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("400")}},
	})
	e := montar(repo, envio())
	e.anticipos.saldos["tit-ana"] = liqDec("1000")

	vistas, err := e.svc.GenerarLiquidacion(context.Background(), "prc-02")
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	o := vistas[0].Orden
	if len(o.Arrastres) != 1 {
		t.Fatalf("arrastres = %v; la prueba necesita la diferida incorporada", o.Arrastres)
	}
	if len(e.anticipos.llamadas) != 1 || !e.anticipos.llamadas[0].Disponible.Equal(liqDec("450")) {
		t.Fatalf("llamadas = %+v; se esperaba 400 del periodo + 50 del arrastre", e.anticipos.llamadas)
	}
	if !o.Neto.IsZero() {
		t.Fatalf("neto = %s; con 1000 de saldo se descuenta todo lo disponible", o.Neto)
	}
	if d, _ := deduccionDe(o, liquidacion.ConceptoAnticipo); !d.Equal(liqDec("450")) {
		t.Fatalf("descuento = %s, se esperaba 450", d)
	}
}

// TestGenerarLiquidacionSiElDescuentoFallaNoEmiteNiAvisa: el descuento va en la
// unidad de la emision. Si no se puede saber cuanto debe el socio, la orden no
// sale: emitirla sin el renglon giraria lo que debe, y el aviso ya habria
// arrancado el plazo de R-10 sobre una cifra equivocada.
func TestGenerarLiquidacionSiElDescuentoFallaNoEmiteNiAvisa(t *testing.T) {
	repo := repoDosTitulares()
	e := montar(repo, envio())
	falla := errors.New("la base no responde")
	e.anticipos.err = falla

	_, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1")
	if !errors.Is(err, falla) {
		t.Fatalf("err = %v, se esperaba el fallo del descuento", err)
	}
	if len(repo.ordenes) != 0 || repo.emisiones != 0 {
		t.Fatalf("se emitieron %d ordenes con el descuento roto", len(repo.ordenes))
	}
	if len(e.avisos.enviados) != 0 {
		t.Fatalf("se enviaron %d avisos; el descuento se resuelve antes de avisar", len(e.avisos.enviados))
	}
}

// TestGenerarLiquidacionSinRecuperacionDeAnticiposNoEmite: sin el puerto
// cableado la emision falla con un mensaje que lo nombra, en vez de girar
// enteras las ordenes de quien debe un anticipo.
func TestGenerarLiquidacionSinRecuperacionDeAnticiposNoEmite(t *testing.T) {
	repo := repoDosTitulares()
	e := montar(repo, envio())
	e.svc.Anticipos = nil

	_, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1")
	if err == nil || !strings.Contains(err.Error(), "RecuperacionAnticipos") {
		t.Fatalf("err = %v, se esperaba el fallo de cableado que nombra RecuperacionAnticipos", err)
	}
	if len(repo.ordenes) != 0 || len(e.avisos.enviados) != 0 {
		t.Fatal("una emision mal cableada no puede dejar ordenes ni avisos")
	}
}
