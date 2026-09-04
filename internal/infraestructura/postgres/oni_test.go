package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

const (
	shaReporte  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	reporteONIA = "rep-oni-a"
	reporteONIB = "rep-oni-b"
	usoONI1     = "uso-oni-1"
	usoONI2     = "uso-oni-2"
	usoResuelto = "uso-resuelto-1"
)

func sembrarUsosONI(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar usos ONI (%s): %v", sql, err)
		}
	}

	ejecutar(`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
	          VALUES ($1, 'caracol', '2026-01', $2, 'obj/rep-a', 100)`,
		reporteONIA, shaReporte)
	ejecutar(`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
	          VALUES ($1, 'netflix', '2025-06', $2, 'obj/rep-b', 80)`,
		reporteONIB, strings.Repeat("b", 64))

	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, escalon, oni, modalidad)
	          VALUES ($1, $2, 'caracol', 'Serie Desconocida', 'ID-99', 'oni', TRUE, 'tv')`,
		usoONI1, reporteONIA)
	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, escalon, oni, modalidad)
	          VALUES ($1, $2, 'caracol', 'Unitario Huerfano', 'ID-100', 'oni', TRUE, 'tv')`,
		usoONI2, reporteONIA)
	// Identificado: no puede salir en el listado publico de este periodo.
	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, escalon, oni, obra_id, modalidad)
	          VALUES ($1, $2, 'caracol', 'La Casa de las Dos Palmas', 'ID-1', 'alias', FALSE, $3, 'tv')`,
		usoResuelto, reporteONIA, obraCompleta)
	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, escalon, oni, modalidad)
	          VALUES ('uso-oni-otro-periodo', $1, 'netflix', 'Show Sin Nombre', 'show-1', 'oni', TRUE, 'ott')`,
		reporteONIB)
}

func publicar(t *testing.T, s *Store, periodo, actor string) aplicacion.PublicacionONI {
	t.Helper()
	uc := aplicacion.PublicarListadoONI{
		ONI:         s,
		Bitacora:    s,
		Tx:          s,
		Reloj:       reloj.Fijo{Instante: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)},
		Fisica:      "Calle 74 #7-35, Bogota D.C.",
		Electronica: "oni@redescritores.com",
	}
	pub, err := uc.Ejecutar(t.Context(), periodo, actor)
	if err != nil {
		t.Fatalf("PublicarListadoONI(%s): %v", periodo, err)
	}
	return pub
}

func TestPublicarListadoONIPersisteVistaAsientoYAncla(t *testing.T) {
	s, pool := sembrar(t)
	sembrarUsosONI(t, pool)
	ctx := t.Context()

	pub := publicar(t, s, "2026-01", usuarioAdmin)

	if pub.Periodo != "2026-01" {
		t.Fatalf("Periodo = %q", pub.Periodo)
	}
	if pub.DireccionFisica == "" || pub.DireccionElectronica == "" {
		t.Fatal("faltan las direcciones de RD 13.8.4.3")
	}
	if len(pub.Obras) != 2 {
		t.Fatalf("Obras = %d, se esperaban 2 (el resuelto no cuenta)", len(pub.Obras))
	}

	var nVista int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM oni_publico WHERE periodo = '2026-01'`).Scan(&nVista); err != nil {
		t.Fatalf("contar oni_publico: %v", err)
	}
	if nVista != 2 {
		t.Fatalf("oni_publico tiene %d filas, se esperaban 2", nVista)
	}

	var titulos []string
	filas, err := pool.Query(ctx, `SELECT id, titulo FROM oni_publico WHERE periodo = '2026-01' ORDER BY titulo`)
	if err != nil {
		t.Fatalf("leer oni_publico: %v", err)
	}
	defer filas.Close()
	for filas.Next() {
		var id, titulo string
		if err := filas.Scan(&id, &titulo); err != nil {
			t.Fatalf("escanear: %v", err)
		}
		titulos = append(titulos, titulo)
		if id == usoResuelto {
			t.Fatal("un uso identificado no puede salir en oni_publico")
		}
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("oni_publico: %v", err)
	}
	if strings.Join(titulos, ",") != "Serie Desconocida,Unitario Huerfano" {
		t.Fatalf("titulos = %v", titulos)
	}

	asientos, err := s.De(ctx, aplicacion.RefTipoPublicacionONI, pub.ID)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("asientos = %d, se esperaba 1", len(asientos))
	}
	if asientos[0].Hecho != aplicacion.HechoListadoONIPublicado {
		t.Fatalf("Hecho = %q", asientos[0].Hecho)
	}
	if asientos[0].ActorID != usuarioAdmin {
		t.Fatalf("ActorID = %q", asientos[0].ActorID)
	}
	cuerpo := strings.ToLower(string(asientos[0].Payload))
	for _, p := range []string{"monto", "importe", "bruto", "neto"} {
		if strings.Contains(cuerpo, p) {
			t.Fatalf("el asiento menciona %q: %s", p, asientos[0].Payload)
		}
	}

	var ancla *time.Time
	if err := pool.QueryRow(ctx, `SELECT publicado_en FROM usos WHERE id = $1`, usoONI1).Scan(&ancla); err != nil {
		t.Fatalf("publicado_en: %v", err)
	}
	if ancla == nil {
		t.Fatal("no se anclo la prescripcion")
	}
}

func TestOniPublicoNoExponeColumnasDeDinero(t *testing.T) {
	_, pool := sembrar(t)
	ctx := t.Context()

	filas, err := pool.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'oni_publico'`)
	if err != nil {
		t.Fatalf("columnas de oni_publico: %v", err)
	}
	defer filas.Close()

	prohibidos := []string{"monto", "importe", "bruto", "neto", "valor", "taquilla", "puntos", "bolsa"}
	var columnas []string
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			t.Fatalf("escanear columna: %v", err)
		}
		columnas = append(columnas, nombre)
		bajo := strings.ToLower(nombre)
		for _, p := range prohibidos {
			if strings.Contains(bajo, p) {
				t.Fatalf("oni_publico.%s parece dinero; R-18 lo prohibe", nombre)
			}
		}
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("columnas: %v", err)
	}
	if len(columnas) == 0 {
		t.Fatal("oni_publico no tiene columnas")
	}
}

