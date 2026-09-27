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
		`INSERT INTO usos (id, reporte_id, fuente, titulo, titulo_original, ids_fuente, modalidad,
		                    escalon, oni, puntaje, emisiones)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pendiente', TRUE, 0, 1)`,
		u.ID, u.ReporteID, u.Fuente, u.Titulo, u.TituloOrig, u.IDsFuente, string(modalidad))
	if err != nil {
		t.Fatalf("insertar uso %q: %v", u.ID, err)
	}
}

// sembrarIdentificacion deja un catalogo minimo -una obra con imdb, una con
// ida, una sin ningun identificador global- y un reporte con tres usos
// pendientes: u-1 y u-2 comparten el par (caracol, id_ficha, 871732) -el
// mismo programa en dos emisiones-, y u-3 no trae ids_fuente. Todo en el
// formato del contrato de ids_fuente (ADR 0018).
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
	if err := s.GuardarMatch(ctx, "u-1", "pendiente", r); err != nil {
		t.Fatalf("GuardarMatch: %v", err)
	}

	var (
		obraID, escalon, evidencia, tipoObra string
		puntaje                              decimal.Decimal
		oni                                  bool
		resueltoPorNulo, resueltoEnNulo      bool
	)
	err := pool.QueryRow(ctx,
		`SELECT obra_id, escalon, evidencia, puntaje, oni, tipo_obra,
		        resuelto_por IS NULL, resuelto_en IS NULL
		   FROM usos WHERE id = 'u-1'`).
		Scan(&obraID, &escalon, &evidencia, &puntaje, &oni, &tipoObra, &resueltoPorNulo, &resueltoEnNulo)
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
	// La fila llego sin tipo_obra (el mapa de Caracol no lo trae). Con obra,
	// el tipo sale del catalogo (#165).
	if tipoObra != string(repertorio.TipoSerie) {
		t.Fatalf("tipo_obra = %q, se esperaba %q desde obras.tipo", tipoObra, repertorio.TipoSerie)
	}
}

// Un tipo que ya trajo la fuente no se pisa con el del catalogo.
func TestGuardarMatchNoPisaElTipoObraQueTrajoLaFuente(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `UPDATE usos SET tipo_obra = 'unitario' WHERE id = 'u-1'`); err != nil {
		t.Fatalf("plantar tipo_obra: %v", err)
	}
	if err := s.GuardarMatch(ctx, "u-1", "pendiente", identificacion.Resultado{
		ObraID:  obraImdb,
		Escalon: identificacion.EscalonAlias,
		Puntaje: decimal.NewFromInt(1),
	}); err != nil {
		t.Fatalf("GuardarMatch: %v", err)
	}
	var tipo string
	if err := pool.QueryRow(ctx, `SELECT tipo_obra FROM usos WHERE id = 'u-1'`).Scan(&tipo); err != nil {
		t.Fatalf("leer tipo_obra: %v", err)
	}
	if tipo != "unitario" {
		t.Fatalf("tipo_obra = %q, se esperaba unitario: la fuente ya lo traia", tipo)
	}
}

// GuardarMatch queda preparado para #32/#37 sin cambios (D6): un Resultado
// con ObraID vacio y Escalon "oni" es justo lo que ese futuro caso de uso
// escribira, y el CHECK uso_resuelto_tiene_obra lo admite.
func TestGuardarMatchConResultadoONIDejaObraNula(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	r := identificacion.Resultado{Escalon: "oni", ObraID: "", ONI: true}
	if err := s.GuardarMatch(ctx, "u-3", "pendiente", r); err != nil {
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

// Cero filas afectadas no pasa en silencio: el adaptador lo dice con
// ErrNoEncontrado, y es el caso de uso el que decide saltar la fila.
func TestGuardarMatchConUsoInexistente(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-no-existe", "pendiente", identificacion.Resultado{
		ObraID: obraImdb, Escalon: identificacion.EscalonAlias, Puntaje: decimal.NewFromInt(1),
	})
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Hallazgo 3: el UPDATE es condicional al escalon con que se leyo la fila. Si
// otro proceso la cambio -aqui, una resolucion manual-, GuardarMatch no la
// pisa y responde ErrNoEncontrado.
func TestGuardarMatchNoPisaUnaFilaQueCambioDeEscalon(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 VALUES ('usr-1', 'a@b.co', 'Revisora', 'distribucion', repeat('x', 20))`); err != nil {
		t.Fatalf("sembrar usuario: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE usos SET escalon = 'manual', obra_id = $1, oni = FALSE,
		        resuelto_por = 'usr-1', resuelto_en = now()
		  WHERE id = 'u-1'`, obraIda); err != nil {
		t.Fatalf("resolver a mano: %v", err)
	}
	antes := leerUso(t, pool, "u-1")

	err := s.GuardarMatch(ctx, "u-1", "pendiente", identificacion.Resultado{
		ObraID: obraImdb, Escalon: identificacion.EscalonAlias, Puntaje: decimal.NewFromInt(1),
	})
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	if got := leerUso(t, pool, "u-1"); got != antes {
		t.Fatalf("la resolucion manual se piso: antes %+v, ahora %+v", antes, got)
	}
}

