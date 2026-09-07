package identificacion

import (
	"reflect"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// entradaValida es la fila base que cada caso ajusta. Trae par local, IMDB
// poblado y ni IDA ni EIDR, para que cada prueba mueva una sola pieza.
func entradaValida() Entrada {
	return Entrada{
		Fuente:  "caracol",
		TipoID:  "id_ficha",
		ValorID: "871732",
		IDA:     "",
		EIDR:    "",
		IMDB:    "tt0100001",
		Titulo:  "La Casa de las Dos Palmas",
	}
}

const obraQuince = "obra-45"

func TestResolver(t *testing.T) {
	casos := []struct {
		nombre    string
		entrada   Entrada
		consulta  Consulta
		excluidas FuentesExcluidas
		quiero    Resultado
	}{
		{
			nombre:   "alias hit resuelve en escalon 1 con confianza maxima",
			entrada:  entradaValida(),
			consulta: Consulta{AliasObraID: obraQuince},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonAlias,
				Puntaje:   decimal.NewFromInt(1),
				Evidencia: "alias caracol id_ficha=871732 -> obra-45",
			},
		},
		{
			nombre:   "id global hit resuelve en escalon 2",
			entrada:  entradaValida(),
			consulta: Consulta{IDGlobalObraID: obraQuince, IDGlobalCual: IMDB},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonIDGlobal,
				Puntaje:   decimal.NewFromInt(1),
				Evidencia: "imdb tt0100001 -> obra-45",
			},
		},
		{
			nombre:   "id poblado sin match no resuelve",
			entrada:  entradaValida(),
			consulta: Consulta{IDGlobalObraID: "", IDGlobalCual: IMDB},
			quiero:   Resultado{},
		},
		{
			nombre:  "el alias manda sobre el id global",
			entrada: entradaValida(),
			consulta: Consulta{
				AliasObraID:    obraQuince,
				IDGlobalObraID: "obra-99",
				IDGlobalCual:   IDA,
			},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonAlias,
				Puntaje:   decimal.NewFromInt(1),
				Evidencia: "alias caracol id_ficha=871732 -> obra-45",
			},
		},
		{
			nombre: "el filtro de repertorio excluye aunque el alias pegara",
			entrada: Entrada{
				Fuente:  "noticias-24h",
				TipoID:  "id_ficha",
				ValorID: "1",
				Titulo:  "Noticiero de la noche",
			},
			consulta:  Consulta{AliasObraID: obraQuince},
			excluidas: FuentesExcluidas{"noticias-24h"},
			quiero: Resultado{
				Escalon:   EscalonExcluido,
				Evidencia: "fuera de repertorio: noticias-24h",
			},
		},
		{
			nombre: "una fuente que no esta en la lista no se excluye",
			entrada: Entrada{
				Fuente:  "caracol",
				TipoID:  "id_ficha",
				ValorID: "871732",
				Titulo:  "La Casa de las Dos Palmas",
			},
			consulta:  Consulta{AliasObraID: obraQuince},
			excluidas: FuentesExcluidas{"noticias-24h"},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonAlias,
				Puntaje:   decimal.NewFromInt(1),
				Evidencia: "alias caracol id_ficha=871732 -> obra-45",
			},
		},
		{
			nombre:   "entrada sin datos no resuelve",
			entrada:  Entrada{Fuente: "caracol", Titulo: "X"},
			consulta: Consulta{},
			quiero:   Resultado{},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tengo := Resolver(c.entrada, c.consulta, c.excluidas)
			if !reflect.DeepEqual(tengo, c.quiero) {
				t.Fatalf("Resolver() = %+v, se esperaba %+v", tengo, c.quiero)
			}
		})
	}
}

// La evidencia y el puntaje son deterministas: dos llamadas con la misma
// entrada y la misma consulta producen exactamente el mismo Resultado, campo
// a campo. Puntaje se compara con Equal, no con ==: dos decimal.Decimal que
// representan 1 pueden tener representacion interna distinta.
func TestResolverEsDeterminista(t *testing.T) {
	e := entradaValida()
	c := Consulta{AliasObraID: obraQuince}

	r1 := Resolver(e, c, nil)
	r2 := Resolver(e, c, nil)

	if r1.Evidencia != r2.Evidencia {
		t.Fatalf("evidencia no deterministica: %q vs %q", r1.Evidencia, r2.Evidencia)
	}
	if !r1.Puntaje.Equal(r2.Puntaje) {
		t.Fatalf("puntaje no deterministico: %s vs %s", r1.Puntaje, r2.Puntaje)
	}
	if r1.Escalon != r2.Escalon || r1.ObraID != r2.ObraID || r1.ONI != r2.ONI {
		t.Fatalf("resultado no deterministico: %+v vs %+v", r1, r2)
	}
	if !strings.Contains(r1.Evidencia, "obra-45") {
		t.Fatalf("la evidencia no nombra la obra: %q", r1.Evidencia)
	}
}