func TestNoSePuedeRepublicarElMismoPeriodo(t *testing.T) {
	s, pool := sembrar(t)
	sembrarUsosONI(t, pool)

	publicar(t, s, "2026-01", usuarioAdmin)
	uc := aplicacion.PublicarListadoONI{
		ONI: s, Bitacora: s, Tx: s,
		Reloj:       reloj.Fijo{Instante: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		Fisica:      "Calle 74 #7-35, Bogota D.C.",
		Electronica: "oni@redescritores.com",
	}
	_, err := uc.Ejecutar(t.Context(), "2026-01", usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrYaPublicado) {
		t.Fatalf("se esperaba ErrYaPublicado, se obtuvo %v", err)
	}
}

func TestElAnclaDePrescripcionNoSeReescribe(t *testing.T) {
	s, pool := sembrar(t)
	sembrarUsosONI(t, pool)
	publicar(t, s, "2026-01", usuarioAdmin)

	_, err := pool.Exec(t.Context(),
		`UPDATE usos SET publicado_en = $1 WHERE id = $2`,
		time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC), usoONI1)
	if err == nil {
		t.Fatal("reescribir publicado_en tiene que fallar: es el ancla de R-19")
	}
}

func TestPublicacionVigenteEsLaUltima(t *testing.T) {
	s, pool := sembrar(t)
	sembrarUsosONI(t, pool)

	publicar(t, s, "2025-06", usuarioAdmin)
	segunda := publicar(t, s, "2026-01", usuarioAdmin)

	vigente, err := s.PublicacionVigente(t.Context())
	if err != nil {
		t.Fatalf("PublicacionVigente: %v", err)
	}
	if vigente.ID != segunda.ID {
		t.Fatalf("vigente = %q, se esperaba %q", vigente.ID, segunda.ID)
	}
	if vigente.Periodo != "2026-01" {
		t.Fatalf("Periodo = %q", vigente.Periodo)
	}
}

func TestConsultarPeriodoSinPublicacionEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.PublicacionDePeriodo(t.Context(), "2020")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestPublicarPeriodoSinONIDejaListadoVacioYAsiento(t *testing.T) {
	s, _ := sembrar(t)

	pub := publicar(t, s, "2024", usuarioAdmin)
	if len(pub.Obras) != 0 {
		t.Fatalf("sin ONI se esperaba listado vacio, llegaron %d", len(pub.Obras))
	}
	asientos, err := s.De(t.Context(), aplicacion.RefTipoPublicacionONI, pub.ID)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("aun vacio tiene que dejar asiento: %d", len(asientos))
	}
}