// El adaptador no traga un estado que el esquema no conoce.
func TestGuardarMatchRechazaUnEscalonQueElEsquemaNoConoce(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", "pendiente", identificacion.Resultado{
		Escalon: "inventado",
	})
	if err == nil {
		t.Fatal("se esperaba un error: el CHECK de escalon no se disparo")
	}
}

// Criterio 4 de #28: excluida es sin obra y SIN ONI. Con la formula de oni
// de antes (oni = obra vacia) la fila quedaria en oni_publico.
func TestGuardarMatchExcluidoNoEsONI(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	r := identificacion.Resultado{
		Escalon:   identificacion.EscalonExcluido,
		Evidencia: "fuera de repertorio: caracol",
	}
	if err := s.GuardarMatch(ctx, "u-1", "pendiente", r); err != nil {
		t.Fatalf("GuardarMatch: %v", err)
	}

	f := leerUso(t, pool, "u-1")
	if f.escalon != identificacion.EscalonExcluido || !f.obraIDNulo || f.oni || f.evidencia != r.Evidencia {
		t.Fatalf("fila excluida mal guardada: %+v", f)
	}

	var enListado int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM oni_publico WHERE id = 'u-1'`).Scan(&enListado); err != nil {
		t.Fatalf("leer oni_publico: %v", err)
	}
	if enListado != 0 {
		t.Fatal("una fila excluida no puede salir en el listado publico de ONI")
	}
}

// uso_resuelto_tiene_obra (00007): la rama de 'excluido' no es una puerta
// trasera para guardar una fila con obra fuera de las reglas de siempre.
func TestGuardarMatchRechazaExcluidoConObra(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", "pendiente", identificacion.Resultado{
		Escalon: identificacion.EscalonExcluido,
		ObraID:  obraImdb,
	})
	if err == nil {
		t.Fatal("se esperaba un error: uso_resuelto_tiene_obra no se disparo")
	}
}

// La resolucion manual no entra por este puerto en este issue: sin
// resuelto_por/resuelto_en, manual_tiene_autor tiene que rechazarla.
func TestGuardarMatchRechazaManualSinAutor(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", "pendiente", identificacion.Resultado{
		Escalon: "manual",
		ObraID:  obraImdb,
	})
	if err == nil {
		t.Fatal("se esperaba un error: manual_tiene_autor no se disparo")
	}
}

func TestGuardarMatchRechazaUnaObraQueNoExiste(t *testing.T) {
	s, _ := sembrarIdentificacion(t)

	err := s.GuardarMatch(t.Context(), "u-1", "pendiente", identificacion.Resultado{
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
// cascadaDePrueba usa los adaptadores REALES de similitud y parametros: es lo
// que hace que estas pruebas ejerzan el SQL.
func cascadaDePrueba(t *testing.T, s *Store, pool *pgxpool.Pool) aplicacion.ResolverUsos {
	t.Helper()

	for clave, valor := range map[string]string{
		aplicacion.ClaveUmbralMatch: "0.60",
		aplicacion.ClaveUmbralBanda: "0.45",
	} {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
			 VALUES ($1, $2, DATE '2000-01-01', 'Consejo Directivo', 'RD-IX-prueba')
			 ON CONFLICT (clave, vigente_desde) DO NOTHING`, clave, valor); err != nil {
			t.Fatalf("sembrar el parametro %q: %v", clave, err)
		}
	}

	return aplicacion.ResolverUsos{
		Usos:           ingestaDePrueba{pool: pool},
		Identificacion: s,
		Similitud:      s,
		Parametros:     s,
		Unidad:         s,
	}
}

type ingestaDePrueba struct {
	pool *pgxpool.Pool
}

