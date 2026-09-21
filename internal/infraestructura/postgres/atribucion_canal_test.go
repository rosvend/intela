package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/ingesta"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
)

// TestElMapaDeCaracolNoPueblaCanalIDYElHuecoEsObservable fija el estado real
// de la ingesta: MapaCaracol, MapaNetflix y MapaCine no mapean ninguna columna
// a CampoCanalID, asi que TODA fila que entra por el camino real de ingesta
// (IngerirReporte, no GuardarUsos con filas armadas a mano) llega con
// `canal_id = ”`. Sin un mecanismo aparte, UsosDeCanal("2025-02", "caracol")
// devuelve 0 usos SIN error, indistinguible de "el canal no emitio". Esta
// prueba comprueba que el hueco -- que sigue sin cablearse, P-20 en
// docs/dominio/preguntas-cliente.md -- es observable y que un canal vacio no
// se puede confundir con "todos los canales".
func TestElMapaDeCaracolNoPueblaCanalIDYElHuecoEsObservable(t *testing.T) {
	s, _ := sembrarReportes(t)
	ctx := t.Context()

	cat, err := ingesta.Catalogo(ingesta.MapaCaracol())
	if err != nil {
		t.Fatalf("construir el catalogo de lectores: %v", err)
	}

	ing := aplicacion.Ingesta{
		Reportes: s,
		Almacen:  objetos.Disco{Dir: t.TempDir()},
		Lectores: cat,
	}

	// Cabecera real de la parrilla de Caracol (fuentes.go): Titulo e ID_Ficha
	// son requeridas; Duracion_total tambien. Dos filas, un solo ID_Ficha
	// (misma obra, dos emisiones), como en la parrilla real.
	csv := []byte("Titulo,ID_Ficha,Duracion_total,Fecha,Hora\n" +
		"La Casa de las Dos Palmas,1234,52,20260201,1930\n" +
		"La Casa de las Dos Palmas,1234,52,20260202,1930\n")

	const periodo = "2026-02"
	if _, err := ing.IngerirReporte(ctx, "caracol", "csv", periodo, csv); err != nil {
		t.Fatalf("IngerirReporte con el mapa real de Caracol: %v", err)
	}

	// 1. El hueco real: las filas que entraron por el camino de produccion
	// llegan sin canal.
	sinCanal, err := s.UsosSinCanal(ctx, periodo)
	if err != nil {
		t.Fatalf("UsosSinCanal: %v", err)
	}
	if sinCanal != 2 {
		t.Fatalf("UsosSinCanal = %d, se esperaban 2: MapaCaracol no puebla canal_id "+
			"hasta que P-20 se resuelva", sinCanal)
	}

	// 2. Pedir el canal real por su nombre da vacio -- correcto, porque
	// efectivamente ninguna fila lo declara -- pero por si solo es ambiguo con
	// "el canal no emitio". Es el paso (1) el que lo desambigua.
	r := aplicacion.Reparto{Usos: s}
	usos, _, err := r.UsosDeCanal(ctx, periodo, "caracol")
	if err != nil {
		t.Fatalf("UsosDeCanal(canal real): %v", err)
	}
	if len(usos) != 0 {
		t.Fatalf("usos = %d, se esperaban 0: ninguna fila de esta entrega declara canal_id=%q",
			len(usos), "caracol")
	}

	// 3. Lo que NO puede pasar: pedir "el canal vacio" y que eso devuelva las
	// dos filas sin atribuir como si fueran de un pagador real.
	if _, _, err := r.UsosDeCanal(ctx, periodo, ""); !errors.Is(err, aplicacion.ErrCanalVacio) {
		t.Fatalf("UsosDeCanal(\"\") se esperaba ErrCanalVacio, se obtuvo (usos=%v, err=%v)", usos, err)
	}
}
