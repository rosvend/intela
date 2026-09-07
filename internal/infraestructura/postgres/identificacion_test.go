package postgres

import (
	"context"
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

// ---------------------------------------------------------------------------
// Integracion de aplicacion.ResolverUsos contra PostgreSQL real.
//
// RepositorioIngesta no tiene adaptador en main (#72 abierta): este issue no
// lo implementa. ingestaDePrueba es un doble LOCAL solo para la lectura de
// usos por periodo -lee del mismo pool con SQL directo-; todo lo que es
// escritura y sondeo de este issue (Alias, GuardarAlias, ObraPorIDGlobal,
// GuardarMatch) corre contra el Store real.

// ingestaDePrueba satisface aplicacion.RepositorioIngesta. Solo UsosDePeriodo
// tiene comportamiento: es el unico metodo que ResolverUsos llama.
type ingestaDePrueba struct {
	pool *pgxpool.Pool
}

func (i ingestaDePrueba) GuardarReporte(context.Context, string, string, string, string, string, int) error {
	return nil
}
func (i ingestaDePrueba) GuardarUsos(context.Context, []aplicacion.UsoPersistido) error { return nil }
func (i ingestaDePrueba) UsosSinResolver(context.Context) ([]aplicacion.UsoPersistido, error) {
	return nil, nil
}
func (i ingestaDePrueba) UsoPorID(context.Context, string) (aplicacion.UsoPersistido, error) {
	return aplicacion.UsoPersistido{}, nil
}

func (i ingestaDePrueba) UsosDePeriodo(ctx context.Context, periodo string) ([]aplicacion.UsoPersistido, error) {
	filas, err := i.pool.Query(ctx,
		`SELECT u.id, u.reporte_id, u.fuente, u.titulo, u.ids_fuente, COALESCE(u.obra_id, ''),
		        u.escalon, u.evidencia, u.oni, u.modalidad
		   FROM usos u
		   JOIN reportes r ON r.id = u.reporte_id
		  WHERE r.periodo = $1
		  ORDER BY u.id`, periodo)
	if err != nil {
		return nil, err
	}
	defer filas.Close()

	var usos []aplicacion.UsoPersistido
	for filas.Next() {
		var u aplicacion.UsoPersistido
		var modalidad string
		if err := filas.Scan(&u.ID, &u.ReporteID, &u.Fuente, &u.Titulo, &u.IDsFuente, &u.ObraID,
			&u.Escalon, &u.Evidencia, &u.ONI, &modalidad); err != nil {
			return nil, err
		}
		u.Modalidad = reparto.Modalidad(modalidad)
		usos = append(usos, u)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	return usos, nil
}

// filaUso trae lo que un test de integracion necesita mirar de una fila de
// usos, en un solo SELECT.
type filaUso struct {
	obraIDNulo bool
	obraID     string
	escalon    string
	evidencia  string
	oni        bool
}

func leerUso(t *testing.T, pool *pgxpool.Pool, id string) filaUso {
	t.Helper()
	var f filaUso
	err := pool.QueryRow(t.Context(),
		`SELECT obra_id IS NULL, COALESCE(obra_id, ''), escalon, evidencia, oni
		   FROM usos WHERE id = $1`, id).
		Scan(&f.obraIDNulo, &f.obraID, &f.escalon, &f.evidencia, &f.oni)
	if err != nil {
		t.Fatalf("leer el uso %q: %v", id, err)
	}
	return f
}

func contarAlias(t *testing.T, pool *pgxpool.Pool, fuente, tipo, valor string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM alias_obra WHERE fuente = $1 AND tipo_id = $2 AND valor = $3`,
		fuente, tipo, valor).Scan(&n); err != nil {
		t.Fatalf("contar alias: %v", err)
	}
	return n
}

// I1 (criterio 1 de la issue): una fila cuyo id de fuente ya esta en
// alias_obra resuelve en escalon 1 con confianza maxima, sin trabajo difuso.
func TestResolverUsosIntegracionCriterio1AliasExistente(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if err := s.GuardarAlias(ctx, "caracol", "id_ficha", "871732", obraImdb, ""); err != nil {
		t.Fatalf("sembrar alias: %v", err)
	}

	r := aplicacion.ResolverUsos{Usos: ingestaDePrueba{pool: pool}, Identificacion: s}
	n, err := r.ResolverUsos(ctx, "2024")
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 2 {
		t.Fatalf("n = %d, se esperaban 2 (u-1 y u-2 comparten el par)", n)
	}

	for _, id := range []string{"u-1", "u-2"} {
		f := leerUso(t, pool, id)
		if f.obraID != obraImdb || f.escalon != identificacion.EscalonAlias || f.oni {
			t.Fatalf("uso %q mal resuelto: %+v", id, f)
		}
		if f.evidencia != "alias caracol id_ficha=871732 -> obra-45" {
			t.Fatalf("uso %q evidencia = %q", id, f.evidencia)
		}
	}
}

// I2 (criterio 2) + I3 (criterio 3, cortocircuito): una fila sin alias pero
// con un identificador global que casa resuelve en escalon 2 y aprende el
// alias; una fila nueva con el mismo id de fuente, en una corrida posterior,
// resuelve en escalon 1 -y la fila vieja no se vuelve a tocar.
func TestResolverUsosIntegracionCriterio2Y3(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()
	r := aplicacion.ResolverUsos{Usos: ingestaDePrueba{pool: pool}, Identificacion: s}

	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("primera corrida: %v", err)
	}

	u1 := leerUso(t, pool, "u-1")
	if u1.obraID != obraImdb || u1.escalon != identificacion.EscalonIDGlobal {
		t.Fatalf("u-1 mal resuelto: %+v", u1)
	}
	if !strings.Contains(u1.evidencia, "imdb") {
		t.Fatalf("la evidencia de u-1 no nombra imdb: %q", u1.evidencia)
	}
	if contarAlias(t, pool, "caracol", "id_ficha", "871732") != 1 {
		t.Fatal("el escalon 2 deberia haber aprendido el alias")
	}

	// Un segundo uso del mismo programa, otra emision, en una corrida
	// posterior: el alias que la corrida anterior aprendio tiene que
	// cortocircuitarlo por escalon 1.
	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-4", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "La Casa de las Dos Palmas", IDsFuente: "id_ficha=871732",
	})
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("segunda corrida: %v", err)
	}

	u4 := leerUso(t, pool, "u-4")
	if u4.obraID != obraImdb || u4.escalon != identificacion.EscalonAlias {
		t.Fatalf("u-4 deberia cortocircuitar por alias: %+v", u4)
	}

	// La fila vieja no se re-procesa: sigue como la dejo el escalon 2.
	u1otraVez := leerUso(t, pool, "u-1")
	if u1otraVez != u1 {
		t.Fatalf("u-1 se volvio a tocar: antes %+v, ahora %+v", u1, u1otraVez)
	}
}

// I4 (criterio 4): un canal/programa fuera de repertorio se excluye antes de
// la cascada y no se marca ONI -queda exactamente como llego.
func TestResolverUsosIntegracionCriterio4Repertorio(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	// El alias SI pegaria si la fuente no estuviera excluida: la prueba real
	// es que la exclusion corre antes y ni lo intenta.
	if err := s.GuardarAlias(ctx, "canal-deportes", "id_ficha", "999", obraImdb, ""); err != nil {
		t.Fatalf("sembrar alias de canal-deportes: %v", err)
	}
	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-5", ReporteID: reporteUno, Fuente: "canal-deportes",
		Titulo: "Gol Caracol", IDsFuente: "id_ficha=999",
	})

	r := aplicacion.ResolverUsos{
		Usos: ingestaDePrueba{pool: pool}, Identificacion: s,
		FueraDeRepertorio: identificacion.FuentesExcluidas{"canal-deportes"},
	}
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	u5 := leerUso(t, pool, "u-5")
	if u5.escalon != "pendiente" || !u5.obraIDNulo || !u5.oni {
		t.Fatalf("una fila excluida no puede cambiar de estado: %+v", u5)
	}
	if contarAlias(t, pool, "canal-deportes", "id_ficha", "999") != 1 {
		t.Fatal("la exclusion no puede ganar ni perder filas de alias_obra")
	}
}

// I5 (criterio 5): lo que no resuelve queda intacto, como insumo del difuso.
func TestResolverUsosIntegracionCriterio5NoResuelto(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-6", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "Obra sin catalogar", IDsFuente: "id_ficha=555\nimdb=tt9999999",
	})

	r := aplicacion.ResolverUsos{Usos: ingestaDePrueba{pool: pool}, Identificacion: s}
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	u6 := leerUso(t, pool, "u-6")
	if u6.escalon != "pendiente" || !u6.obraIDNulo {
		t.Fatalf("una fila sin match no puede resolver: %+v", u6)
	}
	if contarAlias(t, pool, "caracol", "id_ficha", "555") != 0 {
		t.Fatal("una fila que no resolvio no puede aprender un alias")
	}
}

// I6: re-ejecutar sobre un periodo ya corrido no cambia nada y no duplica.
func TestResolverUsosIntegracionEsIdempotente(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()
	r := aplicacion.ResolverUsos{Usos: ingestaDePrueba{pool: pool}, Identificacion: s}

	n1, err := r.ResolverUsos(ctx, "2024")
	if err != nil {
		t.Fatalf("primera corrida: %v", err)
	}
	if n1 == 0 {
		t.Fatal("la primera corrida deberia haber resuelto algo")
	}
	u1 := leerUso(t, pool, "u-1")
	u2 := leerUso(t, pool, "u-2")
	alias := contarAlias(t, pool, "caracol", "id_ficha", "871732")

	n2, err := r.ResolverUsos(ctx, "2024")
	if err != nil {
		t.Fatalf("segunda corrida: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("la segunda corrida resolvio %d filas, se esperaban 0: no quedan pendientes", n2)
	}
	if got := leerUso(t, pool, "u-1"); got != u1 {
		t.Fatalf("u-1 cambio en la re-corrida: antes %+v, ahora %+v", u1, got)
	}
	if got := leerUso(t, pool, "u-2"); got != u2 {
		t.Fatalf("u-2 cambio en la re-corrida: antes %+v, ahora %+v", u2, got)
	}
	if got := contarAlias(t, pool, "caracol", "id_ficha", "871732"); got != alias {
		t.Fatalf("alias_obra duplico filas: antes %d, ahora %d", alias, got)
	}
}
