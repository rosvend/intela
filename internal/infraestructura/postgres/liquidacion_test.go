package postgres

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestDeTitularDevuelveNetoYTotalesDelProceso(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularAna, "2026-01")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %d, se esperaba 1", len(filas))
	}
	f := filas[0]
	if f.ObraID != obraCompleta || f.Periodo != "2026-01" {
		t.Fatalf("fila = %+v", f)
	}
	if !f.Neto.Equal(decimal.RequireFromString("3900")) {
		t.Fatalf("neto = %s", f.Neto)
	}
	if !f.ProcesoAdmin.Equal(decimal.RequireFromString("2000")) {
		t.Fatalf("admin proceso = %s", f.ProcesoAdmin)
	}
}

func TestDeTitularRespetaElPeriodo(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularAna, "2025-06")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 0 {
		t.Fatalf("un periodo sin corridas no es error, es lista vacia: %+v", filas)
	}
}

func TestDeTitularSinFiltroTraeTodosLosPeriodos(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)
	sembrarOtraCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularAna, "")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d, se esperaban 2 periodos", len(filas))
	}
}

func TestDeTitularNoMezclaTitulares(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	filas, err := s.DeTitular(t.Context(), titularBeto, "2026-01")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("beto tiene su propia linea: %d", len(filas))
	}
	if !filas[0].Neto.Equal(decimal.RequireFromString("2600")) {
		t.Fatalf("neto de beto = %s", filas[0].Neto)
	}
}

func TestDeTitularSinCorridasEsListaVacia(t *testing.T) {
	s, _ := sembrar(t)

	filas, err := s.DeTitular(t.Context(), titularAna, "")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(filas) != 0 {
		t.Fatalf("sin corridas: %+v", filas)
	}
}

// Ana y Beto, cada uno por su lado, tienen que recibir porciones de Admin
// cuya suma es exactamente el admin del proceso (no 1999.98 por redondeos
// independientes).
func TestDeTitularNetosProcesoPermitenReconciliarDeducciones(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorrida(t, s)

	ana, err := s.DeTitular(t.Context(), titularAna, "2026-01")
	if err != nil {
		t.Fatalf("ana: %v", err)
	}
	beto, err := s.DeTitular(t.Context(), titularBeto, "2026-01")
	if err != nil {
		t.Fatalf("beto: %v", err)
	}
	if len(ana) != 1 || len(beto) != 1 {
		t.Fatalf("ana=%d beto=%d", len(ana), len(beto))
	}
	if len(ana[0].NetosProceso) != 2 {
		t.Fatalf("netos del proceso = %d, se esperaban Ana+Beto", len(ana[0].NetosProceso))
	}
	if ana[0].Indice == beto[0].Indice {
		t.Fatal("Ana y Beto no pueden compartir indice en el vector del proceso")
	}

	lineas := liquidacion.ProrratearProceso(
		ana[0].NetosProceso, ana[0].ProcesoAdmin, ana[0].ProcesoSocial, ana[0].ProcesoReserva,
	)
	sumaAdmin := lineas[ana[0].Indice].Admin.Add(lineas[beto[0].Indice].Admin)
	if !sumaAdmin.Equal(ana[0].ProcesoAdmin) {
		t.Fatalf("Σ admin Ana+Beto = %s, proceso = %s", sumaAdmin, ana[0].ProcesoAdmin)
	}
}

