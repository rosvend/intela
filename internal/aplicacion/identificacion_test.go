package aplicacion

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// ---------------------------------------------------------------------------
// Dobles

// ingestaFalsa devuelve la lista de usos que se le de y cuenta que periodos
// se pidieron. Los otros cuatro metodos de RepositorioIngesta no los llama
// nunca ResolverUsos: existen solo para satisfacer la interfaz.
type ingestaFalsa struct {
	usos         []UsoPersistido
	usosLlamadas []string
	err          error
}

func (i *ingestaFalsa) GuardarReporte(context.Context, string, string, string, string, string, int) error {
	return nil
}
func (i *ingestaFalsa) GuardarUsos(context.Context, []UsoPersistido) error { return nil }
func (i *ingestaFalsa) UsosSinResolver(context.Context) ([]UsoPersistido, error) {
	return nil, nil
}
func (i *ingestaFalsa) UsoPorID(context.Context, string) (UsoPersistido, error) {
	return UsoPersistido{}, nil
}

func (i *ingestaFalsa) UsosDePeriodo(_ context.Context, periodo string) ([]UsoPersistido, error) {
	i.usosLlamadas = append(i.usosLlamadas, periodo)
	return i.usos, i.err
}

type llamadaAlias struct {
	Fuente, Tipo, Valor, ObraID, Quien string
}

type llamadaMatch struct {
	UsoID string
	R     identificacion.Resultado
}

// identificacionFalsa simula RepositorioIdentificacion con mapas fijos y
// registra cada llamada, para poder afirmar CUANTAS veces se sondeo y en que
// ORDEN se escribio -es lo que hace comprobable D5 (alias antes que match) y
// D7 (nunca mas de un sondeo de alias, nunca mas de tres de id global).
//
// Los tres campos err* aislan el punto de fallo: un fallo de red en un solo
// sondeo tiene que abortar la corrida sin que los demas puntos finjan ir bien
// (D8), y eso exige poder fallar Alias sin fallar tambien ObraPorIDGlobal.
type identificacionFalsa struct {
	alias       map[string]string // "fuente|tipo|valor" -> obraID
	porIDGlobal map[string]string // "ida|eidr|imdb" -> obraID (con las otras dos vacias)

	errAlias        error
	errIDGlobal     error
	errGuardarAlias error
	errGuardarMatch error

	llamadasAlias    []string
	llamadasIDGlobal []string
	guardadosAlias   []llamadaAlias
	guardadosMatch   []llamadaMatch
	orden            []string // "alias" / "match:<usoID>", en el orden en que se llamaron
}

func (f *identificacionFalsa) Alias(_ context.Context, fuente, tipo, valor string) (string, error) {
	clave := fuente + "|" + tipo + "|" + valor
	f.llamadasAlias = append(f.llamadasAlias, clave)
	if f.errAlias != nil {
		return "", f.errAlias
	}
	if obraID, ok := f.alias[clave]; ok {
		return obraID, nil
	}
	return "", ErrNoEncontrado
}

func (f *identificacionFalsa) GuardarAlias(_ context.Context, fuente, tipo, valor, obraID, quien string) error {
	f.guardadosAlias = append(f.guardadosAlias, llamadaAlias{fuente, tipo, valor, obraID, quien})
	f.orden = append(f.orden, "alias")
	return f.errGuardarAlias
}

func (f *identificacionFalsa) ObraPorIDGlobal(_ context.Context, ida, eidr, imdb string) (string, error) {
	clave := ida + "|" + eidr + "|" + imdb
	f.llamadasIDGlobal = append(f.llamadasIDGlobal, clave)
	if f.errIDGlobal != nil {
		return "", f.errIDGlobal
	}
	if obraID, ok := f.porIDGlobal[clave]; ok {
		return obraID, nil
	}
	return "", ErrNoEncontrado
}

