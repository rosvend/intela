package reparto_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

type factorEsperado struct {
	nombre string
	valor  string
	origen string
}

func TestDesglosarUsoPorModalidad(t *testing.T) {
	tv := func(tipo, dur, rating string, em int64) reparto.Uso {
		return reparto.Uso{ObraID: "o", Modalidad: reparto.TV, TipoObra: tipo, CanalID: "z",
			DuracionMin: d(dur), Rating: d(rating), Emisiones: em}
	}
	factoresTV := func(pond, dur, rating, em string) [][]factorEsperado {
		return [][]factorEsperado{{
			{reparto.FactorPonderacion, pond, reparto.OrigenParametro},
			{reparto.FactorDuracion, dur, reparto.OrigenUso},
			{reparto.FactorRating, rating, reparto.OrigenUso},
			{reparto.FactorEmisiones, em, reparto.OrigenUso},
		}}
	}
	suscripcion := tv("serie", "48", "9", 10)
	suscripcion.Modalidad = reparto.Suscripcion
	suscripcion.Grupo = reparto.GrupoPremium
	hotel := tv("serie", "48", "9", 10)
	hotel.Modalidad = reparto.Hotel
	teatroSnap := snapBase()
	teatroSnap.BaseCineTeatro = reparto.BaseTaquilla

	casos := []struct {
		nombre   string
		uso      reparto.Uso
		snap     reparto.Snapshot
		formula  string
		terminos [][]factorEsperado
		puntos   string
	}{
		{"tv cinematografica", tv("cinematografica", "70", "4.5", 1), snapBase(), "RD 9.1.1",
			factoresTV("5.0", "70", "4.5", "1"), "1575"},
		{"tv serie", tv("serie", "48", "9", 10), snapBase(), "RD 9.1.1",
			factoresTV("1.3", "48", "9", "10"), "5616"},
		{"suscripcion", suscripcion, snapBase(), "RD 9.5",
			factoresTV("1.3", "48", "9", "10"), "5616"},
		{"hotel", hotel, snapBase(), "RD 9.6",
			factoresTV("1.3", "48", "9", "10"), "5616"},
		{"cine por espectadores", reparto.Uso{ObraID: "o", Modalidad: reparto.Cine, Espectadores: d("250")},
			snapBase(), "RD 9.2",
			[][]factorEsperado{{{reparto.FactorEspectadores, "250", reparto.OrigenUso}}}, "250"},
		{"teatro por taquilla", reparto.Uso{ObraID: "o", Modalidad: reparto.Teatro, Taquilla: d("1000")},
			teatroSnap, "RD 9.3",
			[][]factorEsperado{{{reparto.FactorTaquilla, "1000", reparto.OrigenUso}}}, "1000"},
		{"transporte", reparto.Uso{ObraID: "o", Modalidad: reparto.Transporte, Exhibiciones: 7},
			snapBase(), "RD 9.4",
			[][]factorEsperado{{{reparto.FactorExhibiciones, "7", reparto.OrigenUso}}}, "7"},
		{"ott", reparto.Uso{ObraID: "o", Modalidad: reparto.OTT, PB: d("1.3"), MinutosVistos: d("40000"), Vistas: d("1000")},
			snapBase(), "RD 9.7",
			[][]factorEsperado{
				{{reparto.FactorPB, "1.3", reparto.OrigenUso}, {reparto.FactorWa, "0.5", reparto.OrigenParametro}},
				{{reparto.FactorMinutosVistos, "40000", reparto.OrigenUso}, {reparto.FactorWb, "0.3", reparto.OrigenParametro}},
				{{reparto.FactorVistas, "1000", reparto.OrigenUso}, {reparto.FactorWc, "0.2", reparto.OrigenParametro}},
			}, "12200.65"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			dg, err := reparto.DesglosarUso(c.uso, c.snap)
			if err != nil {
				t.Fatal(err)
			}
			if dg.Formula != c.formula {
				t.Errorf("formula = %q, quiero %q", dg.Formula, c.formula)
			}
			if !dg.Puntos.Equal(d(c.puntos)) {
				t.Errorf("puntos = %s, quiero %s", dg.Puntos, c.puntos)
			}
			if len(dg.Terminos) != len(c.terminos) {
				t.Fatalf("terminos = %d, quiero %d", len(dg.Terminos), len(c.terminos))
			}
			suma := decimal.Zero
			for i, tm := range dg.Terminos {
				want := c.terminos[i]
				if len(tm.Factores) != len(want) {
					t.Fatalf("termino %d: factores = %d, quiero %d", i, len(tm.Factores), len(want))
				}
				prod := decimal.NewFromInt(1)
				for j, f := range tm.Factores {
					if f.Nombre != want[j].nombre || f.Origen != want[j].origen || !f.Valor.Equal(d(want[j].valor)) {
						t.Errorf("termino %d factor %d = %+v, quiero %+v", i, j, f, want[j])
					}
					prod = prod.Mul(f.Valor)
				}
				if !tm.Producto.Equal(prod) {
					t.Errorf("termino %d: producto %s != multiplicacion de factores %s", i, tm.Producto, prod)
				}
				suma = suma.Add(tm.Producto)
			}
			if !dg.Puntos.Equal(suma) {
				t.Errorf("puntos %s != suma de productos %s", dg.Puntos, suma)
			}
		})
	}
}

