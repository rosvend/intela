package postgres

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// pisoDePrueba: el mismo matching.umbral_banda que siembra el dataset.
var pisoDePrueba = decimal.RequireFromString("0.45")

// obraConTitulo inserta por SQL: lo que se prueba es como se INDEXA el titulo,
// y varios casos necesitan grafias que no hace falta pasar por el dominio. El
// coautor va porque Buscar reconstruye la entidad y la exige.
func obraConTitulo(t *testing.T, pool *pgxpool.Pool, id, titulo string) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ($1, $2, 'Drama', 2000, 'serie')`,
		id, titulo); err != nil {
		t.Fatalf("sembrar la obra %q: %v", id, err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO obra_coautores (obra_id, ipi, nombre, rol)
		 VALUES ($1, 'IPI-' || $1, 'Autor de Prueba', 'guionista')`, id); err != nil {
		t.Fatalf("sembrar el coautor de %q: %v", id, err)
	}
}

func idsCandidatos(cs []identificacion.Candidato) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ObraID)
	}
	return out
}

// Criterios de aceptacion: mayusculas, tildes y orden de palabras llegan a la
// misma obra.
func TestCandidatosResuelveLasVariantesDelTitulo(t *testing.T) {
	s, _ := sembrar(t)

	casos := []struct {
		nombre  string
		consulz string
	}{
		{"identico", "La Casa de las Dos Palmas"},
		{"todo en mayusculas", "LA CASA DE LAS DOS PALMAS"},
		{"todo en minusculas", "la casa de las dos palmas"},
		{"palabras en otro orden", "Dos Palmas, La Casa de las"},
		{"con puntuacion de mas", "¡La Casa de las Dos Palmas!"},
		{"sin articulos", "Casa de las Dos Palmas"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cs, err := s.Candidatos(t.Context(), c.consulz, pisoDePrueba)
			if err != nil {
				t.Fatalf("Candidatos: %v", err)
			}
			if len(cs) == 0 {
				t.Fatalf("%q no propuso ningun candidato", c.consulz)
			}
			if cs[0].ObraID != obraCompleta {
				t.Fatalf("%q propuso %q como mejor candidato, se esperaba %s",
					c.consulz, cs[0].ObraID, obraCompleta)
			}
		})
	}
}

// Las tildes dan igual en los dos sentidos: asi llegan los titulos de las
// parrillas reales.
func TestCandidatosIgnoraLasTildes(t *testing.T) {
	s, pool := sembrar(t)
	obraConTitulo(t, pool, "obra-tildes", "¿Dónde está Elisa?")

	for _, consulta := range []string{"Donde esta Elisa", "¿DÓNDE ESTÁ ELISA?", "donde está elisa"} {
		t.Run(consulta, func(t *testing.T) {
			cs, err := s.Candidatos(t.Context(), consulta, pisoDePrueba)
			if err != nil {
				t.Fatalf("Candidatos: %v", err)
			}
			if len(cs) == 0 || cs[0].ObraID != "obra-tildes" {
				t.Fatalf("%q no encontro la obra con tildes: %v", consulta, idsCandidatos(cs))
			}
		})
	}
}

// Sin ranking, "el mejor candidato" seria el primero que devolviera la base.
func TestCandidatosOrdenaDeMasAMenosParecida(t *testing.T) {
	s, pool := sembrar(t)
	obraConTitulo(t, pool, "obra-casi", "La Casa de las Dos Palmeras")

	cs, err := s.Candidatos(t.Context(), "La Casa de las Dos Palmas", pisoDePrueba)
	if err != nil {
		t.Fatalf("Candidatos: %v", err)
	}
	if len(cs) < 2 {
		t.Fatalf("se esperaban las dos parecidas, llegaron %v", idsCandidatos(cs))
	}
	if cs[0].ObraID != obraCompleta {
		t.Fatalf("el orden no pone primero la exacta: %v", idsCandidatos(cs))
	}
	if !cs[0].Puntaje.GreaterThan(cs[1].Puntaje) {
		t.Fatalf("puntajes sin ordenar: %s luego %s", cs[0].Puntaje, cs[1].Puntaje)
	}
	if !cs[0].Puntaje.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("un titulo identico tendria que puntuar 1, puntuo %s", cs[0].Puntaje)
	}
}