func (f *identificacionFalsa) GuardarMatch(_ context.Context, usoID string, r identificacion.Resultado) error {
	f.guardadosMatch = append(f.guardadosMatch, llamadaMatch{usoID, r})
	f.orden = append(f.orden, "match:"+usoID)
	return f.errGuardarMatch
}

// usoPendiente arma un UsoPersistido con los valores que pone la ingesta: TV,
// oni true, emisiones 1, escalon pendiente. Cada prueba ajusta lo que le
// importa (fuente, ids_fuente, escalon) por encima de esta base.
func usoPendiente(id, fuente, idsFuente string) UsoPersistido {
	return UsoPersistido{
		ID:        id,
		ReporteID: "rep-1",
		Fuente:    fuente,
		Titulo:    "Titulo de prueba",
		IDsFuente: idsFuente,
		Escalon:   "pendiente",
		ONI:       true,
		Modalidad: reparto.TV,
		Emisiones: 1,
	}
}

func correr(t *testing.T, ing *ingestaFalsa, idf *identificacionFalsa, excluidas identificacion.FuentesExcluidas, usos ...UsoPersistido) (int, error) {
	t.Helper()
	ing.usos = usos
	r := ResolverUsos{Usos: ing, Identificacion: idf, FueraDeRepertorio: excluidas}
	return r.ResolverUsos(t.Context(), "2024")
}

// ---------------------------------------------------------------------------
// U1-U11: el caso de uso