func TestDesglosarUsoRechazaComoElMotor(t *testing.T) {
	cineSinBase := snapBase()
	cineSinBase.BaseCineTeatro = ""
	ottSinWa := snapBase()
	ottSinWa.Wa = decimal.Zero

	casos := []struct {
		nombre string
		uso    reparto.Uso
		snap   reparto.Snapshot
		want   error
	}{
		{"tv sin tipo", reparto.Uso{ObraID: "o", Modalidad: reparto.TV, TipoObra: ""}, snapBase(), reparto.ErrRepartoInvalido},
		{"tv tipo desconocido", reparto.Uso{ObraID: "o", Modalidad: reparto.TV, TipoObra: "documental"}, snapBase(), reparto.ErrRepartoInvalido},
		{"tv rating negativo", reparto.Uso{ObraID: "o", Modalidad: reparto.TV, TipoObra: "serie", Rating: d("-1")}, snapBase(), reparto.ErrRepartoInvalido},
		{"cine sin base", reparto.Uso{ObraID: "o", Modalidad: reparto.Cine}, cineSinBase, reparto.ErrParametroAusente},
		{"ott sin wa", reparto.Uso{ObraID: "o", Modalidad: reparto.OTT}, ottSinWa, reparto.ErrParametroAusente},
		{"modalidad desconocida", reparto.Uso{ObraID: "o", Modalidad: "radio"}, snapBase(), reparto.ErrModalidadDesconocida},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := reparto.DesglosarUso(c.uso, c.snap)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, quiero %v", err, c.want)
			}
		})
	}
}

// TestDesgloseReproduceLosPuntosDelMotor prueba que el recibo y el motor no
// divergen: la suma de los desgloses de una obra, redondeada como el motor,
// es el Puntos que el motor persiste (#187).
func TestDesgloseReproduceLosPuntosDelMotor(t *testing.T) {
	susc := func(obra, tipo string, g reparto.GrupoCanal, dur string, em int64) reparto.Uso {
		return reparto.Uso{ObraID: obra, Modalidad: reparto.Suscripcion, TipoObra: tipo, CanalID: "c-" + string(g),
			Grupo: g, DuracionMin: d(dur), Rating: d("2"), Emisiones: em}
	}
	casos := []struct {
		nombre string
		usos   []reparto.Uso
	}{
		{"golden canal Z", []reparto.Uso{
			{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica", CanalID: "z",
				DuracionMin: d("70"), Emisiones: 1, Rating: d("4.5")},
			{ObraID: "y", Modalidad: reparto.TV, TipoObra: "serie", CanalID: "z",
				DuracionMin: d("48"), Emisiones: 10, Rating: d("9")},
		}},
		{"ott con dos obras", []reparto.Uso{
			{ObraID: "x", Modalidad: reparto.OTT, PB: d("1.3"), MinutosVistos: d("40000"), Vistas: d("1000")},
			{ObraID: "x", Modalidad: reparto.OTT, PB: d("0.7"), MinutosVistos: d("1234.5"), Vistas: d("17")},
			{ObraID: "y", Modalidad: reparto.OTT, PB: d("2"), MinutosVistos: d("500"), Vistas: d("3")},
		}},
		{"suscripcion en dos grupos", []reparto.Uso{
			susc("x", "serie", reparto.GrupoPrivadosNacionales, "48", 10),
			susc("x", "serie", reparto.GrupoPrivadosNacionales, "30", 4),
			susc("y", "unitario", reparto.GrupoPremium, "60", 3),
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			snap := snapBase()
			var decls []repertorio.Declaracion
			vistas := map[string]bool{}
			for _, u := range c.usos {
				if !vistas[u.ObraID] {
					vistas[u.ObraID] = true
					decls = append(decls, declCompleta(u.ObraID, "t-"+u.ObraID, "IPI-"+u.ObraID))
				}
			}
			r, err := reparto.Reparto(bolsa("1000000"), c.usos, snap, decls, optSinDed())
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Obras) == 0 {
				t.Fatal("el motor no devolvio obras")
			}
			for _, o := range r.Obras {
				suma := decimal.Zero
				for _, u := range c.usos {
					if u.ObraID != o.ObraID {
						continue
					}
					dg, err := reparto.DesglosarUso(u, snap)
					if err != nil {
						t.Fatal(err)
					}
					suma = suma.Add(dg.Puntos)
				}
				if !suma.Round(8).Equal(o.Puntos) {
					t.Errorf("obra %s: suma de desgloses %s != puntos del motor %s", o.ObraID, suma.Round(8), o.Puntos)
				}
			}
		})
	}
}
