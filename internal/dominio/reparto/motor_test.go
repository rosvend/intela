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
		AdminPct:              d("20"),
		SocialPct:             d("10"),
		ReservaPct:            d("5"),
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

// optSinDed salta deducciones para que los ejemplos del reglamento cierren
// sobre el bruto entero (R-16 / SinDeducciones).
func optSinDed() reparto.Opciones { return reparto.Opciones{SinDeducciones: true} }

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
	got := sumaT.Add(r.Retenido).Add(r.Residuo).Add(r.NoDistribuido)
	if !got.Equal(r.Neto) {
		t.Fatalf("cierre: titulares(%s)+retenido(%s)+residuo(%s)+noDist(%s)=%s != neto %s",
			sumaT, r.Retenido, r.Residuo, r.NoDistribuido, got, r.Neto)
	}
}

func TestReparto_CanalZ_RD911(t *testing.T) {
	casos := []struct {
		nombre string
	}{{nombre: "ejemplo reglamento"}}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
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
			r, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
			if err != nil {
				t.Fatal(err)
			}
			cierra(t, r)

			totalPuntos := d("7191")
			wantVP := d("1000000").Div(totalPuntos).Round(8)
			if !r.ValorPunto.Equal(wantVP) {
				t.Fatalf("valor punto=%s, want %s", r.ValorPunto, wantVP)
			}
			if !importeObra(r, "x").Equal(d("219023.78")) {
				t.Fatalf("obra x=%s, want 219023.78", importeObra(r, "x"))
			}
			if !importeObra(r, "y").Equal(d("780976.22")) {
				t.Fatalf("obra y=%s, want 780976.22", importeObra(r, "y"))
			}
		})
	}
}

func TestReparto_Cine_RD92(t *testing.T) {
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.Cine, Espectadores: d("10000")},
		{ObraID: "y", Modalidad: reparto.Cine, Espectadores: d("5000")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	r, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, optSinDed())
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

func TestReparto_Cine_BaseTaquilla(t *testing.T) {
	snap := snapBase()
	snap.BaseCineTeatro = reparto.BaseTaquilla
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.Cine, Taquilla: d("800")},
		{ObraID: "y", Modalidad: reparto.Cine, Taquilla: d("200")},
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	r, err := reparto.Reparto(bolsa("1000"), usos, snap, decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	if !importeObra(r, "x").Equal(d("800.00")) || !importeObra(r, "y").Equal(d("200.00")) {
		t.Fatalf("x=%s y=%s", importeObra(r, "x"), importeObra(r, "y"))
	}
}

func TestReparto_TeatroIgualCine(t *testing.T) {
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
	rc, err := reparto.Reparto(bolsa("1000000"), cine, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	rt, err := reparto.Reparto(bolsa("1000000"), teatro, snapBase(), decls, optSinDed())
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
	r, err := reparto.Reparto(bolsa("400"), usos, snapBase(), decls, optSinDed())
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
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.Cine, Espectadores: d("1")},
	}
	incompleta, err := repertorio.NuevaDeclaracion("x", []repertorio.Parte{
		{TitularID: "t", IPI: "IPI", Porcentaje: d("99")},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), []repertorio.Declaracion{incompleta}, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Titulares) != 0 {
		t.Fatalf("titulares=%d", len(r.Titulares))
	}
	if !r.Retenido.Equal(d("1000")) && !r.Retenido.Equal(d("1000.00")) {
		t.Fatalf("retenido=%s", r.Retenido)
	}
	cierra(t, r)
}

func TestReparto_OTT_CoeficienteAusente(t *testing.T) {
	snap := snapBase()
	snap.Wa = decimal.Zero
	usos := []reparto.Uso{{ObraID: "x", Modalidad: reparto.OTT, PB: d("1"), MinutosVistos: d("1"), Vistas: d("1")}}
	_, err := reparto.Reparto(bolsa("100"), usos, snap, nil, optSinDed())
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("err=%v", err)
	}
}