func TestResolverUsosResuelvePorAlias(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|871732": "obra-45"}}

	n, err := correr(t, ing, idf, nil, usoPendiente("u-1", "caracol", "id_ficha=871732"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	if len(idf.guardadosMatch) != 1 {
		t.Fatalf("se esperaba 1 match guardado, hubo %d", len(idf.guardadosMatch))
	}
	m := idf.guardadosMatch[0]
	if m.UsoID != "u-1" || m.R.Escalon != identificacion.EscalonAlias || m.R.ObraID != "obra-45" {
		t.Fatalf("match mal armado: %+v", m)
	}
	// El escalon 1 no aprende (D5): el alias ya es lo que resolvio la fila.
	if len(idf.guardadosAlias) != 0 {
		t.Fatal("el escalon 1 no deberia aprender ningun alias")
	}
}

func TestResolverUsosResuelvePorIDGlobalYAprendeAntesDelMatch(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{
		porIDGlobal: map[string]string{"||tt0100001": "obra-45"},
	}

	u := usoPendiente("u-1", "caracol", "id_ficha=871732\nimdb=tt0100001")
	n, err := correr(t, ing, idf, nil, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	if len(idf.guardadosAlias) != 1 {
		t.Fatalf("se esperaba 1 alias aprendido, hubo %d", len(idf.guardadosAlias))
	}
	a := idf.guardadosAlias[0]
	if a.Fuente != "caracol" || a.Tipo != "id_ficha" || a.Valor != "871732" ||
		a.ObraID != "obra-45" || a.Quien != quienCascada {
		t.Fatalf("alias mal aprendido: %+v", a)
	}
	if len(idf.guardadosMatch) != 1 || idf.guardadosMatch[0].R.Escalon != identificacion.EscalonIDGlobal {
		t.Fatalf("match mal armado: %+v", idf.guardadosMatch)
	}
	if !strings.Contains(idf.guardadosMatch[0].R.Evidencia, "imdb") {
		t.Fatalf("la evidencia no nombra el identificador que caso: %q", idf.guardadosMatch[0].R.Evidencia)
	}
	// D5: primero el alias, despues el match. Si el orden fuera al reves, una
	// corrida interrumpida entre los dos dejaria la fila resuelta sin que el
	// conocimiento se aprendiera.
	if !slices.Equal(idf.orden, []string{"alias", "match:u-1"}) {
		t.Fatalf("orden = %v, se esperaba [alias match:u-1]", idf.orden)
	}
}

func TestResolverUsosConAliasYaAprendidoNoSondeaIDGlobal(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{
		alias:       map[string]string{"caracol|id_ficha|871732": "obra-45"},
		porIDGlobal: map[string]string{"||tt0100001": "obra-45"},
	}

	u := usoPendiente("u-1", "caracol", "id_ficha=871732\nimdb=tt0100001")
	n, err := correr(t, ing, idf, nil, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	if idf.guardadosMatch[0].R.Escalon != identificacion.EscalonAlias {
		t.Fatalf("escalon = %q, se esperaba %q", idf.guardadosMatch[0].R.Escalon, identificacion.EscalonAlias)
	}
	if len(idf.llamadasIDGlobal) != 0 {
		t.Fatalf("se sondeo id global %d veces, se esperaban 0: el alias cortocircuita", len(idf.llamadasIDGlobal))
	}
}

func TestResolverUsosConFilaSinDatosNoResuelve(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}

	n, err := correr(t, ing, idf, nil, usoPendiente("u-1", "caracol", ""))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	if len(idf.llamadasAlias) != 0 || len(idf.llamadasIDGlobal) != 0 {
		t.Fatal("una fila sin par ni globales no puede sondear nada")
	}
	if len(idf.guardadosAlias) != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatal("una fila sin datos no puede escribir nada: queda pendiente para el difuso")
	}
}

func TestResolverUsosExcluyeSinSondearNiEscribir(t *testing.T) {
	ing := &ingestaFalsa{}
	// El alias SI pegaria si se sondeara: la prueba es que ni se intenta.
	idf := &identificacionFalsa{alias: map[string]string{"canal-deportes|id_ficha|1": "obra-45"}}

	u := usoPendiente("u-1", "canal-deportes", "id_ficha=1")
	n, err := correr(t, ing, idf, identificacion.FuentesExcluidas{"canal-deportes"}, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	if len(idf.llamadasAlias) != 0 || len(idf.llamadasIDGlobal) != 0 {
		t.Fatal("una fuente fuera de repertorio no puede sondear nada (R-27)")
	}
	if len(idf.guardadosAlias) != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatal("una fuente fuera de repertorio no puede escribir nada")
	}
}

func TestResolverUsosIgnoraLasQueNoEstanPendientes(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|1": "obra-1"}}

	yaAlias := usoPendiente("u-1", "caracol", "id_ficha=1")
	yaAlias.Escalon = "alias"
	yaOni := usoPendiente("u-2", "caracol", "id_ficha=1")
	yaOni.Escalon = "oni"
	yaManual := usoPendiente("u-3", "caracol", "id_ficha=1")
	yaManual.Escalon = "manual"
	pendiente := usoPendiente("u-4", "caracol", "id_ficha=1")

	n, err := correr(t, ing, idf, nil, yaAlias, yaOni, yaManual, pendiente)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1 (solo la pendiente)", n)
	}
	if len(idf.guardadosMatch) != 1 || idf.guardadosMatch[0].UsoID != "u-4" {
		t.Fatalf("se toco una fila que no estaba pendiente: %+v", idf.guardadosMatch)
	}
}

func TestResolverUsosAbortaSiFallaElSondeoDeAlias(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{errAlias: errors.New("red caida")}

	n, err := correr(t, ing, idf, nil, usoPendiente("u-1", "caracol", "id_ficha=1"))
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	if len(idf.guardadosAlias) != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatal("una corrida abortada no puede haber escrito nada")
	}
}

func TestResolverUsosAbortaSiFallaElSondeoDeIDGlobal(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{errIDGlobal: errors.New("red caida")}

	u := usoPendiente("u-1", "caracol", "id_ficha=1\nimdb=tt0100001")
	n, err := correr(t, ing, idf, nil, u)
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	if len(idf.guardadosAlias) != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatal("una corrida abortada no puede haber escrito nada")
	}
}

