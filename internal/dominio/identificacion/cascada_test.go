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

// umbralesPrueba: los mismos cortes que siembra el dataset sintetico.
func umbralesPrueba() Umbrales {
	return Umbrales{
		Match: decimal.RequireFromString("0.60"),
		Banda: decimal.RequireFromString("0.45"),
	}
}

func punt(s string) decimal.Decimal { return decimal.RequireFromString(s) }

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
			nombre:   "id poblado sin match y sin candidatos cae a ONI",
			entrada:  entradaValida(),
			consulta: Consulta{IDGlobalObraID: "", IDGlobalCual: IMDB},
			quiero: Resultado{
				Escalon:   EscalonONI,
				ONI:       true,
				Evidencia: "sin candidato sobre 0.45000",
			},
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
			nombre:   "entrada sin datos y sin candidatos es ONI, no un fallo",
			entrada:  Entrada{Fuente: "caracol", Titulo: "X"},
			consulta: Consulta{},
			quiero: Resultado{
				Escalon:   EscalonONI,
				ONI:       true,
				Evidencia: "sin candidato sobre 0.45000",
			},
		},

		// ------------------------------------------------------------------
		// Escalon 3: difuso (#32)
		// ------------------------------------------------------------------
		{
			nombre:  "candidato sobre el umbral resuelve en escalon 3",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.82")},
			}},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonDifuso,
				Puntaje:   punt("0.82"),
				Evidencia: `difuso "La Casa de las Dos Palmas" ~ obra-45 (0.82000)`,
			},
		},
		{
			nombre:  "gana el candidato de mayor puntaje",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: "obra-99", Puntaje: punt("0.64")},
				{ObraID: obraQuince, Puntaje: punt("0.91")},
			}},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonDifuso,
				Puntaje:   punt("0.91"),
				Evidencia: `difuso "La Casa de las Dos Palmas" ~ obra-45 (0.91000)`,
			},
		},
		{
			nombre:  "empate de puntaje se rompe por obra_id ascendente",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: "obra-99", Puntaje: punt("0.77")},
				{ObraID: "obra-12", Puntaje: punt("0.77")},
			}},
			quiero: Resultado{
				ObraID:    "obra-12",
				Escalon:   EscalonDifuso,
				Puntaje:   punt("0.77"),
				Evidencia: `difuso "La Casa de las Dos Palmas" ~ obra-12 (0.77000)`,
			},
		},
		{
			nombre:  "justo en el umbral resuelve: el corte es inclusivo",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.60")},
			}},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonDifuso,
				Puntaje:   punt("0.60"),
				Evidencia: `difuso "La Casa de las Dos Palmas" ~ obra-45 (0.60000)`,
			},
		},
		{
			nombre:  "la banda ambigua no asigna obra y adjunta los candidatos",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.52")},
				{ObraID: "obra-99", Puntaje: punt("0.47")},
			}},
			quiero: Resultado{
				Escalon:   EscalonONI,
				ONI:       true,
				Evidencia: `banda ambigua: 2 candidatos, mejor obra-45 (0.52000) para "La Casa de las Dos Palmas" bajo umbral 0.60000`,
				Candidatos: []Candidato{
					{ObraID: obraQuince, Puntaje: punt("0.52")},
					{ObraID: "obra-99", Puntaje: punt("0.47")},
				},
			},
		},
		{
			nombre:  "justo en el piso de banda va a la cola manual, no a ONI ciega",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.45")},
			}},
			quiero: Resultado{
				Escalon:   EscalonONI,
				ONI:       true,
				Evidencia: `banda ambigua: 1 candidatos, mejor obra-45 (0.45000) para "La Casa de las Dos Palmas" bajo umbral 0.60000`,
				Candidatos: []Candidato{
					{ObraID: obraQuince, Puntaje: punt("0.45")},
				},
			},
		},
		{
			nombre:  "por debajo del piso de banda es ONI sin candidatos adjuntos",
			entrada: entradaValida(),
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.31")},
			}},
			quiero: Resultado{
				Escalon:   EscalonONI,
				ONI:       true,
				Evidencia: "sin candidato sobre 0.45000",
			},
		},
		{
			nombre:  "la evidencia nombra el titulo original cuando el candidato salio de el",
			entrada: Entrada{Fuente: "caracol", Titulo: "Sin Tetas No Hay Paraiso", TituloOrig: "Without Breasts There Is No Paradise"},
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.88"), TituloConsultado: "Without Breasts There Is No Paradise"},
			}},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonDifuso,
				Puntaje:   punt("0.88"),
				Evidencia: `difuso "Without Breasts There Is No Paradise" ~ obra-45 (0.88000)`,
			},
		},
		{
			nombre:  "la banda tambien nombra el titulo original",
			entrada: Entrada{Fuente: "caracol", Titulo: "Sin Tetas No Hay Paraiso", TituloOrig: "Without Breasts There Is No Paradise"},
			consulta: Consulta{Candidatos: []Candidato{
				{ObraID: obraQuince, Puntaje: punt("0.50"), TituloConsultado: "Without Breasts There Is No Paradise"},
			}},
			quiero: Resultado{
				Escalon:   EscalonONI,
				ONI:       true,
				Evidencia: `banda ambigua: 1 candidatos, mejor obra-45 (0.50000) para "Without Breasts There Is No Paradise" bajo umbral 0.60000`,
				Candidatos: []Candidato{
					{ObraID: obraQuince, Puntaje: punt("0.50"), TituloConsultado: "Without Breasts There Is No Paradise"},
				},
			},
		},
		{
			nombre:  "el alias manda sobre un candidato difuso mejor puntuado",
			entrada: entradaValida(),
			consulta: Consulta{
				AliasObraID: obraQuince,
				Candidatos:  []Candidato{{ObraID: "obra-99", Puntaje: punt("0.99")}},
			},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonAlias,
				Puntaje:   decimal.NewFromInt(1),
				Evidencia: "alias caracol id_ficha=871732 -> obra-45",
			},
		},
		{
			nombre:  "el id global manda sobre un candidato difuso mejor puntuado",
			entrada: entradaValida(),
			consulta: Consulta{
				IDGlobalObraID: obraQuince,
				IDGlobalCual:   IMDB,
				Candidatos:     []Candidato{{ObraID: "obra-99", Puntaje: punt("0.99")}},
			},
			quiero: Resultado{
				ObraID:    obraQuince,
				Escalon:   EscalonIDGlobal,
				Puntaje:   decimal.NewFromInt(1),
				Evidencia: "imdb tt0100001 -> obra-45",
			},
		},
		{
			nombre: "fuera de repertorio corta antes del difuso",
			entrada: Entrada{
				Fuente: "noticias-24h",
				Titulo: "Noticiero de la noche",
			},
			consulta: Consulta{
				Candidatos: []Candidato{{ObraID: obraQuince, Puntaje: punt("0.99")}},
			},
			excluidas: FuentesExcluidas{"noticias-24h"},
			quiero: Resultado{
				Escalon:   EscalonExcluido,
				Evidencia: "fuera de repertorio: noticias-24h",
			},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tengo := Resolver(c.entrada, c.consulta, c.excluidas, umbralesPrueba())
			if !reflect.DeepEqual(tengo, c.quiero) {
				t.Fatalf("Resolver() = %+v, se esperaba %+v", tengo, c.quiero)
			}
		})
	}
}

