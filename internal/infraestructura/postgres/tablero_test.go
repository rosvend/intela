package postgres

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

func TestTableroObrasDeclaradasDeTraeLaDeclaracionEntera(t *testing.T) {
	s, _ := sembrar(t)

	obras, err := s.ObrasDeclaradasDe(t.Context(), titularBeto)
	if err != nil {
		t.Fatalf("ObrasDeclaradasDe: %v", err)
	}
	// Beto declara en obraCompleta y obraSinIPI; la parte de Ana tiene que venir tambien.
	if len(obras) != 2 || obras[0].ID != obraCompleta || obras[1].ID != obraSinIPI {
		t.Fatalf("obras = %+v", obras)
	}
	if obras[0].Titulo != "La Casa de las Dos Palmas" || len(obras[0].Declaracion.Partes) != 2 {
		t.Fatalf("obra completa = %+v", obras[0])
	}
	if obras[0].Declaracion.Estado() != "completa" || obras[1].Declaracion.Estado() != "incompleta" {
		t.Fatalf("estados = %s, %s", obras[0].Declaracion.Estado(), obras[1].Declaracion.Estado())
	}
}

// Una version cerrada de la declaracion no cuenta: sin clausulaVigente, Ana apareceria en obraIncompleta.
func TestTableroObrasDeclaradasDeIgnoraVersionesCerradas(t *testing.T) {
	s, _ := sembrar(t)
	ejecutarEn(t, s, `UPDATE declaracion_versiones SET vigente_desde = now() - interval '2 days',
	                         vigente_hasta = now() - interval '1 day'
	                   WHERE obra_id = $1 AND version = 1`, obraCompleta)
	ejecutarEn(t, s, `INSERT INTO declaracion_versiones (obra_id, version, vigente_desde)
	                  VALUES ($1, 2, now() - interval '1 day')`, obraCompleta)
	ejecutarEn(t, s, `INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje, version)
	                  VALUES ($1, $2, 'IPI-00000002', 100, 2)`, obraCompleta, titularBeto)

	obras, err := s.ObrasDeclaradasDe(t.Context(), titularAna)
	if err != nil {
		t.Fatalf("ObrasDeclaradasDe: %v", err)
	}
	for _, o := range obras {
		if o.ID == obraCompleta {
			t.Fatalf("Ana solo esta en la version cerrada de %s y aparecio: %+v", obraCompleta, obras)
		}
	}
	beto, err := s.ObrasDeclaradasDe(t.Context(), titularBeto)
	if err != nil {
		t.Fatalf("ObrasDeclaradasDe: %v", err)
	}
	if len(beto) == 0 || beto[0].ID != obraCompleta || len(beto[0].Declaracion.Partes) != 1 {
		t.Fatalf("obras de Beto = %+v, se esperaba la version 2 con una sola parte", beto)
	}
}

func TestTableroObrasDeclaradasDeSinPartesEsVacia(t *testing.T) {
	s, _ := sembrar(t)
	obras, err := s.ObrasDeclaradasDe(t.Context(), "tit-nadie")
	if err != nil || len(obras) != 0 {
		t.Fatalf("obras = %+v, err = %v", obras, err)
	}
}

func ejecutarEn(t *testing.T, s *Store, sql string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("sembrar (%s): %v", sql, err)
	}
}

// sembrarSegundaBolsaDelPeriodo pasa bolsa-2 a otro canal y abre proc-3 sobre bolsa-3 en 2026-02 (ADR 0019).
func sembrarSegundaBolsaDelPeriodo(t *testing.T, s *Store) {
	t.Helper()
	sembrarOtraCorrida(t, s)
	ejecutarEn(t, s, `INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('usr-canal-2', 'Otro canal', 'tv_abierta')`)
	ejecutarEn(t, s, `UPDATE bolsas SET usuario_id = 'usr-canal-2' WHERE id = 'bolsa-2'`)
	sembrarProceso(t, s, "bolsa-3", "proc-3", "2026-02",
		"1000", "200", "100", "50", "650", "390", "260")
}

func ultimaLiquidacionDeAna(t *testing.T, s *Store) (aplicacion.ResumenLiquidacion, error) {
	t.Helper()
	ana := aplicacion.Usuario{ID: usuarioTitular, Rol: aplicacion.RolTitular, TitularID: titularAna}
	return aplicacion.Tablero{Repo: s}.UltimaLiquidacion(t.Context(), ana)
}

// emitirOrdenDeAna deja la orden que la liquidacion emite cuando el periodo entero llego a liquidacion_final.
func emitirOrdenDeAna(t *testing.T, s *Store, periodo, neto string, procesos ...string) {
	t.Helper()
	ejecutarEn(t, s, `INSERT INTO ordenes_pago
	   (id, proceso_id, procesos, titular_id, periodo, circuito, bruto, neto, estado, enviada, arrastres)
	 VALUES ('liq-'||$1||'-nacional-'||$2, $3, $4, $2, $1, 'nacional', $5, $5, 'enviada', '2026-03-01', '{}')`,
		periodo, titularAna, procesos[0], procesos, neto)
}

