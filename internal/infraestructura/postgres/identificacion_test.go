package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// Fixtures de identificacion_test.go: un catalogo minimo para el escalon 2 y
// un reporte con tres usos pendientes para sembrar alias y matches.
//
// Vive aparte de sembrar() (semilla_test.go): esa fixture es de afiliacion y
// repertorio y crece con esos issues; esta es solo lo que el escalon 1-2 y
// GuardarMatch necesitan.
const (
	obraImdb   = "obra-45" // imdb = tt0100001
	obraIda    = "obra-12" // ida  = IDA-000001
	obraVacia  = "obra-77" // ida, eidr, imdb los tres vacios
	reporteUno = "rep-1"
)

// nuevaObraGlobal construye una obra de catalogo valida con identificadores
// globales ajustables. Aparte de nuevaObra (catalogo_test.go): esa prueba el
// CRUD del catalogo, esta solo alimenta el escalon 2 de la cascada.
func nuevaObraGlobal(t *testing.T, id string, ajustar ...func(*repertorio.Metadatos)) repertorio.Obra {
	t.Helper()

	m := repertorio.Metadatos{
		Titulo: "Obra de catalogo " + id,
		Genero: "Drama",
		Anio:   2000,
		Tipo:   repertorio.TipoSerie,
		Coautores: []repertorio.Coautor{
			{Nombre: "Autor de Prueba", IPI: "IPI-" + id, Rol: repertorio.RolGuionista},
		},
	}
	for _, f := range ajustar {
		f(&m)
	}

	o, err := repertorio.NuevaObra(id, m)
	if err != nil {
		t.Fatalf("construir la obra de prueba %q: %v", id, err)
	}
	return o
}

// insertarUsoSQL siembra un uso pendiente por SQL directo, con los valores
// que pone la ingesta (#72): escalon 'pendiente', oni TRUE, puntaje 0,
// emisiones 1. Solo llena lo que el escalon 1-2 necesita; el resto queda en
// su DEFAULT.
func insertarUsoSQL(t *testing.T, pool *pgxpool.Pool, u aplicacion.UsoPersistido) {
	t.Helper()

	modalidad := u.Modalidad
	if modalidad == "" {
		modalidad = reparto.TV
	}
	_, err := pool.Exec(t.Context(),
		`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, modalidad,
		                    escalon, oni, puntaje, emisiones)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pendiente', TRUE, 0, 1)`,
		u.ID, u.ReporteID, u.Fuente, u.Titulo, u.IDsFuente, string(modalidad))
	if err != nil {
		t.Fatalf("insertar uso %q: %v", u.ID, err)
	}
}