func (i ingestaDePrueba) GuardarReporte(context.Context, string, string, string, string, string, int) error {
	return nil
}
func (i ingestaDePrueba) GuardarUsos(context.Context, []aplicacion.UsoPersistido) error { return nil }
func (i ingestaDePrueba) GuardarEntrega(context.Context, aplicacion.Reporte, []aplicacion.UsoPersistido) error {
	return nil
}
func (i ingestaDePrueba) UsosSinResolver(context.Context) ([]aplicacion.UsoPersistido, error) {
	return nil, nil
}
func (i ingestaDePrueba) UsoPorID(context.Context, string) (aplicacion.UsoPersistido, error) {
	return aplicacion.UsoPersistido{}, nil
}
func (i ingestaDePrueba) ListarRechazos(context.Context) ([]aplicacion.UsoPersistido, error) {
	return nil, nil
}
func (i ingestaDePrueba) RechazosDeReporte(context.Context, string, aplicacion.Paginacion) ([]aplicacion.UsoPersistido, error) {
	return nil, nil
}
func (i ingestaDePrueba) ListarCargas(context.Context, string, aplicacion.Paginacion) ([]aplicacion.CargaReporte, error) {
	return nil, nil
}

func (i ingestaDePrueba) UsosDePeriodo(ctx context.Context, periodo string) ([]aplicacion.UsoPersistido, error) {
	filas, err := i.pool.Query(ctx,
		`SELECT u.id, u.reporte_id, u.fuente, u.titulo, u.titulo_original, u.ids_fuente, COALESCE(u.obra_id, ''),
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
		if err := filas.Scan(&u.ID, &u.ReporteID, &u.Fuente, &u.Titulo, &u.TituloOrig, &u.IDsFuente, &u.ObraID,
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

	r := cascadaDePrueba(t, s, pool)
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

// Contrato de ids_fuente (ADR 0018): una fila de Netflix escrita con
// EscribirIDsFuente -show_id, series_id y netflix_id, como la entrega del
// cliente- resuelve por el alias de show_id. Dos episodios distintos del
// mismo show casan con el mismo alias: es la razon de sondear show_id (D2).
func TestResolverUsosIntegracionNetflixPorShowID(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	if err := s.GuardarAlias(ctx, "netflix", aplicacion.ClaveShowID, "80141259", obraIda, ""); err != nil {
		t.Fatalf("sembrar alias de netflix: %v", err)
	}
	for _, ep := range []struct{ id, netflixID string }{{"u-7", "81003997"}, {"u-8", "81012675"}} {
		ids, err := aplicacion.EscribirIDsFuente(
			aplicacion.IDFuente{Clave: aplicacion.ClaveShowID, Valor: "80141259"},
			aplicacion.IDFuente{Clave: aplicacion.ClaveSeriesID, Valor: "81004793"},
			aplicacion.IDFuente{Clave: aplicacion.ClaveNetflixID, Valor: ep.netflixID},
		)
		if err != nil {
			t.Fatalf("EscribirIDsFuente: %v", err)
		}
		insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
			ID: ep.id, ReporteID: reporteUno, Fuente: "netflix",
			Titulo: "Un episodio", IDsFuente: ids, Modalidad: reparto.OTT,
		})
	}

	r := cascadaDePrueba(t, s, pool)
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	for _, id := range []string{"u-7", "u-8"} {
		f := leerUso(t, pool, id)
		if f.obraID != obraIda || f.escalon != identificacion.EscalonAlias {
			t.Fatalf("%s deberia resolver por el alias de show_id: %+v", id, f)
		}
		if f.evidencia != "alias netflix show_id=80141259 -> obra-12" {
			t.Fatalf("%s evidencia = %q", id, f.evidencia)
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
	r := cascadaDePrueba(t, s, pool)

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
// la cascada y no se marca ONI: queda escalon='excluido', sin obra y con
// oni=false, fuera de oni_publico. Una segunda corrida con la misma lista no
// la toca; una con la lista corregida la devuelve a la cascada (D4).
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

	r := cascadaDePrueba(t, s, pool)
	r.FueraDeRepertorio = identificacion.FuentesExcluidas{"canal-deportes"}
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	u5 := leerUso(t, pool, "u-5")
	if u5.escalon != identificacion.EscalonExcluido || !u5.obraIDNulo || u5.oni {
		t.Fatalf("una fila excluida tiene que quedar excluido, sin obra y sin ONI: %+v", u5)
	}
	var enListado int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM oni_publico WHERE id = 'u-5'`).Scan(&enListado); err != nil {
		t.Fatalf("leer oni_publico: %v", err)
	}
	if enListado != 0 {
		t.Fatal("una fila excluida no puede salir en el listado publico de ONI")
	}
	if contarAlias(t, pool, "canal-deportes", "id_ficha", "999") != 1 {
		t.Fatal("la exclusion no puede ganar ni perder filas de alias_obra")
	}

	// D9: con la misma lista, la re-corrida no cambia nada.
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("segunda corrida: %v", err)
	}
	if got := leerUso(t, pool, "u-5"); got != u5 {
		t.Fatalf("u-5 se volvio a tocar: antes %+v, ahora %+v", u5, got)
	}

	// La lista estaba mal y se corrige: la fila vuelve a la cascada y resuelve
	// por el alias que antes ni se sondeo.
	r.FueraDeRepertorio = nil
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("corrida con la lista corregida: %v", err)
	}
	if got := leerUso(t, pool, "u-5"); got.escalon != identificacion.EscalonAlias || got.obraID != obraImdb || got.oni {
		t.Fatalf("u-5 deberia resolver por alias tras corregir la lista: %+v", got)
	}
}

