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

func TestTableroUltimaLiquidacionTomaElUltimoPeriodo(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	sembrarOtraCorrida(t, s)

	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	if r.Periodo != "2026-02" || r.Obras != 1 || !r.Neto.Equal(decimal.RequireFromString("780")) {
		t.Fatalf("resumen = %+v", r)
	}
}

// Dos bolsas del mismo periodo reparten la misma obra (ADR 0019): suma las dos, cuenta la obra una vez.
func TestTableroUltimaLiquidacionLaMismaObraEnDosCorridasCuentaUnaVez(t *testing.T) {
	s, _ := sembrar(t)
	sembrarSegundaBolsaDelPeriodo(t, s)

	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	if r.Periodo != "2026-02" || r.Obras != 1 || !r.Neto.Equal(decimal.RequireFromString("1170")) {
		t.Fatalf("resumen = %+v", r)
	}
}

func TestTableroUltimaLiquidacionIgnoraCorridasSinFirmar(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	sembrarProceso(t, s, "bolsa-3", "proc-3", "2026-03",
		"1000", "200", "100", "50", "650", "390", "260")
	ejecutarEn(t, s, `UPDATE procesos SET etapa = 'importe_titular' WHERE id = 'proc-3'`)

	r, err := ultimaLiquidacionDeAna(t, s)
	if err != nil {
		t.Fatalf("UltimaLiquidacion: %v", err)
	}
	if r.Periodo != "2026-01" || !r.Neto.Equal(decimal.RequireFromString("3900")) {
		t.Fatalf("resumen = %+v, la corrida en importe_titular no debia contar", r)
	}
}

func TestTableroUltimaLiquidacionSinLineasEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	sembrarCorridaReporte(t, s)
	nadie := aplicacion.Usuario{ID: "usr-x", Rol: aplicacion.RolTitular, TitularID: "tit-nadie"}
	if _, err := (aplicacion.Tablero{Repo: s}).UltimaLiquidacion(t.Context(), nadie); !errors.Is(err, aplicacion.ErrNoEncontrado) {
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
