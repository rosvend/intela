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
	shaReporte    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	reporteONIA   = "rep-oni-a"
	reporteONIB   = "rep-oni-b"
	usoONI1       = "uso-oni-1"
	usoONI2       = "uso-oni-2"
	usoResuelto   = "uso-resuelto-1"
	usoSinCascada = "uso-pendiente-sin-cascada"
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
	// La ingesta siembra oni=TRUE con escalon pendiente, antes de la cascada.
	// Esa fila no es ONI real y no puede congelarse al publicar el periodo.
	ejecutar(`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, escalon, oni, modalidad)
	          VALUES ($1, $2, 'caracol', 'Aun Sin Cascada', 'ID-pend', 'pendiente', TRUE, 'tv')`,
		usoSinCascada, reporteONIA)
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

func TestFilaPendienteConBanderaONINoSeCongela(t *testing.T) {
	s, pool := sembrar(t)
	sembrarUsosONI(t, pool)
	ctx := t.Context()

	pub := publicar(t, s, "2026-01", usuarioAdmin)
	for _, o := range pub.Obras {
		if o.ID == usoSinCascada {
			t.Fatal("un uso con escalon pendiente no es ONI: la cascada aun no corrio")
		}
	}

	var enVista int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM oni_publico WHERE id = $1`, usoSinCascada).Scan(&enVista); err != nil {
		t.Fatalf("oni_publico: %v", err)
	}
	if enVista != 0 {
		t.Fatalf("oni_publico incluye el pendiente: %d", enVista)
	}

	var ancla *time.Time
	if err := pool.QueryRow(ctx, `SELECT publicado_en FROM usos WHERE id = $1`, usoSinCascada).Scan(&ancla); err != nil {
		t.Fatalf("publicado_en: %v", err)
	}
	if ancla != nil {
		t.Fatal("anclar la prescripcion de un pendiente arranca R-19 antes de identificar")
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

func TestPublicacionComplementariaONITardio(t *testing.T) {
	s, pool := sembrar(t)
	sembrarUsosONI(t, pool)
	ctx := t.Context()

	// 1. Primera publicacion de 2026-01.
	t1 := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	uc1 := aplicacion.PublicarListadoONI{
		ONI:         s,
		Bitacora:    s,
		Tx:          s,
		Reloj:       reloj.Fijo{Instante: t1},
		Fisica:      "Calle 74 #7-35, Bogota D.C.",
		Electronica: "oni@redescritores.com",
	}
	pub1, err := uc1.Ejecutar(ctx, "2026-01", usuarioAdmin)
	if err != nil {
		t.Fatalf("primera publicacion: %v", err)
	}
	if pub1.Secuencia != 1 {
		t.Fatalf("Secuencia = %d, se esperaba 1", pub1.Secuencia)
	}
	if len(pub1.Obras) != 2 {
		t.Fatalf("Obras en primera publicacion = %d, se esperaban 2", len(pub1.Obras))
	}

	// 2. Intentar republicar sin nuevos usos ONI falla con ErrYaPublicado.
	_, err = uc1.Ejecutar(ctx, "2026-01", usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrYaPublicado) {
		t.Fatalf("republicar sin pendientes: se esperaba ErrYaPublicado, se obtuvo %v", err)
	}

	// 3. Entra un uso ONI tardio para 2026-01 (publicado_en = NULL).
	usoTardio := "uso-oni-tardio-1"
	if _, err := pool.Exec(ctx, `
		INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, escalon, oni, modalidad)
		VALUES ($1, $2, 'caracol', 'Capitulo Perdido Tardio', 'ID-tardio', 'oni', TRUE, 'tv')`,
		usoTardio, reporteONIA); err != nil {
		t.Fatalf("insertar uso tardio: %v", err)
	}

	// 4. Publicacion complementaria en t2.
	t2 := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	uc2 := aplicacion.PublicarListadoONI{
		ONI:         s,
		Bitacora:    s,
		Tx:          s,
		Reloj:       reloj.Fijo{Instante: t2},
		Fisica:      "Calle 74 #7-35, Bogota D.C.",
		Electronica: "oni@redescritores.com",
	}
	pub2, err := uc2.Ejecutar(ctx, "2026-01", usuarioAdmin)
	if err != nil {
		t.Fatalf("publicacion complementaria: %v", err)
	}
	if pub2.Secuencia != 2 {
		t.Fatalf("Secuencia complementaria = %d, se esperaba 2", pub2.Secuencia)
	}
	if pub2.ID == pub1.ID {
		t.Fatalf("pub2.ID no debe ser igual a pub1.ID")
	}
	if len(pub2.Obras) != 1 || pub2.Obras[0].ID != usoTardio {
		t.Fatalf("Obras en complementaria = %v, se esperaba solo el tardio", pub2.Obras)
	}

	// 5. Verificar anclas en la base de datos:
	// El uso tardio quedo anclado en t2.
	var anclaTardio *time.Time
	if err := pool.QueryRow(ctx, `SELECT publicado_en FROM usos WHERE id = $1`, usoTardio).Scan(&anclaTardio); err != nil {
		t.Fatalf("publicado_en tardio: %v", err)
	}
	if anclaTardio == nil || !anclaTardio.Equal(t2) {
		t.Fatalf("ancla tardio = %v, se esperaba %v", anclaTardio, t2)
	}

	// Los usos iniciales conservan su ancla en t1 (el reloj de R-19 no se resetea).
	var anclaInicial *time.Time
	if err := pool.QueryRow(ctx, `SELECT publicado_en FROM usos WHERE id = $1`, usoONI1).Scan(&anclaInicial); err != nil {
		t.Fatalf("publicado_en inicial: %v", err)
	}
	if anclaInicial == nil || !anclaInicial.Equal(t1) {
		t.Fatalf("ancla inicial = %v, se esperaba %v", anclaInicial, t1)
	}

	// 6. Consultar el periodo devuelve el listado consolidado (3 obras).
	periodoConsolidado, err := s.PublicacionDePeriodo(ctx, "2026-01")
	if err != nil {
		t.Fatalf("PublicacionDePeriodo: %v", err)
	}
	if len(periodoConsolidado.Obras) != 3 {
		t.Fatalf("obras periodo = %d, se esperaban 3 (2 iniciales + 1 tardio)", len(periodoConsolidado.Obras))
	}
	if !periodoConsolidado.FechaProceso.Equal(t2) {
		t.Fatalf("cabecera = %v, se esperaba la complementaria %v", periodoConsolidado.FechaProceso, t2)
	}
	anclaPublica := map[string]time.Time{}
	for _, o := range periodoConsolidado.Obras {
		got, err := time.Parse(time.RFC3339, o.FechaProceso)
		if err != nil {
			t.Fatalf("fecha publica de %s: %v", o.ID, err)
		}
		anclaPublica[o.ID] = got
	}
	if !anclaPublica[usoONI1].Equal(t1) || !anclaPublica[usoONI2].Equal(t1) {
		t.Fatalf("las obras de la secuencia 1 muestran %v y %v, se esperaba %v",
			anclaPublica[usoONI1], anclaPublica[usoONI2], t1)
	}
	if !anclaPublica[usoTardio].Equal(t2) {
		t.Fatalf("el tardio muestra %v, se esperaba %v", anclaPublica[usoTardio], t2)
	}

	// 7. La vista publica oni_publico refleja las 3 obras para 2026-01.
	var nVista int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM oni_publico WHERE periodo = '2026-01'`).Scan(&nVista); err != nil {
		t.Fatalf("contar oni_publico: %v", err)
	}
	if nVista != 3 {
		t.Fatalf("oni_publico = %d filas, se esperaban 3", nVista)
	}
	var fechaVista time.Time
	if err := pool.QueryRow(ctx, `SELECT fecha_proceso FROM oni_publico WHERE id = $1`, usoONI1).Scan(&fechaVista); err != nil {
		t.Fatalf("fecha en oni_publico de %s: %v", usoONI1, err)
	}
	if !fechaVista.Equal(t1) {
		t.Fatalf("oni_publico.fecha_proceso de la secuencia 1 = %v, se esperaba %v", fechaVista, t1)
	}
	if err := pool.QueryRow(ctx, `SELECT fecha_proceso FROM oni_publico WHERE id = $1`, usoTardio).Scan(&fechaVista); err != nil {
		t.Fatalf("fecha en oni_publico del tardio: %v", err)
	}
	if !fechaVista.Equal(t2) {
		t.Fatalf("oni_publico.fecha_proceso del tardio = %v, se esperaba %v", fechaVista, t2)
	}

	// 8. Bitacora contiene 2 asientos de publicacion.
	asientos1, err := s.De(ctx, aplicacion.RefTipoPublicacionONI, pub1.ID)
	if err != nil || len(asientos1) != 1 {
		t.Fatalf("asientos pub1: %v (len=%d)", err, len(asientos1))
	}
	asientos2, err := s.De(ctx, aplicacion.RefTipoPublicacionONI, pub2.ID)
	if err != nil || len(asientos2) != 1 {
		t.Fatalf("asientos pub2: %v (len=%d)", err, len(asientos2))
	}

	// 9. Intentar publicar de nuevo sin mas pendientes falla con ErrYaPublicado.
	_, err = uc2.Ejecutar(ctx, "2026-01", usuarioAdmin)
	if !errors.Is(err, aplicacion.ErrYaPublicado) {
		t.Fatalf("republicar sin pendientes: se esperaba ErrYaPublicado, se obtuvo %v", err)
	}
}
