package migrations_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rosvend/intela/migrations"
)

// TestSinVersionesDuplicadas es el modo de fallo 1 del issue #110.
//
// Dos ficheros con nombres distintos y el mismo numero no dan conflicto en
// git: se mergean limpio. goose, al cargar el directorio, entra en panic:
//
//	duplicate version 7 detected
//
// Eso solo se veia en el terraform apply (lambda-migrate). Aqui se lee el
// mismo FS embebido que viaja en el binario.
func TestSinVersionesDuplicadas(t *testing.T) {
	dup, err := migrations.Duplicadas(migrations.FS)
	if err != nil {
		t.Fatalf("listar migraciones: %v", err)
	}
	if len(dup) == 0 {
		return
	}
	versiones := make([]int64, 0, len(dup))
	for v := range dup {
		versiones = append(versiones, v)
	}
	sort.Slice(versiones, func(i, j int) bool { return versiones[i] < versiones[j] })

	var b strings.Builder
	b.WriteString("hay versiones goose duplicadas; el despliegue entra en panic:\n")
	for _, v := range versiones {
		fmt.Fprintf(&b, "  version %d:\n", v)
		for _, n := range dup[v] {
			fmt.Fprintf(&b, "    %s\n", n)
		}
	}
	t.Fatal(b.String())
}

// TestNuevasPorEncimaDeLaAplicada es el modo de fallo 2 del issue #110.
//
// goose corre con allowMissing = false. Una migracion numerada por debajo de
// la version que produccion ya aplico no llega tarde: para el up en seco
// antes de aplicar nada, y como module.api depende de module.migrations el
// apply entero se cae.
//
// La version aplicada se deriva de MIGRACIONES_BASE_REF (main), no de una
// lista a mano: la que habia en migraciones_test se quedo en 00005 mientras
// produccion avanzaba a 00006 y luego a 00007, y la prueba paso en verde.
func TestNuevasPorEncimaDeLaAplicada(t *testing.T) {
	ref := migrations.BaseRef()
	desplegadas, err := migrations.DesplegadasEn(ref)
	if err != nil {
		t.Fatalf("%v", err)
	}
	aplicada, err := migrations.VersionAplicada(ref)
	if err != nil {
		t.Fatalf("%v", err)
	}
	malas, err := migrations.NuevasPorDebajo(migrations.FS, desplegadas)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(malas) == 0 {
		t.Logf("base %s aplicada en version %d (%d ficheros); ninguna nueva por debajo",
			ref, aplicada, len(desplegadas))
		return
	}
	t.Fatalf("migraciones nuevas con version <= %d (ya aplicada en %s); "+
		"goose las rechaza con allowMissing=false y tumban el deploy:\n  %s\n"+
		"renumerar al primer libre por encima de %d",
		aplicada, ref, strings.Join(malas, "\n  "), aplicada)
}

// Las dos comprobaciones tienen que fallar sobre un FS fabricado: si solo se
// miran contra el arbol real, un bug que las deje siempre en verde no se ve.
func TestNumeracionDetectaDuplicadoYHueco(t *testing.T) {
	t.Run("duplicado", func(t *testing.T) {
		fsys := fstest.MapFS{
			"00001_a.sql": {Data: []byte("-- +goose Up\n")},
			"00001_b.sql": {Data: []byte("-- +goose Up\n")},
			"00002_c.sql": {Data: []byte("-- +goose Up\n")},
		}
		dup, err := migrations.Duplicadas(fsys)
		if err != nil {
			t.Fatal(err)
		}
		if len(dup[1]) != 2 {
			t.Fatalf("version 1 duplicada = %v, se esperaban 2 ficheros", dup[1])
		}
	})

	t.Run("hueco por debajo", func(t *testing.T) {
		fsys := fstest.MapFS{
			"00001_init.sql":   {Data: []byte("-- +goose Up\n")},
			"00002_main.sql":   {Data: []byte("-- +goose Up\n")},
			"00005_cola.sql":   {Data: []byte("-- +goose Up\n")},
			"00003_tardia.sql": {Data: []byte("-- +goose Up\n")}, // nueva, version <= 5
			"00006_ok.sql":     {Data: []byte("-- +goose Up\n")}, // nueva, por encima: vale
		}
		desplegadas := []string{"00001_init.sql", "00002_main.sql", "00005_cola.sql"}
		malas, err := migrations.NuevasPorDebajo(fsys, desplegadas)
		if err != nil {
			t.Fatal(err)
		}
		if len(malas) != 1 || malas[0] != "00003_tardia.sql" {
			t.Fatalf("malas = %v, se esperaba solo 00003_tardia.sql", malas)
		}
	})
}