// Los creditos no puntuan (R-02). Vigila la forma del tipo: el dia que alguien
// anada un campo de creditos a Entrada "para mejorar el matching", esto ladra.
func TestEntradaNoTieneCamposDeCreditos(t *testing.T) {
	prohibidos := []string{"autor", "guionista", "director", "actor", "libretista"}
	tipo := reflect.TypeOf(Entrada{})
	for i := range tipo.NumField() {
		campo := strings.ToLower(tipo.Field(i).Name)
		for _, p := range prohibidos {
			if strings.Contains(campo, p) {
				t.Fatalf("Entrada.%s es un campo de creditos: R-02 los deja fuera del matching",
					tipo.Field(i).Name)
			}
		}
	}
}

// La evidencia y el puntaje son deterministas: dos llamadas con la misma
// entrada y la misma consulta producen exactamente el mismo Resultado, campo
// a campo. Puntaje se compara con Equal, no con ==: dos decimal.Decimal que
// representan 1 pueden tener representacion interna distinta.
func TestResolverEsDeterminista(t *testing.T) {
	e := entradaValida()
	c := Consulta{AliasObraID: obraQuince}

	r1 := Resolver(e, c, nil, umbralesPrueba())
	r2 := Resolver(e, c, nil, umbralesPrueba())

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

// El orden en que llegan los candidatos no puede cambiar a quien se le paga
// (ADR 0005).
func TestResolverDifusoNoDependeDelOrdenDeCandidatos(t *testing.T) {
	e := entradaValida()
	asc := []Candidato{
		{ObraID: "obra-12", Puntaje: punt("0.77")},
		{ObraID: "obra-99", Puntaje: punt("0.77")},
	}
	desc := []Candidato{asc[1], asc[0]}

	r1 := Resolver(e, Consulta{Candidatos: asc}, nil, umbralesPrueba())
	r2 := Resolver(e, Consulta{Candidatos: desc}, nil, umbralesPrueba())

	if r1.ObraID != r2.ObraID {
		t.Fatalf("el orden de los candidatos cambio el match: %q vs %q", r1.ObraID, r2.ObraID)
	}
	if r1.Evidencia != r2.Evidencia {
		t.Fatalf("el orden de los candidatos cambio la evidencia: %q vs %q", r1.Evidencia, r2.Evidencia)
	}
}

// UnirCandidatos junta lo que devolvio cada titulo de la MISMA fila. Es la
// funcion que decide, asi que el orden tiene que ser total (ADR 0005).
func TestUnirCandidatos(t *testing.T) {
	cand := func(obra, puntaje, titulo string) Candidato {
		return Candidato{ObraID: obra, Puntaje: punt(puntaje), TituloConsultado: titulo}
	}

	casos := []struct {
		nombre string
		listas [][]Candidato
		quiero []Candidato
	}{
		{
			nombre: "por obra se queda con el mejor puntaje, venga de la lista que venga",
			listas: [][]Candidato{
				{cand("obra-1", "0.50", "emitido"), cand("obra-2", "0.80", "emitido")},
				{cand("obra-1", "0.90", "original"), cand("obra-2", "0.40", "original")},
			},
			quiero: []Candidato{cand("obra-1", "0.90", "original"), cand("obra-2", "0.80", "emitido")},
		},
		{
			nombre: "un empate entre listas lo gana la primera: el titulo emitido va antes",
			listas: [][]Candidato{
				{cand("obra-1", "0.70", "emitido")},
				{cand("obra-1", "0.70", "original")},
			},
			quiero: []Candidato{cand("obra-1", "0.70", "emitido")},
		},
		{
			nombre: "el orden es total: puntaje descendente y a igual puntaje obra_id ascendente",
			listas: [][]Candidato{
				{cand("obra-9", "0.60", "a"), cand("obra-3", "0.60", "a"), cand("obra-5", "0.75", "a")},
				{cand("obra-1", "0.60", "b")},
			},
			quiero: []Candidato{
				cand("obra-5", "0.75", "a"),
				cand("obra-1", "0.60", "b"),
				cand("obra-3", "0.60", "a"),
				cand("obra-9", "0.60", "a"),
			},
		},
		{
			nombre: "recorta a MaxCandidatos y conserva los mejores",
			listas: [][]Candidato{
				{
					cand("obra-1", "0.61", "a"), cand("obra-2", "0.62", "a"), cand("obra-3", "0.63", "a"),
					cand("obra-4", "0.64", "a"),
				},
				{cand("obra-5", "0.65", "b"), cand("obra-6", "0.66", "b"), cand("obra-7", "0.60", "b")},
			},
			quiero: []Candidato{
				cand("obra-6", "0.66", "b"), cand("obra-5", "0.65", "b"), cand("obra-4", "0.64", "a"),
				cand("obra-3", "0.63", "a"), cand("obra-2", "0.62", "a"),
			},
		},
		{
			nombre: "sin listas no hay candidatos",
			listas: nil,
			quiero: nil,
		},
		{
			nombre: "listas vacias tampoco",
			listas: [][]Candidato{{}, nil},
			quiero: nil,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tengo := UnirCandidatos(c.listas...)
			if len(tengo) != len(c.quiero) {
				t.Fatalf("UnirCandidatos() = %+v, se esperaba %+v", tengo, c.quiero)
			}
			for i := range c.quiero {
				if tengo[i].ObraID != c.quiero[i].ObraID ||
					!tengo[i].Puntaje.Equal(c.quiero[i].Puntaje) ||
					tengo[i].TituloConsultado != c.quiero[i].TituloConsultado {
					t.Fatalf("UnirCandidatos()[%d] = %+v, se esperaba %+v", i, tengo[i], c.quiero[i])
				}
			}
		})
	}
}