func TestResolverUsosSinAliasYSinGlobalesQueCasenNoResuelve(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{} // todos los mapas vacios: todo sondeo es "no encontrado"

	u := usoPendiente("u-1", "caracol", "id_ficha=1\nimdb=tt9999999")
	n, err := correr(t, ing, idf, nil, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	if len(idf.guardadosAlias) != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatal("una fila no resuelta no puede escribir nada: queda pendiente para el difuso")
	}
}

func TestResolverUsosPropagaElErrorDeGuardarMatch(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{
		alias:           map[string]string{"caracol|id_ficha|1": "obra-1"},
		errGuardarMatch: errors.New("fallo al guardar"),
	}

	n, err := correr(t, ing, idf, nil, usoPendiente("u-1", "caracol", "id_ficha=1"))
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0: la fila que fallo no cuenta", n)
	}
}

func TestResolverUsosPideElPeriodoCorrecto(t *testing.T) {
	ing := &ingestaFalsa{usos: []UsoPersistido{usoPendiente("u-1", "caracol", "id_ficha=1")}}
	idf := &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|1": "obra-1"}}

	n, err := (ResolverUsos{Usos: ing, Identificacion: idf}).ResolverUsos(t.Context(), "2024-06")
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	if !slices.Equal(ing.usosLlamadas, []string{"2024-06"}) {
		t.Fatalf("periodos pedidos = %v, se esperaba [2024-06]", ing.usosLlamadas)
	}
}

// ---------------------------------------------------------------------------
// U12: entradaDesdeUso / el parser de ids_fuente (D1) y el par local
// canonico por precedencia de fuente (D2)

func TestEntradaDesdeUso(t *testing.T) {
	casos := []struct {
		nombre    string
		fuente    string
		idsFuente string
		quiero    identificacion.Entrada
	}{
		{
			nombre:    "caracol con par local y un global",
			fuente:    "caracol",
			idsFuente: "id_ficha=871732\nimdb=tt0100001",
			quiero:    identificacion.Entrada{Fuente: "caracol", TipoID: "id_ficha", ValorID: "871732", IMDB: "tt0100001"},
		},
		{
			nombre:    "netflix prefiere show_id sobre series_id y netflix_id",
			fuente:    "netflix",
			idsFuente: "show_id=80141259\nseries_id=123\nnetflix_id=456",
			quiero:    identificacion.Entrada{Fuente: "netflix", TipoID: "show_id", ValorID: "80141259"},
		},
		{
			nombre:    "fuente con mapeo pero sin su clave preferida no produce par",
			fuente:    "caracol",
			idsFuente: "foo=bar",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "linea sin igual se ignora",
			fuente:    "caracol",
			idsFuente: "solo-texto",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "clave sin valor se ignora",
			fuente:    "caracol",
			idsFuente: "k=",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "valor sin clave se ignora",
			fuente:    "caracol",
			idsFuente: "=v",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "ids_fuente vacio no produce par ni globales",
			fuente:    "caracol",
			idsFuente: "",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "fuente sin mapeo con una sola clave local la usa",
			fuente:    "otra",
			idsFuente: "unica=1",
			quiero:    identificacion.Entrada{Fuente: "otra", TipoID: "unica", ValorID: "1"},
		},
		{
			nombre:    "clave local repetida: gana la ultima",
			fuente:    "otra",
			idsFuente: "a=1\na=2",
			quiero:    identificacion.Entrada{Fuente: "otra", TipoID: "a", ValorID: "2"},
		},
		{
			nombre:    "fuente sin mapeo con varias claves: la alfabetica",
			fuente:    "otra",
			idsFuente: "y=2\nx=1",
			quiero:    identificacion.Entrada{Fuente: "otra", TipoID: "x", ValorID: "1"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			u := usoPendiente("u-1", c.fuente, c.idsFuente)
			c.quiero.Titulo = u.Titulo // entradaDesdeUso copia el titulo tal cual

			tengo := entradaDesdeUso(u)
			if tengo != c.quiero {
				t.Fatalf("entradaDesdeUso() = %+v, se esperaba %+v", tengo, c.quiero)
			}
		})
	}
}
