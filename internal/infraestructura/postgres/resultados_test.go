package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// sembrarCorridaBase deja una bolsa, un proceso, dos obras y dos titulares
// listos para que resultados_obra/resultados_titular referencien por FK.
func sembrarCorridaBase(t *testing.T) *Store {
	t.Helper()
	pool := testhelp.Pool(t)
	s := &Store{pool: pool}
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('usuario-1', 'Usuario 1', 'tv_abierta')`); err != nil {
		t.Fatalf("sembrar usuario_recaudo: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
		 VALUES ('bolsa-1', 'usuario-1', '2026-01', 'nacional', 1000.00)`); err != nil {
		t.Fatalf("sembrar bolsa: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
		 VALUES ('proceso-1', 'nacional', 'importe_titular', '2026-01', 'bolsa-1', 'snap-1', 'IX')`); err != nil {
		t.Fatalf("sembrar proceso: %v", err)
	}
	for _, obraID := range []string{"obra-1", "obra-2"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ($1, $2, 'Drama', 2026, 'serie')`,
			obraID, "Obra "+obraID); err != nil {
			t.Fatalf("sembrar %q: %v", obraID, err)
		}
	}
	for _, tt := range []struct{ id, ipi string }{
		{"titular-a", "111"},
		{"titular-b", "222"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO titulares (id, nombre, ipi, clase) VALUES ($1, $2, $3, 'socio')`,
			tt.id, "Titular "+tt.id, tt.ipi); err != nil {
			t.Fatalf("sembrar %q: %v", tt.id, err)
		}
	}
	return s
}

func resultadoDeCorridaBase() reparto.Resultado {
	return reparto.Resultado{
		Neto:       dec("850.00"),
		Admin:      dec("100.00"),
		Social:     dec("50.00"),
		Reserva:    dec("0.00"),
		Retenido:   dec("0.00"),
		Residuo:    dec("0.01"),
		ValorPunto: dec("1.5"),
		SnapshotID: "snap-1",
		Reglamento: "IX",
		Obras: []reparto.LineaObra{
			{ObraID: "obra-1", Puntos: dec("300"), Importe: dec("400.00")},
			{ObraID: "obra-2", Puntos: dec("225"), Importe: dec("300.00")},
		},
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-1", TitularID: "titular-a", IPI: "111", Porcentaje: dec("40"), Importe: dec("160.00")},
			{ObraID: "obra-1", TitularID: "titular-b", IPI: "222", Porcentaje: dec("60"), Importe: dec("240.00")},
			{ObraID: "obra-2", TitularID: "titular-a", IPI: "111", Porcentaje: dec("100"), Importe: dec("300.00")},
		},
	}
}

// TestResultadosGuardarYPorProcesoReproducenLasProporciones es el caso que
// #121 necesita de verdad: releer de Postgres las lineas de titular de una
// corrida ya cerrada, sin perder la proporcion que el replay va a usar
// (RD 14.4, RD 10.1).
func TestResultadosGuardarYPorProcesoReproducenLasProporciones(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()
	r := resultadoDeCorridaBase()

	if err := s.GuardarResultado(ctx, "proceso-1", r); err != nil {
		t.Fatalf("guardar resultado: %v", err)
	}

	leido, err := s.ResultadoPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer resultado: %v", err)
	}

	if !leido.Neto.Equal(r.Neto) || !leido.Admin.Equal(r.Admin) || !leido.Social.Equal(r.Social) ||
		!leido.Reserva.Equal(r.Reserva) || !leido.Retenido.Equal(r.Retenido) || !leido.Residuo.Equal(r.Residuo) ||
		!leido.NoDistribuido.Equal(r.NoDistribuido) || !leido.ValorPunto.Equal(r.ValorPunto) {
		t.Fatalf("los agregados no cuadran: leido=%+v, original=%+v", leido, r)
	}
	// Esta corrida no reparte suscripcion: las tres estructuras viajan vacias
	// y tienen que volver vacias. Un nil que se vuelve lista inventada
	// cambiaria el replay.
	if len(leido.PartesNoDistribuidas) != 0 || len(leido.PorGrupo) != 0 {
		t.Fatalf("se esperaban partes y grupos vacios, leido=%+v", leido)
	}
	if leido.SnapshotID != r.SnapshotID || leido.Reglamento != r.Reglamento {
		t.Fatalf("procedencia perdida: leido=%+v", leido)
	}

	if len(leido.Obras) != 2 {
		t.Fatalf("se esperaban 2 lineas de obra, hubo %d", len(leido.Obras))
	}
	if len(leido.Titulares) != 3 {
		t.Fatalf("se esperaban 3 lineas de titular, hubo %d", len(leido.Titulares))
	}
	// La proporcion que el replay va a usar es el Importe original: tiene que
	// sobrevivir exacto al round-trip.
	for i, orig := range r.Titulares {
		got := leido.Titulares[i]
		if got.ObraID != orig.ObraID || got.TitularID != orig.TitularID || !got.Importe.Equal(orig.Importe) {
			t.Fatalf("linea %d cambio: leido=%+v, original=%+v", i, got, orig)
		}
	}
}

func TestResultadosPorProcesoSinCorridaEsNoEncontrado(t *testing.T) {
	s := sembrarCorridaBase(t)
	_, err := s.ResultadoPorProceso(t.Context(), "proceso-que-no-existe")
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("error = %v, se esperaba ErrNoEncontrado", err)
	}
}

// TestGuardarResultadoParticipaEnLaUnidadAmbiente reproduce el hallazgo de
// revision de #159: GuardarResultado usaba s.EnTransaccion, que SIEMPRE abre
// una transaccion nueva contra el pool sin mirar ctx. Envuelto en un
// aplicacion.Procesos.AvanzarEtapa que ata valorizar+GuardarProceso a una
// UnidadDeTrabajo, el resultado quedaba committeado de todos modos aunque el
// resto de la unidad revirtiera -- exactamente el bug que ese commit decia
// haber corregido.
func TestGuardarResultadoParticipaEnLaUnidadAmbiente(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()
	r := resultadoDeCorridaBase()
	fallo := errors.New("el caso de uso de fuera se arrepintio")

	err := s.EnUnidad(ctx, func(ctx context.Context) error {
		if err := s.GuardarResultado(ctx, "proceso-1", r); err != nil {
			return err
		}
		return fallo
	})
	if !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error de fuera, se obtuvo %v", err)
	}

	if _, err := s.ResultadoPorProceso(ctx, "proceso-1"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("el resultado sobrevivio al rollback de la unidad de fuera: %v", err)
	}
}

// TestResultadosRoundTripConservaNoDistribuidoPartesYGrupos es el caso de
// #162: un importe que el motor no reparte (grupo vacio, exclusion R-27,
// peso cero) tiene que sobrevivir a Postgres, con cada tramo y con el valor
// punto de cada grupo. Antes de la migracion 00018 GuardarResultado lo
// descartaba y la releida volvia con los tres en cero.
//
// La suma es la misma que cierra() en el motor de #33:
//
//	titulares + retenido + residuo + NoDistribuido == Neto
//
// y la suma de las partes tiene que ser exactamente NoDistribuido (RD 16).
func TestResultadosRoundTripConservaNoDistribuidoPartesYGrupos(t *testing.T) {
	s := sembrarCorridaBase(t)
	ctx := t.Context()

	// 700 de titulares (la corrida base) + 119.99 retenido + 0.01 de
	// residuo + 30 no distribuidos = 850 de neto.
	r := resultadoDeCorridaBase()
	r.Retenido = dec("119.99")
	r.NoDistribuido = dec("30.00")
	r.PartesNoDistribuidas = []reparto.ParteNoDistribuida{
		{
			Motivo:  reparto.MotivoExclusionR27,
			Grupo:   reparto.GrupoPrivadosNacionales,
			ObraID:  "obra-1",
			Importe: dec("10.00"),
		},
		{
			Motivo:  reparto.MotivoGrupoSinObras,
			Grupo:   reparto.GrupoEstandar,
			Importe: dec("15.00"),
		},
		{
			Motivo:  reparto.MotivoPesoCero,
			Importe: dec("5.00"),
		},
	}
	r.PorGrupo = []reparto.LineaGrupo{
		{
			Grupo:       reparto.GrupoPrivadosNacionales,
			Bolsa:       dec("50.00"),
			TotalPuntos: dec("2.8"),
			ValorPunto:  dec("17.85714286"),
			Residuo:     dec("0.01"),
		},
		{
			Grupo:       reparto.GrupoPremium,
			Bolsa:       dec("0.00"),
			TotalPuntos: dec("0"),
			ValorPunto:  dec("0"),
			Residuo:     dec("-0.01"),
		},
	}
	assertCierreResultado(t, r)

	if err := s.GuardarResultado(ctx, "proceso-1", r); err != nil {
		t.Fatalf("guardar resultado: %v", err)
	}
	leido, err := s.ResultadoPorProceso(ctx, "proceso-1")
	if err != nil {
		t.Fatalf("leer resultado: %v", err)
	}
	assertCierreResultado(t, leido)

	if !leido.NoDistribuido.Equal(r.NoDistribuido) {
		t.Fatalf("NoDistribuido = %s, se esperaba %s", leido.NoDistribuido, r.NoDistribuido)
	}
	if len(leido.PartesNoDistribuidas) != len(r.PartesNoDistribuidas) {
		t.Fatalf("partes = %d, se esperaban %d", len(leido.PartesNoDistribuidas), len(r.PartesNoDistribuidas))
	}
	for i, orig := range r.PartesNoDistribuidas {
		got := leido.PartesNoDistribuidas[i]
		if got.Motivo != orig.Motivo || got.Grupo != orig.Grupo || got.ObraID != orig.ObraID ||
			!got.Importe.Equal(orig.Importe) {
			t.Fatalf("parte %d cambio: leido=%+v, original=%+v", i, got, orig)
		}
	}
	if !sumaPartes(leido.PartesNoDistribuidas).Equal(leido.NoDistribuido) {
		t.Fatalf("las partes suman %s y NoDistribuido es %s",
			sumaPartes(leido.PartesNoDistribuidas), leido.NoDistribuido)
	}

	if len(leido.PorGrupo) != len(r.PorGrupo) {
		t.Fatalf("grupos = %d, se esperaban %d", len(leido.PorGrupo), len(r.PorGrupo))
	}
	for i, orig := range r.PorGrupo {
		got := leido.PorGrupo[i]
		if got.Grupo != orig.Grupo || !got.Bolsa.Equal(orig.Bolsa) ||
			!got.TotalPuntos.Equal(orig.TotalPuntos) || !got.ValorPunto.Equal(orig.ValorPunto) ||
			!got.Residuo.Equal(orig.Residuo) {
			t.Fatalf("grupo %d cambio: leido=%+v, original=%+v", i, got, orig)
		}
	}
	// El orden guardado es el del motor, no el alfabetico: premium iria
	// antes que privados si se ordenara por el nombre del grupo.
	if leido.PorGrupo[0].Grupo != reparto.GrupoPrivadosNacionales {
		t.Fatalf("el primer grupo es %q, se esperaba privado_nacional", leido.PorGrupo[0].Grupo)
	}
}

func assertCierreResultado(t *testing.T, r reparto.Resultado) {
	t.Helper()
	sumaT := decimal.Zero
	for _, tt := range r.Titulares {
		sumaT = sumaT.Add(tt.Importe)
	}
	got := sumaT.Add(r.Retenido).Add(r.Residuo).Add(r.NoDistribuido)
	if !got.Equal(r.Neto) {
		t.Fatalf("cierre: titulares(%s)+retenido(%s)+residuo(%s)+noDist(%s)=%s != neto %s",
			sumaT, r.Retenido, r.Residuo, r.NoDistribuido, got, r.Neto)
	}
}

func sumaPartes(partes []reparto.ParteNoDistribuida) decimal.Decimal {
	suma := decimal.Zero
	for _, p := range partes {
		suma = suma.Add(p.Importe)
	}
	return suma
}