// Sin parecido: lista vacia, no error. Es el camino a ONI (RD 13.8).
func TestCandidatosSinParecidoDevuelveListaVacia(t *testing.T) {
	s, _ := sembrar(t)

	cs, err := s.Candidatos(t.Context(), "Zzzz Qqqq Wwww Xxxx", pisoDePrueba)
	if err != nil {
		t.Fatalf("Candidatos: %v", err)
	}
	if len(cs) != 0 {
		t.Fatalf("se esperaba lista vacia, llegaron %v", idsCandidatos(cs))
	}
}

// El piso recorta de verdad: es lo que da efecto al parametro normativo.
func TestCandidatosRespetaElPiso(t *testing.T) {
	s, pool := sembrar(t)
	obraConTitulo(t, pool, "obra-lejana", "La Casa Grande")

	bajo, err := s.Candidatos(t.Context(), "La Casa de las Dos Palmas", decimal.RequireFromString("0.10"))
	if err != nil {
		t.Fatalf("Candidatos con piso bajo: %v", err)
	}
	alto, err := s.Candidatos(t.Context(), "La Casa de las Dos Palmas", decimal.RequireFromString("0.95"))
	if err != nil {
		t.Fatalf("Candidatos con piso alto: %v", err)
	}

	if !slices.Contains(idsCandidatos(bajo), "obra-lejana") {
		t.Fatalf("con piso 0.10 se esperaba la obra lejana: %v", idsCandidatos(bajo))
	}
	if slices.Contains(idsCandidatos(alto), "obra-lejana") {
		t.Fatalf("con piso 0.95 no se esperaba la obra lejana: %v", idsCandidatos(alto))
	}
	if len(alto) != 1 || alto[0].ObraID != obraCompleta {
		t.Fatalf("con piso 0.95 solo cabe la exacta: %v", idsCandidatos(alto))
	}
}

// El tope acota tambien lo que se persiste en candidatos_match.
func TestCandidatosNoDevuelveMasDeLTope(t *testing.T) {
	s, pool := sembrar(t)
	for _, id := range []string{"c-1", "c-2", "c-3", "c-4", "c-5", "c-6", "c-7"} {
		obraConTitulo(t, pool, id, "La Casa de las Dos Palmas "+id)
	}

	cs, err := s.Candidatos(t.Context(), "La Casa de las Dos Palmas", decimal.RequireFromString("0.10"))
	if err != nil {
		t.Fatalf("Candidatos: %v", err)
	}
	if len(cs) > identificacion.MaxCandidatos {
		t.Fatalf("se propusieron %d candidatos, el tope es %d", len(cs), identificacion.MaxCandidatos)
	}
}

// Dos corridas proponen lo mismo en el mismo orden (ADR 0005).
func TestCandidatosEsDeterministaConEmpates(t *testing.T) {
	s, pool := sembrar(t)
	// Tres grafias que normalizan igual: puntuan 1 las tres, solo el id ordena.
	obraConTitulo(t, pool, "obra-zz", "LA CASA DE LAS DOS PALMAS")
	obraConTitulo(t, pool, "obra-aa", "la casa de las dos palmas")

	var primera []string
	for range 5 {
		cs, err := s.Candidatos(t.Context(), "La Casa de las Dos Palmas", pisoDePrueba)
		if err != nil {
			t.Fatalf("Candidatos: %v", err)
		}
		got := idsCandidatos(cs)
		if primera == nil {
			primera = got
			continue
		}
		if !slices.Equal(got, primera) {
			t.Fatalf("dos corridas propusieron ordenes distintos: %v y %v", primera, got)
		}
	}
	if !slices.Equal(primera, []string{"obra-aa", obraCompleta, "obra-zz"}) {
		t.Fatalf("el empate no se rompio por id ascendente: %v", primera)
	}
}

// SET LOCAL y no SET: la conexion vuelve al pool sin el corte puesto, o la
// siguiente consulta heredaria un umbral que nadie le pidio.
func TestCandidatosNoDejaElPisoPegadoALaConexion(t *testing.T) {
	s, pool := sembrar(t)

	if _, err := s.Candidatos(t.Context(), "La Casa de las Dos Palmas", decimal.RequireFromString("0.99")); err != nil {
		t.Fatalf("Candidatos: %v", err)
	}

	var corte string
	if err := pool.QueryRow(t.Context(),
		`SELECT current_setting('pg_trgm.similarity_threshold')`).Scan(&corte); err != nil {
		t.Fatalf("leer el corte: %v", err)
	}
	if corte != "0.3" {
		t.Fatalf("la conexion quedo con el corte en %s, se esperaba el de fabrica (0.3)", corte)
	}
}
