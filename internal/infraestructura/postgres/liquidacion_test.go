package postgres

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

const (
	sha64 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	procEnero   = "proc-2026-01"
	procFebrero = "proc-2026-02"
	bolsaEnero  = "bolsa-2026-01"
	bolsaFeb    = "bolsa-2026-02"
	rptCaracol  = "rpt-caracol-2026-01"
	rptNetflix  = "rpt-netflix-2026-02"
	obraBeto    = "obra-beto"
	obraAna2    = "obra-ana-2"
)

// sembrarCorridas deja dos periodos y dos titulares con cifras distintas.
// Ana cobra en obraCompleta (caracol 2026-01 y netflix 2026-02) y en
// obraAna2 (caracol 2026-01). Beto cobra en obraCompleta y en una obra
// donde Ana no figura: esa es la que prueba OE-6.
func sembrarCorridas(t *testing.T) *Store {
	t.Helper()
	s, pool := sembrar(t)
	ctx := t.Context()

	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar corrida (%s): %v", sql, err)
		}
	}

	ejecutar(`INSERT INTO obras (id, titulo, tipo) VALUES
	            ($1, 'Solo de Beto', 'unitario'),
	            ($2, 'El Segundo Guion', 'serie')`,
		obraBeto, obraAna2)
	ejecutar(`INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje) VALUES
	            ($1, $2, 'IPI-00000002', 100.0000),
	            ($3, $4, 'IPI-00000001', 100.0000)`,
		obraBeto, titularBeto, obraAna2, titularAna)

	ejecutar(`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto, convenio, tarifa, factura) VALUES
	            ($1, 'usr-caracol', '2026-01', 'nacional', 10000.00, 'conv-1', 'T-01', 'FAC-1'),
	            ($2, 'usr-netflix', '2026-02', 'nacional',  2000.00, 'conv-2', 'T-08', 'FAC-2')`,
		bolsaEnero, bolsaFeb)
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento) VALUES
	            ($1, 'nacional', 'liquidacion_final', '2026-01', $3, 'snap-2026-01', 'RD-IX'),
	            ($2, 'nacional', 'liquidacion_final', '2026-02', $4, 'snap-2026-02', 'RD-IX')`,
		procEnero, procFebrero, bolsaEnero, bolsaFeb)

	// 10000 - 1000 - 500 - 1000 = 7500. Cierra el CHECK de resultados_proceso.
	ejecutar(`INSERT INTO resultados_proceso
	            (proceso_id, bruto, admin, social, reserva, neto, retenido, residuo, valor_punto, snapshot_id, reglamento)
	          VALUES
	            ($1, 10000.00, 1000.00, 500.00, 1000.00, 7500.00, 0, 0, 1, 'snap-2026-01', 'RD-IX'),
	            ($2,  2000.00,  200.00, 100.00,  200.00, 1500.00, 0, 0, 1, 'snap-2026-02', 'RD-IX')`,
		procEnero, procFebrero)

	ejecutar(`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida) VALUES
	            ($1, $3, 80, 6000.00, FALSE),
	            ($1, $4, 10,  750.00, FALSE),
	            ($1, $5, 10,  750.00, FALSE),
	            ($2, $3, 100, 1500.00, FALSE)`,
		procEnero, procFebrero, obraCompleta, obraAna2, obraBeto)

	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe) VALUES
	            ($1, $3, $5, 'IPI-00000001', 60.0000, 3600.00),
	            ($1, $3, $6, 'IPI-00000002', 40.0000, 2400.00),
	            ($1, $4, $5, 'IPI-00000001', 100.0000, 750.00),
	            ($1, $7, $6, 'IPI-00000002', 100.0000, 750.00),
	            ($2, $3, $5, 'IPI-00000001', 60.0000, 900.00)`,
		procEnero, procFebrero, obraCompleta, obraAna2, titularAna, titularBeto, obraBeto)

	ejecutar(`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes) VALUES
	            ($1, 'caracol', '2026-01', $3, 'obj/rpt-1', 128),
	            ($2, 'netflix', '2026-02', $4, 'obj/rpt-2', 64)`,
		rptCaracol, rptNetflix, sha64, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, obra_id, escalon, evidencia, puntaje, oni, modalidad, tipo_obra) VALUES
	            ('uso-1', $1, 'caracol', 'La Casa de las Dos Palmas', $3, 'alias',  'alias conocido', 1.00000, FALSE, 'tv',  'serie'),
	            ('uso-2', $1, 'caracol', 'El Segundo Guion',          $4, 'difuso', 'similitud',      0.87000, FALSE, 'tv',  'serie'),
	            ('uso-3', $2, 'netflix', 'La Casa de las Dos Palmas', $3, 'id_global', 'IDA',         1.00000, FALSE, 'ott', 'serie')`,
		rptCaracol, rptNetflix, obraCompleta, obraAna2)

	return s
}

func TestIngresosDeSoloLasDelTitular(t *testing.T) {
	s := sembrarCorridas(t)

	filas, err := s.IngresosDe(t.Context(), titularAna, aplicacion.FiltroIngresos{})
	if err != nil {
		t.Fatalf("IngresosDe: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("Ana tiene que ver 3 lineas, vio %d: %+v", len(filas), filas)
	}
	for _, f := range filas {
		if f.ObraID == obraBeto {
			t.Fatal("Ana no puede ver la obra donde no figura")
		}
		_, _, tit, err := aplicacion.ParsearRef(f.Ref)
		if err != nil {
			t.Fatalf("ref %q: %v", f.Ref, err)
		}
		if tit != titularAna {
			t.Fatalf("ref de otro titular: %q", f.Ref)
		}
	}
}

func TestIngresosDeBetoNoIncluyeLasDeAna(t *testing.T) {
	s := sembrarCorridas(t)

	filas, err := s.IngresosDe(t.Context(), titularBeto, aplicacion.FiltroIngresos{})
	if err != nil {
		t.Fatalf("IngresosDe: %v", err)
	}
	for _, f := range filas {
		if f.ObraID == obraAna2 {
			t.Fatal("Beto no participa en obra-ana-2")
		}
		_, _, tit, _ := aplicacion.ParsearRef(f.Ref)
		if tit != titularBeto {
			t.Fatalf("linea de otro: %q", f.Ref)
		}
	}
}

func TestIngresosDeFiltroObra(t *testing.T) {
	s := sembrarCorridas(t)

	filas, err := s.IngresosDe(t.Context(), titularAna, aplicacion.FiltroIngresos{ObraID: obraAna2})
	if err != nil {
		t.Fatalf("IngresosDe: %v", err)
	}
	if len(filas) != 1 || filas[0].ObraID != obraAna2 {
		t.Fatalf("filtro obra: %+v", filas)
	}
}

func TestIngresosDeFiltroPeriodo(t *testing.T) {
	s := sembrarCorridas(t)

	filas, err := s.IngresosDe(t.Context(), titularAna, aplicacion.FiltroIngresos{Periodo: "2026-02"})
	if err != nil {
		t.Fatalf("IngresosDe: %v", err)
	}
	if len(filas) != 1 || filas[0].Periodo != "2026-02" {
		t.Fatalf("filtro periodo: %+v", filas)
	}
}

func TestIngresosDeFiltroFuente(t *testing.T) {
	s := sembrarCorridas(t)

	filas, err := s.IngresosDe(t.Context(), titularAna, aplicacion.FiltroIngresos{Fuente: "netflix"})
	if err != nil {
		t.Fatalf("IngresosDe: %v", err)
	}
	if len(filas) != 1 || filas[0].Fuente != "netflix" {
		t.Fatalf("filtro fuente: %+v", filas)
	}
}

func TestIngresosDeElMontoEsNeto(t *testing.T) {
	s := sembrarCorridas(t)

	filas, err := s.IngresosDe(t.Context(), titularAna, aplicacion.FiltroIngresos{
		ObraID: obraCompleta, Periodo: "2026-01",
	})
	if err != nil {
		t.Fatalf("IngresosDe: %v", err)
	}
	if len(filas) != 1 {
		t.Fatalf("filas = %+v", filas)
	}
	if !filas[0].Neto.Equal(decimal.RequireFromString("3600.00")) {
		t.Fatalf("neto = %s, se esperaba 3600.00 (importe de resultados_titular, no el bruto)", filas[0].Neto)
	}
}

func TestPorLineaLinajeCompleto(t *testing.T) {
	s := sembrarCorridas(t)

	x, err := s.PorLinea(t.Context(), procEnero, obraCompleta, titularAna)
	if err != nil {
		t.Fatalf("PorLinea: %v", err)
	}
	if x.Ref != aplicacion.FormarRef(procEnero, obraCompleta, titularAna) {
		t.Fatalf("ref = %q", x.Ref)
	}
	if x.TitularID != titularAna {
		t.Fatalf("TitularID = %q", x.TitularID)
	}
	if !x.Neto.Equal(decimal.RequireFromString("3600.00")) {
		t.Fatalf("neto = %s", x.Neto)
	}
	// 3600/7500 * 10000 = 4800. El bruto del titular es proporcional, no
	// la bolsa entera, y solo existe aqui.
	if !x.Bruto.Equal(decimal.RequireFromString("4800.00")) {
		t.Fatalf("bruto = %s, se esperaba 4800.00", x.Bruto)
	}
	if x.Corrida.ProcesoID != procEnero || x.Corrida.Periodo != "2026-01" {
		t.Fatalf("corrida = %+v", x.Corrida)
	}
	if x.Reporte.ID != rptCaracol || x.Reporte.Fuente != "caracol" {
		t.Fatalf("reporte = %+v", x.Reporte)
	}
	if x.Obra.Escalon != "alias" || !x.Obra.Puntaje.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("obra = %+v", x.Obra)
	}
	if x.Regla.SnapshotID != "snap-2026-01" || x.Regla.Reglamento != "RD-IX" {
		t.Fatalf("regla = %+v", x.Regla)
	}
	if x.Split.Version != 1 || !x.Split.Porcentaje.Equal(decimal.RequireFromString("60")) {
		t.Fatalf("split = %+v", x.Split)
	}
	if len(x.Deducciones) != 3 {
		t.Fatalf("deducciones = %+v", x.Deducciones)
	}
	if x.Deducciones[0].Concepto != "gastos administrativos" {
		t.Fatalf("concepto = %q", x.Deducciones[0].Concepto)
	}
}

func TestPorLineaNoEncontrada(t *testing.T) {
	s := sembrarCorridas(t)

	_, err := s.PorLinea(t.Context(), "proc-no", obraCompleta, titularAna)
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}
