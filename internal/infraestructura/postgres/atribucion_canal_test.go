package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/ingesta"
	"github.com/rosvend/intela/internal/infraestructura/objetos"
)

// TestElMapaDeCaracolRechazaLaFilaSinCanal: MapaCaracol no mapea canal_id.
// Antes la fila entraba a `usos` y UsosDeCanal la perdia sin error. Ahora el
// acuse la rechaza y nombra el campo (#165, P-20).
func TestElMapaDeCaracolRechazaLaFilaSinCanal(t *testing.T) {
	s, pool := sembrarReportes(t)
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

	csv := []byte("Titulo,ID_Ficha,Duracion_total,Fecha,Hora\n" +
		"La Casa de las Dos Palmas,1234,52,20260201,1930\n" +
		"La Casa de las Dos Palmas,1234,52,20260202,1930\n")

	const periodo = "2026-02"
	rec, err := ing.IngerirReporte(ctx, "caracol", "csv", periodo, csv)
	if err != nil {
		t.Fatalf("IngerirReporte con el mapa real de Caracol: %v", err)
	}
	if rec.Aceptados != 0 || len(rec.Rechazados) != 2 {
		t.Fatalf("aceptados=%d rechazados=%d, se esperaban 0 y 2", rec.Aceptados, len(rec.Rechazados))
	}
	for _, u := range rec.Rechazados {
		if !strings.Contains(u.RechazoMotivo, "canal_id") {
			t.Fatalf("motivo = %q, tenia que nombrar canal_id", u.RechazoMotivo)
		}
	}

	var enUsos int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM usos`).Scan(&enUsos); err != nil {
		t.Fatalf("contar usos: %v", err)
	}
	if enUsos != 0 {
		t.Fatalf("usos = %d, se esperaban 0: una fila sin canal no entra al reparto", enUsos)
	}

	sinCanal, err := s.UsosSinCanal(ctx, periodo)
	if err != nil {
		t.Fatalf("UsosSinCanal: %v", err)
	}
	if sinCanal != 0 {
		t.Fatalf("UsosSinCanal = %d, se esperaba 0: el rechazo no deja la fila en usos", sinCanal)
	}

	r := aplicacion.Reparto{Usos: s}
	if _, _, err := r.UsosDeCanal(ctx, periodo, ""); !errors.Is(err, aplicacion.ErrCanalVacio) {
		t.Fatalf("UsosDeCanal(\"\") se esperaba ErrCanalVacio, se obtuvo %v", err)
	}
}
