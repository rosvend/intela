package postgres

import (
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// TestReplayDeReservaEsInmuneAQueLosParametrosCambienDespues es el caso
// central que #121 pide probar cruzado: congelar una corrida, y comprobar
// que liberar su reserva mas tarde reproduce las proporciones EXACTAS de
// entonces -- sin volver a leer ningun snapshot ni parametro. La firma de
// [aplicacion.BolsasAccesorias.LiberarReservaPrescrita] no acepta un
// Snapshot: estructuralmente no hay forma de que una tasa distinta, vigente
// anos despues, se cuele en el replay (RD 14.4, ADR 0005).
func TestReplayDeReservaEsInmuneAQueLosParametrosCambienDespues(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	// La corrida original: snapshot de 2026, tasa de reserva 5%, dos
	// titulares en 40/60.
	original := reparto.Resultado{
		Neto:       dec("950.00"),
		Admin:      dec("0.00"),
		Social:     dec("0.00"),
		Reserva:    dec("50.00"),
		SnapshotID: "snap-2026",
		Reglamento: "IX",
		Obras: []reparto.LineaObra{
			{ObraID: "obra-1", Puntos: dec("100"), Importe: dec("380.00")},
			{ObraID: "obra-2", Puntos: dec("150"), Importe: dec("570.00")},
		},
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("380.00")},
			{ObraID: "obra-2", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("570.00")},
		},
	}
	if err := s.GuardarResultado(ctx, "proceso-1", original); err != nil {
		t.Fatalf("congelar la corrida original: %v", err)
	}

	b := aplicacion.BolsasAccesorias{Resultados: s, Reservas: s}
	if _, err := b.RegistrarReserva(ctx, "proceso-1", reparto.Nacional, dec("5")); err != nil {
		t.Fatalf("registrar reserva: %v", err)
	}

	// "Los parametros cambiaron desde entonces" no tiene nada que simular
	// aqui: LiberarReservaPrescrita no recibe un Snapshot -- no hay adaptador
	// de ParametrosNormativos, fuera de alcance de #121 -- asi que no existe
	// un hueco en la firma por el que una tasa o ponderacion vigente HOY
	// pueda colarse en el replay. Lo que sigue prueba la otra mitad: que el
	// resultado es identico al proporcionado en 2026.
	nuevas, residuo, err := b.LiberarReservaPrescrita(ctx, "proceso-1", dec("0.00"))
	if err != nil {
		t.Fatalf("liberar reserva: %v", err)
	}

	if !nuevas[0].Importe.Equal(dec("20.00")) || !nuevas[1].Importe.Equal(dec("30.00")) {
		t.Fatalf("importes = %s, %s; se esperaba 40/60 de 50.00 = 20.00/30.00, exactos como en 2026",
			nuevas[0].Importe, nuevas[1].Importe)
	}
	if !residuo.IsZero() {
		t.Fatalf("residuo = %s, se esperaba cero", residuo)
	}

	// La corrida original en Postgres queda intacta: liberar la reserva no
	// revaloriza ni reescribe resultados_titular.
	releido, err := s.ResultadoPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("releer la corrida original: %v", err)
	}
	if releido.SnapshotID != "snap-2026" {
		t.Fatalf("SnapshotID = %q, se esperaba que la corrida original no cambiara", releido.SnapshotID)
	}
	for i, orig := range original.Titulares {
		if !releido.Titulares[i].Importe.Equal(orig.Importe) || !releido.Titulares[i].Porcentaje.Equal(orig.Porcentaje) {
			t.Fatalf("la linea %d de la corrida original cambio: %+v (era %+v)", i, releido.Titulares[i], orig)
		}
	}
}