func TestReparto_ModalidadDesconocida(t *testing.T) {
	usos := []reparto.Uso{{ObraID: "x", Modalidad: "radio"}}
	_, err := reparto.Reparto(bolsa("100"), usos, snapBase(), nil, optSinDed())
	if !errors.Is(err, reparto.ErrModalidadDesconocida) {
		t.Fatalf("err=%v", err)
	}
}

func TestReparto_Suscripcion_GrupoVacio(t *testing.T) {
	usos := []reparto.Uso{{
		ObraID: "a", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
		Grupo: "", DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
	}}
	_, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), nil, optSinDed())
	if err == nil {
		t.Fatal("esperado error por grupo vacio")
	}
	if !errors.Is(err, reparto.ErrRepartoInvalido) && !errors.Is(err, reparto.ErrGrupoDesconocido) {
		t.Fatalf("err=%v", err)
	}
}

func TestReparto_Suscripcion_SplitYValorPuntoPorGrupo(t *testing.T) {
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
	r, err := reparto.Reparto(bolsa("100.00"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)

	if !importeObra(r, "a").Equal(d("50.00")) {
		t.Fatalf("privados a=%s want 50", importeObra(r, "a"))
	}
	if !importeObra(r, "b").Equal(d("20.00")) {
		t.Fatalf("regionales b=%s want 20", importeObra(r, "b"))
	}
	if !r.NoDistribuido.Equal(d("30.00")) {
		t.Fatalf("grupos vacios noDist=%s want 30", r.NoDistribuido)
	}
	if !r.Residuo.IsZero() {
		t.Fatalf("residuo de redondeo deberia ser 0, got %s", r.Residuo)
	}

	porGrupo := map[reparto.GrupoCanal]reparto.LineaGrupo{}
	for _, g := range r.PorGrupo {
		porGrupo[g.Grupo] = g
	}
	if !porGrupo[reparto.GrupoPrivadosNacionales].ValorPunto.Equal(
		d("50").Div(d("2.8")).Round(8),
	) {
		t.Fatalf("valor punto privados=%s", porGrupo[reparto.GrupoPrivadosNacionales].ValorPunto)
	}
	if !porGrupo[reparto.GrupoRegionalesPublicos].ValorPunto.Equal(
		d("20").Div(d("2.8")).Round(8),
	) {
		t.Fatalf("valor punto regionales=%s", porGrupo[reparto.GrupoRegionalesPublicos].ValorPunto)
	}
}

func TestReparto_Suscripcion_R27_ExcluyeAntes(t *testing.T) {
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

	r, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	if importeObra(r, "fuera").GreaterThan(decimal.Zero) {
		t.Fatalf("fuera no deberia cobrar, got %s", importeObra(r, "fuera"))
	}
	// 50% del neto al grupo privados; la obra en repertorio se lo lleva entero.
	if !importeObra(r, "buena").Equal(d("500.00")) {
		t.Fatalf("buena=%s want 500", importeObra(r, "buena"))
	}
}

func TestAsignarPlataformaTerceros(t *testing.T) {
	parent := []reparto.LineaObra{
		{ObraID: "x", Importe: d("600"), Puntos: d("6")},
		{ObraID: "y", Importe: d("400"), Puntos: d("4")},
		{ObraID: "z", Importe: d("100"), Puntos: d("1")}, // no comunicada
	}
	decls := []repertorio.Declaracion{
		declCompleta("x", "tx", "IPI-X"),
		declCompleta("y", "ty", "IPI-Y"),
	}
	r, err := reparto.AsignarPlataformaTerceros(
		parent, []string{"x", "y"}, d("1000"), recaudo.Nacional, snapBase(), decls, optSinDed(),
	)
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	// 5% de 1000 = 50; proporciones 600:400 → 30 / 20.
	if !r.Neto.Equal(d("50.00")) {
		t.Fatalf("pool=%s", r.Neto)
	}
	if !importeObra(r, "x").Equal(d("30.00")) || !importeObra(r, "y").Equal(d("20.00")) {
		t.Fatalf("x=%s y=%s", importeObra(r, "x"), importeObra(r, "y"))
	}
	if importeObra(r, "z").GreaterThan(decimal.Zero) {
		t.Fatalf("z no comunicada cobro %s", importeObra(r, "z"))
	}
}

func TestAsignarPlataformaTerceros_R04(t *testing.T) {
	parent := []reparto.LineaObra{
		{ObraID: "ok", Importe: d("600"), Puntos: d("6")},
		{ObraID: "ret", Importe: d("400"), Puntos: d("4"), Retenida: true},
	}
	incompleta, err := repertorio.NuevaDeclaracion("ret", []repertorio.Parte{
		{TitularID: "t", IPI: "IPI", Porcentaje: d("99")},
	})
	if err != nil {
		t.Fatal(err)
	}
	decls := []repertorio.Declaracion{
		declCompleta("ok", "to", "IPI-OK"),
		incompleta,
	}
	r, err := reparto.AsignarPlataformaTerceros(
		parent, []string{"ok", "ret"}, d("1000"), recaudo.Nacional, snapBase(), decls, optSinDed(),
	)
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	for _, o := range r.Obras {
		if o.ObraID == "ret" && !o.Retenida {
			t.Fatal("obra con declaracion incompleta salio sin Retenida")
		}
	}
	if r.Retenido.IsZero() {
		t.Fatal("R-04 debio retener la parte de ret")
	}
}

func TestReparto_Reproducible(t *testing.T) {
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
	opt := reparto.Opciones{SnapshotID: "s1", SinDeducciones: true}
	a, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, opt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := reparto.Reparto(bolsa("1000000"), usos, snapBase(), decls, opt)
	if err != nil {
		t.Fatal(err)
	}
	if !a.ValorPunto.Equal(b.ValorPunto) || !importeObra(a, "x").Equal(importeObra(b, "x")) {
		t.Fatal("dos corridas identicas divergieron")
	}
}

func TestReparto_HotelUsaSuscripcion(t *testing.T) {
	usos := []reparto.Uso{{
		ObraID: "a", Modalidad: reparto.Hotel, TipoObra: "unitario",
		Grupo: reparto.GrupoPrivadosNacionales, CanalID: "c1",
		DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
	}}
	decls := []repertorio.Declaracion{declCompleta("a", "ta", "IPI-A")}
	r, err := reparto.Reparto(bolsa("1000"), usos, snapBase(), decls, optSinDed())
	if err != nil {
		t.Fatal(err)
	}
	cierra(t, r)
	if !importeObra(r, "a").Equal(d("500.00")) {
		t.Fatalf("hotel/privados=%s want 500", importeObra(r, "a"))
	}
	if !r.NoDistribuido.Equal(d("500.00")) {
		t.Fatalf("grupos vacios noDist=%s want 500", r.NoDistribuido)
	}
}

func TestReparto_SinLiteralesDeGrupoEnSnapshotVacio(t *testing.T) {
	snap := snapBase()
	snap.GrupoPrivadosPct = decimal.Zero
	usos := []reparto.Uso{{
		ObraID: "a", Modalidad: reparto.Suscripcion, TipoObra: "unitario",
		Grupo:       reparto.GrupoPrivadosNacionales,
		DuracionMin: d("1"), Emisiones: 1, Rating: d("1"),
	}}
	_, err := reparto.Reparto(bolsa("100"), usos, snap, nil, optSinDed())
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("err=%v", err)
	}
}

func TestReparto_DeduccionAusente(t *testing.T) {
	snap := snapBase()
	snap.AdminPct = decimal.Zero
	usos := []reparto.Uso{{ObraID: "x", Modalidad: reparto.Cine, Espectadores: d("1")}}
	_, err := reparto.Reparto(bolsa("100"), usos, snap, nil, reparto.Opciones{})
	if !errors.Is(err, reparto.ErrParametroAusente) {
		t.Fatalf("err=%v", err)
	}
}