func TestTableroUltimaLiquidacionTomaElUltimoPeriodo(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	sembrarOtraCorrida(t, s)
	emitirOrdenDeAna(t, s, "2026-01", "3900", "proc-1")
	emitirOrdenDeAna(t, s, "2026-02", "780", "proc-2")

	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	if r.Periodo != "2026-02" || r.Obras != 1 || !r.Neto.Equal(decimal.RequireFromString("780")) {
		t.Fatalf("resumen = %+v", r)
	}
}

// Dos bolsas del mismo periodo reparten la misma obra (ADR 0019): la orden agrega las dos y la obra cuenta una vez.
func TestTableroUltimaLiquidacionLaMismaObraEnDosCorridasCuentaUnaVez(t *testing.T) {
	s, _ := sembrar(t)
	sembrarSegundaBolsaDelPeriodo(t, s)
	emitirOrdenDeAna(t, s, "2026-02", "1170", "proc-2", "proc-3")

	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	if r.Periodo != "2026-02" || r.Obras != 1 || !r.Neto.Equal(decimal.RequireFromString("1170")) {
		t.Fatalf("resumen = %+v", r)
	}
}

// El caso de la revision de #215: 2026-02 con proc-2 en liquidacion_final y proc-3 en verificacion
// no tiene ordenes (ADR 0024), asi que no es la ultima liquidacion aunque una linea ya este firmada.
func TestTableroUltimaLiquidacionUnPeriodoAMediasNoCuenta(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	sembrarSegundaBolsaDelPeriodo(t, s)
	ejecutarEn(t, s, `UPDATE procesos SET etapa = 'verificacion' WHERE id = 'proc-3'`)

	if _, err := ultimaLiquidacionDeAna(t, s); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("sin ordenes: err = %v, se esperaba ErrNoEncontrado", err)
	}

	emitirOrdenDeAna(t, s, "2026-01", "3900", "proc-1")
	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	if r.Periodo != "2026-01" || r.Obras != 1 || !r.Neto.Equal(decimal.RequireFromString("3900")) {
		t.Fatalf("resumen = %+v, 2026-02 no esta liquidado", r)
	}
}

// Mismo periodo y mismo neto que /mis-liquidaciones: las dos salen de ordenes_pago.
func TestTableroUltimaLiquidacionCoincideConMisLiquidaciones(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	sembrarOtraCorrida(t, s)
	emitirOrdenDeAna(t, s, "2026-01", "3900", "proc-1")
	emitirOrdenDeAna(t, s, "2026-02", "780", "proc-2")

	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	ordenes, err := s.DeTitular(t.Context(), titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	ultima := ordenes[len(ordenes)-1]
	if ultima.Periodo != r.Periodo || !ultima.Neto.Equal(r.Neto) {
		t.Fatalf("tablero = %+v, /mis-liquidaciones = %s %s", r, ultima.Periodo, ultima.Neto)
	}
}

func TestTableroUltimaLiquidacionSinOrdenesEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	if _, err := ultimaLiquidacionDeAna(t, s); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("err = %v, se esperaba ErrNoEncontrado", err)
	}
}

func TestTableroUltimaCorrida(t *testing.T) {
	s, _ := sembrar(t)
	if _, err := s.UltimaCorrida(t.Context()); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("sin corridas: err = %v", err)
	}
	sembrarCorridaReporte(t, s)
	sembrarOtraCorrida(t, s)

	p, err := s.UltimaCorrida(t.Context())
	if err != nil {
		t.Fatalf("UltimaCorrida: %v", err)
	}
	if p.Periodo != "2026-02" || p.Etapa != reparto.EtapaLiquidacionFinal || p.Circuito != reparto.Nacional {
		t.Fatalf("corrida = %+v", p)
	}
}

// A igual periodo gana la abierta despues, aunque su id ordene antes.
func TestTableroUltimaCorridaDesempataPorApertura(t *testing.T) {
	s, _ := sembrar(t)
	sembrarSegundaBolsaDelPeriodo(t, s)
	ejecutarEn(t, s, `UPDATE procesos SET abierto = '2026-03-01T00:00:00Z' WHERE id = 'proc-3'`)
	ejecutarEn(t, s, `UPDATE procesos SET abierto = '2026-03-02T00:00:00Z' WHERE id = 'proc-2'`)

	p, err := s.UltimaCorrida(t.Context())
	if err != nil {
		t.Fatalf("UltimaCorrida: %v", err)
	}
	if p.ID != "proc-2" {
		t.Fatalf("corrida = %+v, se esperaba proc-2 (abierta despues)", p)
	}
}

