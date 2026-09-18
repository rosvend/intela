package reparto_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func snapBase() reparto.Snapshot {
	return reparto.Snapshot{
		AdminPct:              d("0"),
		SocialPct:             d("0"),
		ReservaPct:            d("0"),
		PondCine:              d("5.0"),
		PondUnitario:          d("2.8"),
		PondSerie:             d("1.3"),
		PondSketch:            d("0.8"),
		Wa:                    d("0.5"),
		Wb:                    d("0.3"),
		Wc:                    d("0.2"),
		GrupoPrivadosPct:      d("50"),
		GrupoRegionalesPct:    d("20"),
		GrupoPremiumPct:       d("10"),
		GrupoLideresPct:       d("10"),
		GrupoEstandarPct:      d("10"),
		AsignacionTercerosPct: d("5"),
		BaseCineTeatro:        reparto.BaseEspectadores,
		Reglamento:            "RD-IX",
	}
}

func bolsa(bruto string) reparto.Bolsa {
	b, err := recaudo.NuevaBolsa("u1", "2025", recaudo.Nacional, d(bruto))
	if err != nil {
		panic(err)
	}
	return b
}

func declCompleta(obraID, titular, ipi string) repertorio.Declaracion {
	dd, err := repertorio.NuevaDeclaracion(obraID, []repertorio.Parte{
		{TitularID: titular, IPI: ipi, Porcentaje: d("100")},
	})
	if err != nil {
		panic(err)
	}
	return dd
}

func importeObra(r reparto.Resultado, obraID string) decimal.Decimal {
	for _, o := range r.Obras {
		if o.ObraID == obraID {
			return o.Importe
		}
	}
	return decimal.Zero
}

func cierra(t *testing.T, r reparto.Resultado) {
	t.Helper()
	sumaT := decimal.Zero
	for _, tt := range r.Titulares {
		sumaT = sumaT.Add(tt.Importe)
	}
	got := sumaT.Add(r.Retenido).Add(r.Residuo)
	if !got.Equal(r.Neto) {
		t.Fatalf("cierre: titulares(%s)+retenido(%s)+residuo(%s)=%s != neto %s",
			sumaT, r.Retenido, r.Residuo, got, r.Neto)
	}
}

func TestReparto_CanalZ_RD911(t *testing.T) {
	t.Parallel()
	// formulas.md: Pelicula X 1*5*70*4.5=1575; Serie Y 10*1.3*48*9=5616; total 7191.
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica", CanalID: "z",
			DuracionMin: d("70"), Emisiones: 1, Rating: d("4.5")},
		{ObraID: "y", Modalidad: reparto.TV, TipoObra: "serie", CanalID: "z",
			DuracionMin: d("48"), Emisiones: 10, Rating: d("9")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	r, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)

	totalPuntos := d("7191")
	wantVP := d("1000000").Div(totalPuntos).Round(8)
	if !r.ValorPunto.Equal(wantVP) {
		t.Fatalf("valor punto=%s, want %s (reglamento muestra ≈139,1)", r.ValorPunto, wantVP)
	}
	// Asignacion exacta con Round(2): 1000000*1575/7191 → 219023.78;
	// 1000000*5616/7191 → 780976.22. El ejemplo del reglamento redondea el
	// valor punto a un decimal (139,1) y muestra 219.024 / 780.976.
	if !importeObra(r, "x").Equal(d("219023.78")) {
		t.Fatalf("obra x=%s, want 219023.78", importeObra(r, "x"))
	}
	if !importeObra(r, "y").Equal(d("780976.22")) {
		t.Fatalf("obra y=%s, want 780976.22", importeObra(r, "y"))
	}
}

func TestReparto_Cine_RD92(t *testing.T) {
	t.Parallel()
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.Cine, Espectadores: d("10000")},
		{ObraID: "y", Modalidad: reparto.Cine, Espectadores: d("5000")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	r, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	if !importeObra(r, "x").Equal(d("666666.67")) {
		t.Fatalf("x=%s", importeObra(r, "x"))
	}
	if !importeObra(r, "y").Equal(d("333333.33")) {
		t.Fatalf("y=%s", importeObra(r, "y"))
	}
}