// sembrarIdentificacion deja un catalogo minimo -una obra con imdb, una con
// ida, una sin ningun identificador global- y un reporte con tres usos
// pendientes: u-1 y u-2 comparten el par (caracol, id_ficha, 871732) -el
// mismo programa en dos emisiones-, y u-3 no trae ids_fuente.
func sembrarIdentificacion(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	pool := testhelp.Pool(t)
	ctx := t.Context()
	s := &Store{pool: pool}

	if err := s.Registrar(ctx, nuevaObraGlobal(t, obraImdb, func(m *repertorio.Metadatos) {
		m.IMDB = "tt0100001"
	})); err != nil {
		t.Fatalf("sembrar %q: %v", obraImdb, err)
	}
	if err := s.Registrar(ctx, nuevaObraGlobal(t, obraIda, func(m *repertorio.Metadatos) {
		m.IDA = "IDA-000001"
	})); err != nil {
		t.Fatalf("sembrar %q: %v", obraIda, err)
	}
	if err := s.Registrar(ctx, nuevaObraGlobal(t, obraVacia)); err != nil {
		t.Fatalf("sembrar %q: %v", obraVacia, err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ($1, 'caracol', '2024', repeat('a', 64), 'reportes/rep-1.csv', 100)`,
		reporteUno); err != nil {
		t.Fatalf("sembrar reporte: %v", err)
	}

	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-1", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "La Casa de las Dos Palmas", IDsFuente: "id_ficha=871732\nimdb=tt0100001",
	})
	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-2", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "La Casa de las Dos Palmas", IDsFuente: "id_ficha=871732",
	})
	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-3", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "Sin identificadores", IDsFuente: "",
	})

	return s, pool
}

// ---------------------------------------------------------------------------
// Alias

func TestAliasSinFilaEsNoEncontrado(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	_, err := s.Alias(t.Context(), "caracol", "id_ficha", "871732")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	if !strings.Contains(err.Error(), "alias de") {
		t.Fatalf("el mensaje no nombra la consulta: %v", err)
	}
}

func TestAliasConFilaDevuelveLaObra(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO alias_obra (fuente, tipo_id, valor, obra_id, quien)
		 VALUES ('caracol', 'id_ficha', '871732', $1, NULL)`, obraImdb); err != nil {
		t.Fatalf("sembrar alias: %v", err)
	}

	obraID, err := s.Alias(ctx, "caracol", "id_ficha", "871732")
	if err != nil {
		t.Fatalf("Alias: %v", err)
	}
	if obraID != obraImdb {
		t.Fatalf("obraID = %q, se esperaba %q", obraID, obraImdb)
	}
}

// ---------------------------------------------------------------------------
// GuardarAlias

func TestGuardarAliasInsertaYSeLee(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraImdb, "cascada"); err != nil {
		t.Fatalf("GuardarAlias: %v", err)
	}

	var fuente, tipo, valor, obraID, quien string
	err := pool.QueryRow(ctx,
		`SELECT fuente, tipo_id, valor, obra_id, quien FROM alias_obra
		  WHERE fuente = 'caracol' AND tipo_id = 'id_ficha' AND valor = '871732'`).
		Scan(&fuente, &tipo, &valor, &obraID, &quien)
	if err != nil {
		t.Fatalf("leer el alias: %v", err)
	}
	if fuente != "caracol" || tipo != "id_ficha" || valor != "871732" || obraID != obraImdb || quien != "cascada" {
		t.Fatalf("alias mal guardado: fuente=%q tipo=%q valor=%q obra=%q quien=%q",
			fuente, tipo, valor, obraID, quien)
	}
}