// D4: una fila excluida por error, cuya fuente deja de estar excluida y que
// la cascada no resuelve, vuelve a pendiente y a ONI -a oni_publico y al
// difuso-, en vez de quedarse fuera de todo.
func TestResolverUsosIntegracionExcluidaSinMatchVuelveAPendiente(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-5", ReporteID: reporteUno, Fuente: "canal-deportes",
		Titulo: "Gol Caracol", IDsFuente: "id_ficha=999",
	})
	r := cascadaDePrueba(t, s, pool)
	r.FueraDeRepertorio = identificacion.FuentesExcluidas{"canal-deportes"}
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("corrida con exclusion: %v", err)
	}

	r.FueraDeRepertorio = nil
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("corrida sin exclusion: %v", err)
	}
	// Vuelve al repertorio y aun asi no la reconoce nadie: ONI (RD 13.8).
	u5 := leerUso(t, pool, "u-5")
	if u5.escalon != identificacion.EscalonONI || !u5.obraIDNulo || !u5.oni {
		t.Fatalf("u-5 deberia salir a ONI: %+v", u5)
	}
}

// M1: un id local de solo espacios no puede llegar a GuardarAlias. Contra el
// CHECK de valor no vacio real de alias_obra: antes del recorte en
// entradaDesdeUso, u-0 resolvia por imdb, intentaba aprender el alias
// (caracol, id_ficha, "   "), el CHECK lo rechazaba y la corrida abortaba
// entera (D8) -sin procesar u-1..u-3, que vienen despues en el lote-.
func TestResolverUsosIntegracionIDLocalDeSoloEspaciosNoTumbaLaCorrida(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-0", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "La Casa de las Dos Palmas", IDsFuente: "id_ficha=   \nimdb=tt0100001",
	})

	r := cascadaDePrueba(t, s, pool)
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	u0 := leerUso(t, pool, "u-0")
	if u0.obraID != obraImdb || u0.escalon != identificacion.EscalonIDGlobal {
		t.Fatalf("u-0 deberia resolver por imdb: %+v", u0)
	}
	// u-0 no tiene par local tras el recorte: el unico alias aprendido es el
	// de u-1 (id_ficha=871732).
	var alias int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM alias_obra`).Scan(&alias); err != nil {
		t.Fatalf("contar alias: %v", err)
	}
	if alias != 1 || contarAlias(t, pool, "caracol", "id_ficha", "871732") != 1 {
		t.Fatalf("alias_obra tiene %d filas, se esperaba solo la de u-1", alias)
	}

	// La fila sana que viene despues en el lote se proceso.
	u1 := leerUso(t, pool, "u-1")
	if u1.obraID != obraImdb || u1.escalon != identificacion.EscalonIDGlobal {
		t.Fatalf("u-1 no se proceso tras la fila sucia: %+v", u1)
	}
}

// I5 (criterio 5): lo que ningun escalon reconoce sale a ONI, no se asigna mal.
func TestResolverUsosIntegracionCriterio5NoResuelto(t *testing.T) {
	s, pool := sembrarIdentificacion(t)
	ctx := t.Context()

	insertarUsoSQL(t, pool, aplicacion.UsoPersistido{
		ID: "u-6", ReporteID: reporteUno, Fuente: "caracol",
		Titulo: "Obra sin catalogar", IDsFuente: "id_ficha=555\nimdb=tt9999999",
	})

	r := cascadaDePrueba(t, s, pool)
	if _, err := r.ResolverUsos(ctx, "2024"); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}

	u6 := leerUso(t, pool, "u-6")
	if u6.escalon != identificacion.EscalonONI || !u6.obraIDNulo {
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
	r := cascadaDePrueba(t, s, pool)

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