// Dos corridas no pueden pagar distinto por el orden en que llegaron las
// listas cuando NO hay empate entre ellas: el resultado es el mismo.
func TestUnirCandidatosNoDependeDelOrdenDeLasListasSinEmpates(t *testing.T) {
	a := []Candidato{{ObraID: "obra-1", Puntaje: punt("0.55")}, {ObraID: "obra-2", Puntaje: punt("0.80")}}
	b := []Candidato{{ObraID: "obra-1", Puntaje: punt("0.90")}, {ObraID: "obra-3", Puntaje: punt("0.30")}}

	ab := UnirCandidatos(a, b)
	ba := UnirCandidatos(b, a)

	if len(ab) != len(ba) {
		t.Fatalf("largos distintos: %+v vs %+v", ab, ba)
	}
	for i := range ab {
		if ab[i].ObraID != ba[i].ObraID || !ab[i].Puntaje.Equal(ba[i].Puntaje) {
			t.Fatalf("el orden de las listas cambio el resultado: %+v vs %+v", ab, ba)
		}
	}
}

// Umbrales.Validar: 0 < Banda < Match <= 1. Cada caso es una forma de dejar la
// cascada en un estado que no decide lo que dice decidir.
func TestUmbralesValidar(t *testing.T) {
	casos := []struct {
		nombre   string
		umbrales Umbrales
		quiero   string // fragmento del error; vacio = valido
	}{
		{"los de arranque", Umbrales{Match: punt("0.60"), Banda: punt("0.45")}, ""},
		{"banda pegada al umbral", Umbrales{Match: punt("0.60"), Banda: punt("0.599999")}, ""},
		{"umbral en el tope", Umbrales{Match: punt("1"), Banda: punt("0.45")}, ""},
		{"banda igual al umbral: la banda no tiene ancho", Umbrales{Match: punt("0.60"), Banda: punt("0.60")}, "estrictamente por debajo"},
		{"banda por encima del umbral", Umbrales{Match: punt("0.60"), Banda: punt("0.70")}, "estrictamente por debajo"},
		{"banda en cero es ausente, no un piso", Umbrales{Match: punt("0.60"), Banda: punt("0")}, "mayor que 0"},
		{"banda negativa", Umbrales{Match: punt("0.60"), Banda: punt("-0.1")}, "mayor que 0"},
		{"umbral por encima de 1", Umbrales{Match: punt("1.01"), Banda: punt("0.45")}, "no puede pasar de 1"},
		{"todo en cero", Umbrales{}, "mayor que 0"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := c.umbrales.Validar()
			if c.quiero == "" {
				if err != nil {
					t.Fatalf("Validar() = %v, se esperaba nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validar() = nil, se esperaba un error con %q", c.quiero)
			}
			if !strings.Contains(err.Error(), c.quiero) {
				t.Fatalf("Validar() = %q, se esperaba que dijera %q", err, c.quiero)
			}
		})
	}
}
