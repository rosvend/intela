package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

const (
	procesoNac = "prc-nac-2026"
	bolsaNac   = "bolsa-nac-2026"
)

func pgDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// sembrarCorrida deja un proceso nacional con resultado y lineas de
// titular, el SMMLV vigente y, si conDocs, RUT + banco de Ana.
func sembrarCorrida(t *testing.T, conDocs bool) (*Store, *aplicacion.Liquidaciones) {
	t.Helper()
	s, pool := sembrar(t)
	ctx := t.Context()

	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar corrida (%s): %v", sql, err)
		}
	}

	ejecutar(`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
	          VALUES ($1, 'usr-caracol', '2026', 'nacional', 1000000)`, bolsaNac)
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
	          VALUES ($1, 'nacional', 'liquidacion_final', '2026', $2, 'snap-1', 'RD-IX')`,
		procesoNac, bolsaNac)
	ejecutar(`INSERT INTO resultados_proceso
	            (proceso_id, bruto, admin, social, reserva, neto, retenido, residuo, valor_punto, snapshot_id, reglamento)
	          VALUES ($1, 1000000, 200000, 100000, 50000, 650000, 0, 0, 1, 'snap-1', 'RD-IX')`, procesoNac)
	ejecutar(`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
	          VALUES ($1, $2, 10, 650000, FALSE)`, procesoNac, obraCompleta)
	// 60/40 de la obra completa: 390000 y 260000, suman el neto 650000.
	// Ambos superan el 2% de 1_300_000 (26000), para que el silencio sea
	// R-10 y no el arrastre de R-11.
	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
	          VALUES ($1, $2, $3, 'IPI-00000001', 60, 390000),
	                 ($1, $2, $4, 'IPI-00000002', 40, 260000)`,
		procesoNac, obraCompleta, titularAna, titularBeto)

	ejecutar(`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
	          VALUES ('smmlv', 1300000, '2026-01-01', 'Gobierno Nacional', 'Decreto SMMLV 2026')`)

	if conDocs {
		ejecutar(`INSERT INTO documentos_titular (titular_id, tipo, clave_objeto)
		          VALUES ($1, 'rut', 'docs/ana/rut'),
		                 ($1, 'certificacion_bancaria', 'docs/ana/banco')`, titularAna)
	}

	liq := &aplicacion.Liquidaciones{
		Ordenes: s,
		Reloj:   reloj.Fijo{Instante: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
	}
	return s, liq
}

func TestGenerarLiquidacionDesdeResultadoYDeTitular(t *testing.T) {
	s, liq := sembrarCorrida(t, true)
	ctx := t.Context()

	vistas, err := liq.GenerarLiquidacion(ctx, procesoNac)
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	if len(vistas) != 2 {
		t.Fatalf("se esperaban 2 ordenes, llegaron %d", len(vistas))
	}

	ana, err := s.DeTitular(ctx, titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(ana) != 1 {
		t.Fatalf("Ana tenia %d ordenes, se esperaba 1", len(ana))
	}
	if !ana[0].Neto.Equal(pgDec("390000")) {
		t.Fatalf("neto ana = %s, se esperaba 390000", ana[0].Neto)
	}
	if len(ana[0].Deducciones) != 3 {
		t.Fatalf("ana: %d deducciones, el desglose tiene que persistirse", len(ana[0].Deducciones))
	}
	if ana[0].Estado != liquidacion.EstadoEnviada {
		t.Fatalf("Estado = %q", ana[0].Estado)
	}
	if ana[0].EnviadaDia != "2026-01-01" {
		t.Fatalf("EnviadaDia = %q", ana[0].EnviadaDia)
	}

	beto, err := s.DeTitular(ctx, titularBeto)
	if err != nil {
		t.Fatalf("DeTitular beto: %v", err)
	}
	if len(beto) != 1 || !beto[0].Neto.Equal(pgDec("260000")) {
		t.Fatalf("beto = %+v", beto)
	}
}

func TestDeTitularListaVaciaSinOrdenes(t *testing.T) {
	s, _ := sembrarCorrida(t, false)

	got, err := s.DeTitular(t.Context(), titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if got == nil {
		t.Fatal("una lista vacia no puede ser nil")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestSilencioALos15DiasPersisteAceptadaPorSilencio(t *testing.T) {
	_, liq := sembrarCorrida(t, true)
	ctx := t.Context()

	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); err != nil {
		t.Fatalf("generar: %v", err)
	}

	actor := aplicacion.Usuario{Rol: aplicacion.RolTitular, TitularID: titularAna}

	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)}
	dia14, err := liq.DeTitular(ctx, actor)
	if err != nil {
		t.Fatalf("dia 14: %v", err)
	}
	if dia14[0].Orden.Estado != liquidacion.EstadoEnviada {
		t.Fatalf("dia 14: %q", dia14[0].Orden.Estado)
	}

	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)}
	dia15, err := liq.DeTitular(ctx, actor)
	if err != nil {
		t.Fatalf("dia 15: %v", err)
	}
	if dia15[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("dia 15: %q", dia15[0].Orden.Estado)
	}
	if !dia15[0].Pagable {
		t.Fatal("Ana tiene RUT y banco: tiene que ser pagable")
	}

	// La transicion quedo escrita: un reloj que volviera atras no la
	// deshace, porque EvaluarPlazo no toca un estado distinto de enviada.
	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)}
	despues, err := liq.DeTitular(ctx, actor)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	if despues[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("la aceptacion por silencio no persistio: %q", despues[0].Orden.Estado)
	}
}

func TestOrdenSinDocumentosNoEsPagableTrasSilencio(t *testing.T) {
	_, liq := sembrarCorrida(t, false)
	ctx := t.Context()

	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); err != nil {
		t.Fatalf("generar: %v", err)
	}

	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)}
	vistas, err := liq.DeTitular(ctx, aplicacion.Usuario{Rol: aplicacion.RolTitular, TitularID: titularAna})
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if vistas[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("Estado = %q", vistas[0].Orden.Estado)
	}
	if vistas[0].Pagable {
		t.Fatal("sin RUT y banco no es pagable (R-12)")
	}
}

func TestSMMLVAusenteEsParametroAusente(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.SMMLVVigente(t.Context(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, aplicacion.ErrParametroAusente) {
		t.Fatalf("se esperaba ErrParametroAusente, se obtuvo %v", err)
	}
}

func TestInsumoDeProcesoNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.InsumoDeProceso(t.Context(), "prc-inexistente")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}