func TestReparto_TeatroIgualCine(t *testing.T) {
	t.Parallel()
	base := []reparto.Uso{
		{ObraID: "x", Espectadores: d("10000")},
		{ObraID: "y", Espectadores: d("5000")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	cine := make([]reparto.Uso, len(base))
	teatro := make([]reparto.Uso, len(base))
	for i := range base {
		cine[i] = base[i]
		cine[i].Modalidad = reparto.Cine
		teatro[i] = base[i]
		teatro[i].Modalidad = reparto.Teatro
	}
	rc, err := reparto.Reparto(bolsa("1000000"), cine, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := reparto.Reparto(bolsa("1000000"), teatro, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	if !importeObra(rc, "x").Equal(importeObra(rt, "x")) || !importeObra(rc, "y").Equal(importeObra(rt, "y")) {
		t.Fatalf("cine x=%s y=%s; teatro x=%s y=%s",
			importeObra(rc, "x"), importeObra(rc, "y"),
			importeObra(rt, "x"), importeObra(rt, "y"))
	}
}

func TestReparto_TransportePorExhibiciones(t *testing.T) {
	t.Parallel()
	usos := []reparto.Uso{
		{ObraID: "a", Modalidad: reparto.Transporte, Exhibiciones: 10},
		{ObraID: "b", Modalidad: reparto.Transporte, Exhibiciones: 0},
		{ObraID: "c", Modalidad: reparto.Transporte, Exhibiciones: 30},
	}
	decls := []repertorio.Declaracion{
		declCompleta("a", "ta", "IPI-A"),
		declCompleta("b", "tb", "IPI-B"),
		declCompleta("c", "tc", "IPI-C"),
	}
	r, err := reparto.Reparto(bolsa("400"), usos, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	if !importeObra(r, "b").IsZero() {
		t.Fatalf("obra con 0 exhibiciones recibio %s", importeObra(r, "b"))
	}
	if !importeObra(r, "a").Equal(d("100.00")) {
		t.Fatalf("a=%s want 100", importeObra(r, "a"))
	}
	if !importeObra(r, "c").Equal(d("300.00")) {
		t.Fatalf("c=%s want 300", importeObra(r, "c"))
	}
}

func TestReparto_R04_RetieneCompleto(t *testing.T) {
	t.Parallel()
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.Cine, Espectadores: d("1")},
	}
	incompleta, err := repertorio.NuevaDeclaracion("x", []repertorio.Parte{
		{TitularID: "t", IPI: "IPI", Porcentaje: d("99")},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), []repertorio.Declaracion{incompleta}, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Titulares) != 0 {
		t.Fatalf("titulares=%d", len(r.Titulares))
	}
	if !r.Retenido.Equal(r.Neto.Sub(r.Residuo)) && !r.Retenido.Equal(d("1000.00")) {
		// neto 1000, una sola obra, residuo 0, retenido 1000
		if !r.Retenido.Equal(d("1000")) && !r.Retenido.Equal(d("1000.00")) {
			t.Fatalf("retenido=%s", r.Retenido)
		}
	}
	cierra(t, r)
}

func TestReparto_OTT_CoeficienteAusente(t *testing.T) {
	t.Parallel()
	snap := snapBase()
	snap.Wa = decimal.Zero
	usos := []reparto.Uso{{ObraID: "x", Modalidad: reparto.OTT, PB: d("1"), MinutosVistos: d("1"), Vistas: d("1")}}
	_, err := reparto.Reparto(bolsa("100"), usos, snap, nil, reparto.Opciones{})
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("err=%v", err)
	}
}

func TestReparto_ModalidadDesconocida(t *testing.T) {
	t.Parallel()
	usos := []reparto.Uso{{ObraID: "x", Modalidad: "radio"}}
	_, err := reparto.Reparto(bolsa("100"), usos, snapBase(), nil, reparto.Opciones{})
	if !errors.Is(err, reparto.ErrModalidadDesconocida) {
		t.Fatalf("err=%v", err)
	}
}

func TestReparto_Suscripcion_SplitYValorPuntoPorGrupo(t *testing.T) {
	t.Parallel()
	// Neto 100 (sin deducciones). Split 50/20/10/10/10.
	// Grupo privados: obra A con 1 punto → se lleva 50.
	// Grupo regionales: obra B con 1 punto → 20.
	// Premium/lideres/estandar vacios → sus partes van a residuo (30).
	usos := []reparto.Uso{
		{
			ObraID: "a", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
			Grupo: reparto.GrupoPrivadosNacionales, CanalID: "c1",
			DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
		},
		{
			ObraID: "b", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
			Grupo: reparto.GrupoRegionalesPublicos, CanalID: "c2",
			DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
		},
	}
	decls := []repertorio.Declaracion{
		declCompleta("a", "ta", "IPI-A"),
		declCompleta("b", "tb", "IPI-B"),
	}
	// Bruto 100.03 fuerza residuo de centavos en el split 50/20/10/10/10.
	r, err := reparto.Reparto(bolsa("100.03"), usos, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)

	partes := []decimal.Decimal{
		importeObra(r, "a"),
		importeObra(r, "b"),
	}
	sumaPartes := partes[0].Add(partes[1]).Add(r.Residuo)
	if !sumaPartes.Equal(r.Neto) && !partes[0].Add(partes[1]).Add(r.Residuo).Equal(r.Neto) {
		t.Fatalf("a+b+residuo=%s+%s+%s != neto %s", partes[0], partes[1], r.Residuo, r.Neto)
	}
	// Valor punto por grupo distinto: mismos puntos (2.8*1*1*1=2.8) pero bolsas 50% vs 20%.
	if importeObra(r, "a").LessThanOrEqual(importeObra(r, "b")) {
		t.Fatalf("privados deberia pagar mas que regionales: a=%s b=%s",
			importeObra(r, "a"), importeObra(r, "b"))
	}
}

func TestReparto_Suscripcion_R27_Orden(t *testing.T) {
	t.Parallel()
	// Mismo grupo: una obra en repertorio y otra fuera. Si se excluye ANTES,
	// la obra buena se lleva toda la bolsa del grupo. Si se excluye DESPUES,
	// compiten por puntos y la parte de la fuera cae a residuo.
	usos := []reparto.Uso{
		{
			ObraID: "buena", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
			Grupo: reparto.GrupoPrivadosNacionales, CanalID: "ok",
			DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
		},
		{
			ObraID: "fuera", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
			Grupo: reparto.GrupoPrivadosNacionales, CanalID: "bad",
			DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
			FueraDeRepertorio: true,
		},
	}
	decls := []repertorio.Declaracion{
		declCompleta("buena", "t1", "IPI-1"),
		declCompleta("fuera", "t2", "IPI-2"),
	}

	antes, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	despues, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), decls, reparto.Opciones{
		OmitirExclusionAntesDeSplit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if importeObra(antes, "fuera").GreaterThan(decimal.Zero) {
		t.Fatalf("antes: fuera no deberia cobrar, got %s", importeObra(antes, "fuera"))
	}
	if importeObra(despues, "fuera").GreaterThan(decimal.Zero) {
		t.Fatalf("despues: fuera filtrado, got %s", importeObra(despues, "fuera"))
	}
	if !importeObra(antes, "buena").GreaterThan(importeObra(despues, "buena")) {
		t.Fatalf("excluir antes debio dar mas a la obra en repertorio: antes=%s despues=%s",
			importeObra(antes, "buena"), importeObra(despues, "buena"))
	}
}

func TestReparto_Suscripcion_ClasificacionAnioAnterior(t *testing.T) {
	t.Parallel()
	// Misma obra/canal; la clasificacion efectiva (Grupo) la fija quien llama
	// segun canales_clasificacion del ano anterior. Reejecutar 2024 usa 2023.
	uso2024 := reparto.Uso{
		ObraID: "x", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
		Grupo: reparto.GrupoLideresRating, CanalID: "c",
		DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
	}
	uso2025 := uso2024
	uso2025.Grupo = reparto.GrupoEstandar // reclasificado con audiencia 2024

	decls := []repertorio.Declaracion{declCompleta("x", "t", "IPI")}
	r24, err := reparto.Reparto(bolsa("1000"), []reparto.Uso{uso2024}, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	r25, err := reparto.Reparto(bolsa("1000"), []reparto.Uso{uso2025}, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	// Lideres y estandar tienen el mismo % (10), misma obra → mismo importe.
	// Lo que se prueba es que el Grupo viaja como dato del uso (vigencia externa).
	if uso2024.Grupo == uso2025.Grupo {
		t.Fatal("fixture de clasificacion no cambio")
	}
	if !importeObra(r24, "x").Equal(importeObra(r25, "x")) {
		t.Fatalf("mismo pct de grupo deberia dar mismo importe: %s vs %s",
			importeObra(r24, "x"), importeObra(r25, "x"))
	}
	_ = r24
}

func TestAsignarPlataformaTerceros(t *testing.T) {
	t.Parallel()
	parent := []reparto.LineaObra{
		{ObraID: "x", Importe: d("600"), Puntos: d("6")},
		{ObraID: "y", Importe: d("400"), Puntos: d("4")},
	}
	r, err := reparto.AsignarPlataformaTerceros(parent, d("1000"), snapBase())
	if err != nil {
		t.Fatal(err)
	}
	// 5% de 1000 = 50; 60/40 → 30 / 20.
	if !r.Neto.Equal(d("50.00")) {
		t.Fatalf("pool=%s", r.Neto)
	}
	if !importeObra(r, "x").Equal(d("30.00")) || !importeObra(r, "y").Equal(d("20.00")) {
		t.Fatalf("x=%s y=%s", importeObra(r, "x"), importeObra(r, "y"))
	}
}

func TestReparto_Reproducible(t *testing.T) {
	t.Parallel()
	usos := []reparto.Uso{
		{ObraID: "y", Modalidad: reparto.TV, TipoObra: "serie", CanalID: "z",
			DuracionMin: d("48"), Emisiones: 10, Rating: d("9")},
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica", CanalID: "z",
			DuracionMin: d("70"), Emisiones: 1, Rating: d("4.5")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	a, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, reparto.Opciones{SnapshotID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, reparto.Opciones{SnapshotID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.ValorPunto.Equal(b.ValorPunto) || !importeObra(a, "x").Equal(importeObra(b, "x")) {
		t.Fatal("dos corridas identicas divergieron")
	}
}

func TestReparto_HotelUsaSuscripcion(t *testing.T) {
	t.Parallel()
	usos := []reparto.Uso{{
		ObraID: "a", Modalidad: reparto.Hotel, TipoObra: "unitario",
		Grupo: reparto.GrupoPrivadosNacionales, CanalID: "c1",
		DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
	}}
	decls := []repertorio.Declaracion{declCompleta("a", "ta", "IPI-A")}
	r, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), decls, reparto.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	// Solo grupo privados tiene obras: cobra el 50% del neto; el resto es residuo.
	if !importeObra(r, "a").Equal(d("500.00")) {
		t.Fatalf("hotel/privados=%s want 500", importeObra(r, "a"))
	}
}

func TestReparto_SinLiteralesDeGrupoEnSnapshotVacio(t *testing.T) {
	t.Parallel()
	snap := snapBase()
	snap.GrupoPrivadosPct = decimal.Zero
	usos := []reparto.Uso{{
		ObraID: "a", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
		Grupo:       reparto.GrupoPrivadosNacionales,
		DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
	}}
	_, err := reparto.Reparto(bolsa("100"), usos, snap, nil, reparto.Opciones{})
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("err=%v", err)
	}
}