// R-04: obra-incompleta, obra-sin-ipi y obra-sin-declaracion se retienen; las tres cuentan.
func TestTableroObrasEnReservaCuentaLasNoDeclaradas(t *testing.T) {
	s, _ := sembrar(t)
	n, err := aplicacion.Tablero{Repo: s}.ObrasEnReserva(t.Context())
	if err != nil || n != 3 {
		t.Fatalf("obras en reserva = %d, err = %v, se esperaban 3", n, err)
	}
}

// El conteo en SQL tiene que decir lo mismo que repertorio.Declaracion.Completa() obra a obra,
// tambien con una version cerrada incompleta bajo una vigente completa, y tres partes que suman 100.
func TestTableroObrasEnReservaCoincideConCompleta(t *testing.T) {
	s, _ := sembrar(t)
	ejecutarEn(t, s, `INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES
	                    ('obra-tres', 'Tres Partes', 'Drama', 2001, 'serie'),
	                    ('obra-reabierta', 'Reabierta', 'Drama', 2002, 'serie')`)
	ejecutarEn(t, s, `INSERT INTO declaracion_versiones (obra_id, version, vigente_desde, vigente_hasta) VALUES
	                    ('obra-tres', 1, now(), NULL),
	                    ('obra-reabierta', 1, now() - interval '2 days', now() - interval '1 day'),
	                    ('obra-reabierta', 2, now() - interval '1 day', NULL)`)
	ejecutarEn(t, s, `INSERT INTO declaraciones (obra_id, titular_id, ipi, porcentaje, version) VALUES
	                    ('obra-tres', $1, 'IPI-00000001', 33.3333, 1),
	                    ('obra-tres', $2, 'IPI-00000002', 33.3333, 1),
	                    ('obra-tres', $3, 'IPI-00000003', 33.3334, 1),
	                    ('obra-reabierta', $1, 'IPI-00000001', 50, 1),
	                    ('obra-reabierta', $1, 'IPI-00000001', 100, 2)`, titularAna, titularBeto, titularCarla(t, s))

	obras, err := s.ListarObras(t.Context(), aplicacion.Paginacion{Limite: aplicacion.LimiteSinTope})
	if err != nil {
		t.Fatalf("ListarObras: %v", err)
	}
	decls, err := s.Declaraciones(t.Context())
	if err != nil {
		t.Fatalf("Declaraciones: %v", err)
	}
	quiere := 0
	for _, o := range obras {
		if !decls[o.ID].Completa() {
			quiere++
		}
	}
	n, err := s.ContarObrasEnReserva(t.Context())
	if err != nil {
		t.Fatalf("ContarObrasEnReserva: %v", err)
	}
	if n != quiere || n != 3 {
		t.Fatalf("SQL = %d, Completa() = %d, se esperaban 3", n, quiere)
	}
}

// titularCarla da de alta un tercer titular para la declaracion de tres partes.
func titularCarla(t *testing.T, s *Store) string {
	t.Helper()
	ejecutarEn(t, s, `INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
	                  VALUES ('tit-carla', 'Carla Guionista', 'IPI-00000003', TRUE, 'socio')`)
	return "tit-carla"
}

func TestTableroConteosDeUsos(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar (%s): %v", sql, err)
		}
	}
	sha := func(c string) string {
		b := make([]byte, 64)
		for i := range b {
			b[i] = c[0]
		}
		return string(b)
	}
	ejecutar(`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes) VALUES
	            ('rep-1', 'caracol', '2026-01', $1, 'k1', 10),
	            ('rep-2', 'rcn',     '2026-01', $2, 'k2', 10),
	            ('rep-3', 'rcn',     '2026-02', $3, 'k3', 10)`, sha("a"), sha("b"), sha("c"))
	// rep-1 tiene dos filas sin procesar; rep-2 una ONI; rep-3 ya identificado.
	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, modalidad, escalon, oni, obra_id) VALUES
	            ('u-1', 'rep-1', 'caracol', 'A', 'tv', 'pendiente', TRUE,  NULL),
	            ('u-2', 'rep-1', 'caracol', 'B', 'tv', 'pendiente', TRUE,  NULL),
	            ('u-3', 'rep-2', 'rcn',     'C', 'tv', 'oni',       TRUE,  NULL),
	            ('u-4', 'rep-3', 'rcn',     'D', 'tv', 'alias',     FALSE, $1)`, obraCompleta)

	if n, err := s.CargasPendientes(ctx); err != nil || n != 1 {
		t.Fatalf("cargas pendientes = %d, err = %v, se esperaba 1", n, err)
	}
	if n, err := s.CasosONIPendientes(ctx); err != nil || n != 1 {
		t.Fatalf("oni pendientes = %d, err = %v, se esperaba 1", n, err)
	}
}