// Con retenido en el proceso, DeTitular + Consultar no deben inflar las
// deducciones de Ana con la parte de la obra retenida (Bloqueante 1 / RD 16).
func TestConsultarConRetenidoNoInflaDeducciones(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar retenido: %v", err)
		}
	}
	ejecutar(`INSERT INTO usuarios_recaudo (id, nombre, categoria)
	          VALUES ('usr-canal', 'Canal de prueba', 'tv_abierta')
	          ON CONFLICT (id) DO NOTHING`)
	ejecutar(`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
	          VALUES ('bolsa-ret', 'usr-canal', '2026-03', 'nacional', 10000)`)
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
	          VALUES ('proc-ret', 'nacional', 'liquidacion_final', '2026-03', 'bolsa-ret', 'snap-1', 'RD IX')`)

	r := reparto.Resultado{
		Neto:       decimal.RequireFromString("6500"),
		Admin:      decimal.RequireFromString("2000"),
		Social:     decimal.RequireFromString("1000"),
		Reserva:    decimal.RequireFromString("500"),
		Retenido:   decimal.RequireFromString("2600"),
		Residuo:    decimal.Zero,
		ValorPunto: decimal.RequireFromString("1"),
		SnapshotID: "snap-1",
		Reglamento: "RD IX",
		Obras: []reparto.LineaObra{
			{ObraID: obraCompleta, Puntos: decimal.RequireFromString("60"), Importe: decimal.RequireFromString("3900")},
			{ObraID: obraIncompleta, Puntos: decimal.RequireFromString("40"), Importe: decimal.RequireFromString("2600"), Retenida: true, Motivo: "declaracion_incompleta"},
		},
		Titulares: []reparto.LineaTitular{
			{ObraID: obraCompleta, TitularID: titularAna, IPI: "IPI-00000001", Porcentaje: decimal.RequireFromString("100"), Importe: decimal.RequireFromString("3900")},
		},
	}
	if err := s.GuardarResultado(ctx, "proc-ret", r); err != nil {
		t.Fatalf("GuardarResultado: %v", err)
	}

	svc := aplicacion.ServicioLiquidacion{Repo: s}
	liq, err := svc.Consultar(ctx, aplicacion.Usuario{
		ID: "usr-ana", Rol: aplicacion.RolTitular, TitularID: titularAna,
	}, "2026-03")
	if err != nil {
		t.Fatalf("Consultar: %v", err)
	}
	if len(liq.Lineas) != 1 {
		t.Fatalf("lineas = %d", len(liq.Lineas))
	}
	l := liq.Lineas[0]
	if !l.Admin.Equal(decimal.RequireFromString("1200")) ||
		!l.Social.Equal(decimal.RequireFromString("600")) ||
		!l.Reserva.Equal(decimal.RequireFromString("300")) ||
		!l.Bruto.Equal(decimal.RequireFromString("6000")) ||
		!l.Neto.Equal(decimal.RequireFromString("3900")) {
		t.Fatalf("Ana con retenido quedo inflada: %+v (admin proporcional=1200)", l)
	}
}

func sembrarCorrida(t *testing.T, s *Store) {
	t.Helper()
	sembrarProceso(t, s, "bolsa-1", "proc-1", "2026-01",
		"10000", "2000", "1000", "500", "6500", "3900", "2600")
}

func sembrarOtraCorrida(t *testing.T, s *Store) {
	t.Helper()
	sembrarProceso(t, s, "bolsa-2", "proc-2", "2026-02",
		"2000", "400", "200", "100", "1300", "780", "520")
}

func sembrarProceso(t *testing.T, s *Store, bolsaID, procesoID, periodo, bruto, admin, social, reserva, neto, anaNeto, betoNeto string) {
	t.Helper()
	ctx := t.Context()
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar proceso (%s): %v", sql, err)
		}
	}

	// Desde 00009, `bolsas.usuario_id` apunta a `usuarios_recaudo`.
	// ON CONFLICT: este helper se llama dos veces en el mismo test.
	ejecutar(`INSERT INTO usuarios_recaudo (id, nombre, categoria)
	          VALUES ('usr-canal', 'Canal de prueba', 'tv_abierta')
	          ON CONFLICT (id) DO NOTHING`)
	ejecutar(`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
	          VALUES ($1, 'usr-canal', $2, 'nacional', $3)`,
		bolsaID, periodo, bruto)
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
	          VALUES ($1, 'nacional', 'liquidacion_final', $2, $3, 'snap-1', 'RD IX')`,
		procesoID, periodo, bolsaID)
	ejecutar(`INSERT INTO resultados_proceso
	            (proceso_id, bruto, admin, social, reserva, neto, snapshot_id, reglamento)
	          VALUES ($1, $2, $3, $4, $5, $6, 'snap-1', 'RD IX')`,
		procesoID, bruto, admin, social, reserva, neto)
	ejecutar(`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
	          VALUES ($1, $2, 100, $3, FALSE)`,
		procesoID, obraCompleta, neto)
	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
	          VALUES ($1, $2, $3, 'IPI-00000001', 60, $4),
	                 ($1, $2, $5, 'IPI-00000002', 40, $6)`,
		procesoID, obraCompleta, titularAna, anaNeto, titularBeto, betoNeto)
}