// Re-correr GuardarAlias con el mismo par es lo que hace una re-corrida sobre
// un periodo ya procesado (D9): no puede duplicar ni fallar.
func TestGuardarAliasEsIdempotente(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	for range 2 {
		if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraImdb, "cascada"); err != nil {
			t.Fatalf("GuardarAlias: %v", err)
		}
	}

	var cuantas int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM alias_obra
		  WHERE fuente = 'caracol' AND tipo_id = 'id_ficha' AND valor = '871732'`).
		Scan(&cuantas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if cuantas != 1 {
		t.Fatalf("se esperaba una sola fila, hay %d", cuantas)
	}
}

// D5: un match automatico no puede deshacer en silencio una decision
// anterior. ON CONFLICT DO NOTHING es lo que hace real esa regla.
func TestGuardarAliasNoPisaUnAliasExistente(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraImdb, "cascada"); err != nil {
		t.Fatalf("primer GuardarAlias: %v", err)
	}
	if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraIda, "cascada"); err != nil {
		t.Fatalf("segundo GuardarAlias: %v", err)
	}

	var obraID string
	if err := pool.QueryRow(ctx,
		`SELECT obra_id FROM alias_obra
		  WHERE fuente = 'caracol' AND tipo_id = 'id_ficha' AND valor = '871732'`).
		Scan(&obraID); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if obraID != obraImdb {
		t.Fatalf("obra_id = %q, la resolucion automatica piso un alias existente", obraID)
	}
}

func TestGuardarAliasRechazaValorVacio(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarAlias(t.Context(), "caracol", "id_ficha", "", obraImdb, "cascada")
	if err == nil {
		t.Fatal("se esperaba un error: el CHECK de valor no vacio no se disparo")
	}
	if errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba un error de constraint, no ErrNoEncontrado: %v", err)
	}
}

func TestGuardarAliasSinQuienQuedaNull(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraImdb, ""); err != nil {
		t.Fatalf("GuardarAlias: %v", err)
	}

	var quienEsNull bool
	if err := pool.QueryRow(ctx,
		`SELECT quien IS NULL FROM alias_obra
		  WHERE fuente = 'caracol' AND tipo_id = 'id_ficha' AND valor = '871732'`).
		Scan(&quienEsNull); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if !quienEsNull {
		t.Fatal("quien deberia quedar NULL cuando no se pasa actor")
	}
}

// ---------------------------------------------------------------------------
// ObraPorIDGlobal

func TestObraPorIDGlobalMatchPorCadaIdentificador(t *testing.T) {
	s, _ := sembrarIdentificacion(t)
	ctx := t.Context()

	const obraEidr = "obra-eidr"
	if err := s.Registrar(ctx, nuevaObraGlobal(t, obraEidr, func(m *repertorio.Metadatos) {
		m.EIDR = "EIDR-000001"
	})); err != nil {
		t.Fatalf("sembrar %q: %v", obraEidr, err)
	}

	casos := []struct {
		nombre          string
		ida, eidr, imdb string
		quiero          string
	}{
		{"ida", "IDA-000001", "", "", obraIda},
		{"eidr", "", "EIDR-000001", "", obraEidr},
		{"imdb", "", "", "tt0100001", obraImdb},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			obraID, err := s.ObraPorIDGlobal(ctx, c.ida, c.eidr, c.imdb)
			if err != nil {
				t.Fatalf("ObraPorIDGlobal: %v", err)
			}
			if obraID != c.quiero {
				t.Fatalf("obraID = %q, se esperaba %q", obraID, c.quiero)
			}
		})
	}
}

func TestObraPorIDGlobalSinMatch(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	_, err := s.ObraPorIDGlobal(t.Context(), "IDA-NO-EXISTE", "", "")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Contrato del puerto (puertos.go): los tres identificadores vacios no pueden
// pasar por "no hay match" sin tocar la base -llamarla asi no puede pasar en
// la cascada real, pero el adaptador tiene que rechazarlo igual.
func TestObraPorIDGlobalConLosTresVacios(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	_, err := s.ObraPorIDGlobal(t.Context(), "", "", "")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// D9: sin UNIQUE en obras.imdb, dos obras pueden compartir el mismo
// identificador global. La corrida no puede depender del orden fisico.
func TestObraPorIDGlobalConDuplicadosDevuelveElMenorID(t *testing.T) {
	s, _ := sembrarIdentificacion(t)
	ctx := t.Context()

	if err := s.Registrar(ctx, nuevaObraGlobal(t, "obra-99-duplicada", func(m *repertorio.Metadatos) {
		m.IMDB = "tt0100001" // mismo imdb que obraImdb ("obra-45")
	})); err != nil {
		t.Fatalf("sembrar duplicado: %v", err)
	}

	obraID, err := s.ObraPorIDGlobal(ctx, "", "", "tt0100001")
	if err != nil {
		t.Fatalf("ObraPorIDGlobal: %v", err)
	}
	if obraID != obraImdb {
		t.Fatalf("obraID = %q, se esperaba el menor id (%q)", obraID, obraImdb)
	}
}

// La cascada nunca llama asi -siempre con exactamente un identificador
// poblado (D3)-, pero el SQL usa OR y el determinismo tiene que sostenerse
// igual si dos identificadores distintos casan filas distintas.
func TestObraPorIDGlobalConDosIdentificadoresPobladosDecidePorID(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	obraID, err := s.ObraPorIDGlobal(t.Context(), "IDA-000001", "", "tt0100001")
	if err != nil {
		t.Fatalf("ObraPorIDGlobal: %v", err)
	}
	// obraIda ("obra-12") e obraImdb ("obra-45") casan cada una por su lado;
	// "obra-12" es la menor alfabeticamente.
	if obraID != obraIda {
		t.Fatalf("obraID = %q, se esperaba %q (ORDER BY id)", obraID, obraIda)
	}
}

// ---------------------------------------------------------------------------
// GuardarMatch

func TestGuardarMatchActualizaLaFilaConElResultado(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	r := identificacion.Resultado{
		ObraID:    obraImdb,
		Escalon:   identificacion.EscalonAlias,
		Puntaje:   decimal.NewFromInt(1),
		Evidencia: "alias caracol id_ficha=871732 -> obra-45",
	}
	if err := s.GuardarMatch(ctx, "u-1", r); err != nil {
		t.Fatalf("GuardarMatch: %v", err)
	}

	var (
		obraID, escalon, evidencia      string
		puntaje                         decimal.Decimal
		oni                             bool
		resueltoPorNulo, resueltoEnNulo bool
	)
	err := pool.QueryRow(ctx,
		`SELECT obra_id, escalon, evidencia, puntaje, oni,
		        resuelto_por IS NULL, resuelto_en IS NULL
		   FROM usos WHERE id = 'u-1'`).
		Scan(&obraID, &escalon, &evidencia, &puntaje, &oni, &resueltoPorNulo, &resueltoEnNulo)
	if err != nil {
		t.Fatalf("leer el uso: %v", err)
	}
	if obraID != obraImdb || escalon != identificacion.EscalonAlias || evidencia != r.Evidencia {
		t.Fatalf("fila mal actualizada: obra=%q escalon=%q evidencia=%q", obraID, escalon, evidencia)
	}
	if !puntaje.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("puntaje = %s, se esperaba 1", puntaje)
	}
	if oni {
		t.Fatal("oni deberia quedar false: la fila tiene obra")
	}
	if !resueltoPorNulo || !resueltoEnNulo {
		t.Fatal("resuelto_por/resuelto_en deberian quedar NULL: no es resolucion manual")
	}
}

// GuardarMatch queda preparado para #32/#37 sin cambios (D6): un Resultado
// con ObraID vacio y Escalon "oni" es justo lo que ese futuro caso de uso
// escribira, y el CHECK uso_resuelto_tiene_obra lo admite.
func TestGuardarMatchConResultadoONIDejaObraNula(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	r := identificacion.Resultado{Escalon: "oni", ObraID: "", ONI: true}
	if err := s.GuardarMatch(ctx, "u-3", r); err != nil {
		t.Fatalf("GuardarMatch: %v", err)
	}

	var obraIDNulo, oni bool
	if err := pool.QueryRow(ctx,
		`SELECT obra_id IS NULL, oni FROM usos WHERE id = 'u-3'`).
		Scan(&obraIDNulo, &oni); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if !obraIDNulo {
		t.Fatal("obra_id deberia quedar NULL")
	}
	if !oni {
		t.Fatal("oni deberia quedar true")
	}
}

// Cero filas afectadas no puede pasar por silencio (D8): con la lectura de
// pendientes como unica fuente de usoID, solo pasa por una carrera.
func TestGuardarMatchConUsoInexistente(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-no-existe", identificacion.Resultado{
		ObraID: obraImdb, Escalon: identificacion.EscalonAlias, Puntaje: decimal.NewFromInt(1),
	})
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// El adaptador no traga un estado que el esquema no conoce: "excluido" es
// una decision que solo vive en memoria (D4) y nunca llega a GuardarMatch en
// una corrida real.
func TestGuardarMatchRechazaUnEscalonQueElEsquemaNoConoce(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", identificacion.Resultado{
		Escalon: identificacion.EscalonExcluido,
	})
	if err == nil {
		t.Fatal("se esperaba un error: el CHECK de escalon no se disparo")
	}
}

// La resolucion manual no entra por este puerto en este issue: sin
// resuelto_por/resuelto_en, manual_tiene_autor tiene que rechazarla.
func TestGuardarMatchRechazaManualSinAutor(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", identificacion.Resultado{
		Escalon: "manual",
		ObraID:  obraImdb,
	})
	if err == nil {
		t.Fatal("se esperaba un error: manual_tiene_autor no se disparo")
	}
}

func TestGuardarMatchRechazaUnaObraQueNoExiste(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", identificacion.Resultado{
		Escalon: identificacion.EscalonAlias,
		ObraID:  "obra-inexistente",
		Puntaje: decimal.NewFromInt(1),
	})
	if err == nil {
		t.Fatal("se esperaba un error de clave foranea")
	}
}
