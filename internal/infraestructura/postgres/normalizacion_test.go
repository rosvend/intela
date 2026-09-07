package postgres

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/normalizacion"
)

func TestNormalizarUnLoteMixtoDejaCanonicasYRevision(t *testing.T) {
	s, pool := sembrarReportes(t)
	ctx := t.Context()
	n := aplicacion.Normalizacion{Reportes: s}
	p := normalizacion.Parametros{
		DuracionArtisticaPct: decimal.RequireFromString("0.80"),
		MinutosHoraTV:        decimal.NewFromInt(48),
		MonedaBase:           "COP",
		MonedasReconocidas:   []string{"COP", "USD", "EUR"},
	}
	rep := aplicacion.Reporte{ID: reporteEnero, Fuente: "caracol"}

	filas := []normalizacion.Fila{
		{
			ID: "tv-ok", Fuente: "caracol", Modalidad: "tv",
			Titulo: "Serie Y", Fecha: "20241231", Duracion: "60",
		},
		{
			ID: "cine-ok", Fuente: "caracol", Modalidad: "cine",
			Titulo: "Pelicula X", Fecha: "2024-06-01", Taquilla: "100", Moneda: "COP",
		},
		{
			ID: "tv-fecha", Fuente: "caracol", Modalidad: "tv",
			Titulo: "Fecha rota", Fecha: "ayer", Duracion: "30",
		},
		{
			ID: "cine-yen", Fuente: "caracol", Modalidad: "cine",
			Titulo: "Yen", Taquilla: "50", Moneda: "JPY",
		},
	}

	primero, err := n.ProcesarYGuardar(ctx, rep, filas, p)
	if err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	if len(primero.Normalizados) != 2 || len(primero.Revision) != 2 {
		t.Fatalf("primera pasada: %d normalizados, %d revision; se esperaba 2 y 2",
			len(primero.Normalizados), len(primero.Revision))
	}
	contar(t, ctx, pool, reporteEnero, 2, 2)

	leido, err := s.UsoPorID(ctx, "tv-ok")
	if err != nil {
		t.Fatalf("UsoPorID: %v", err)
	}
	if !leido.DuracionMin.Equal(decimal.RequireFromString("48")) {
		t.Fatalf("duracion persistida = %s, se esperaba 48", leido.DuracionMin)
	}

	if _, err := n.ProcesarYGuardar(ctx, rep, filas, p); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	contar(t, ctx, pool, reporteEnero, 2, 2)

	items, err := n.ListarRevision(ctx)
	if err != nil {
		t.Fatalf("ListarRevision: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("cola = %d, se esperaban 2", len(items))
	}
	for _, it := range items {
		if it.Tipo != aplicacion.TipoRevisionNormalizacion || it.Motivo == "" {
			t.Fatalf("item incompleto: %+v", it)
		}
	}
}
