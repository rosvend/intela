package aplicacion

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// ---------------------------------------------------------------------------
// Dobles

// ingestaFalsa devuelve la lista de usos que se le de y cuenta que periodos
// se pidieron. Los otros metodos de RepositorioIngesta no los llama nunca
// ResolverUsos: existen solo para satisfacer la interfaz.
type ingestaFalsa struct {
	usos         []UsoPersistido
	usosLlamadas []string
	err          error
}

func (i *ingestaFalsa) GuardarReporte(context.Context, string, string, string, string, string, int) error {
	return nil
}
func (i *ingestaFalsa) GuardarUsos(context.Context, []UsoPersistido) error { return nil }
func (i *ingestaFalsa) GuardarEntrega(context.Context, Reporte, []UsoPersistido) error {
	return nil
}
func (i *ingestaFalsa) UsosSinResolver(context.Context) ([]UsoPersistido, error) {
	return nil, nil
}
func (i *ingestaFalsa) UsoPorID(context.Context, string) (UsoPersistido, error) {
	return UsoPersistido{}, nil
}
func (i *ingestaFalsa) ListarCargas(context.Context, string) ([]CargaReporte, error) {
	return nil, nil
}

func (i *ingestaFalsa) UsosDePeriodo(_ context.Context, periodo string) ([]UsoPersistido, error) {
	i.usosLlamadas = append(i.usosLlamadas, periodo)
	return i.usos, i.err
}
func (i *ingestaFalsa) ListarRechazos(context.Context) ([]UsoPersistido, error) {
	return nil, nil
}
func (i *ingestaFalsa) RechazosDeReporte(context.Context, string, Paginacion) ([]UsoPersistido, error) {
	return nil, nil
}

type llamadaAlias struct {
	Fuente, Tipo, Valor, ObraID, Quien string
}

type llamadaMatch struct {
	UsoID         string
	EscalonPrevio string
	R             identificacion.Resultado
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

	errAlias             error
	errIDGlobal          error
	errGuardarAlias      error
	errGuardarMatch      error
	errGuardarCandidatos error
	// cambiadas simula filas que otro proceso cambio entre la lectura y la
	// escritura: GuardarMatch responde ErrNoEncontrado, como el UPDATE
	// condicional del adaptador real.
	cambiadas map[string]bool

	llamadasAlias       []string
	llamadasIDGlobal    []string
	guardadosAlias      []llamadaAlias
	guardadosMatch      []llamadaMatch
	guardadosCandidatos []llamadaCandidatos
	orden               []string // "alias" / "candidatos:<usoID>" / "match:<usoID>", en orden
}

type llamadaCandidatos struct {
	UsoID      string
	Candidatos []identificacion.Candidato
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

func (f *identificacionFalsa) GuardarMatch(_ context.Context, usoID, escalonPrevio string, r identificacion.Resultado) error {
	if f.cambiadas[usoID] {
		return ErrNoEncontrado
	}
	f.guardadosMatch = append(f.guardadosMatch, llamadaMatch{usoID, escalonPrevio, r})
	f.orden = append(f.orden, "match:"+usoID)
	return f.errGuardarMatch
}

func (f *identificacionFalsa) GuardarCandidatos(_ context.Context, usoID string, cs []identificacion.Candidato) error {
	f.guardadosCandidatos = append(f.guardadosCandidatos, llamadaCandidatos{usoID, cs})
	f.orden = append(f.orden, "candidatos:"+usoID)
	return f.errGuardarCandidatos
}

// similitudFalsa cuenta consultas: "no se consulta si ya hay match" hay que
// poder probarlo.
type similitudFalsa struct {
	porTitulo map[string][]identificacion.Candidato
	err       error
	// errPorTitulo hace fallar solo la consulta de un titulo: la fila con dos
	// titulos tiene que abortar aunque el primero haya ido bien (D8).
	errPorTitulo map[string]error
	llamadas     []string
	pisos        []decimal.Decimal
}

func (s *similitudFalsa) Candidatos(_ context.Context, titulo string, piso decimal.Decimal) ([]identificacion.Candidato, error) {
	s.llamadas = append(s.llamadas, titulo)
	s.pisos = append(s.pisos, piso)
	if s.err != nil {
		return nil, s.err
	}
	if err := s.errPorTitulo[titulo]; err != nil {
		return nil, err
	}
	return s.porTitulo[titulo], nil
}

// parametroEnFechaFalso registra con que fecha se pregunto cada umbral.
type parametroEnFechaFalso struct {
	valores  map[string]string
	err      error
	llamadas []llamadaParametro
}

type llamadaParametro struct {
	Clave string
	Fecha time.Time
}

func (p *parametroEnFechaFalso) ParametroVigente(_ context.Context, clave string, fecha time.Time) (decimal.Decimal, error) {
	p.llamadas = append(p.llamadas, llamadaParametro{clave, fecha})
	if p.err != nil {
		return decimal.Zero, p.err
	}
	v, ok := p.valores[clave]
	if !ok {
		return decimal.Zero, errors.New("parametro sin vigencia: " + clave)
	}
	return decimal.RequireFromString(v), nil
}

func umbralesPorDefecto() *parametroEnFechaFalso {
	return &parametroEnFechaFalso{valores: map[string]string{
		ClaveUmbralMatch: "0.60",
		ClaveUmbralBanda: "0.45",
	}}
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

// correr: cascada con un motor que no propone nada. Caso base de los
// escalones 1-2, donde lo que no resuelva sale a ONI.
func correr(t *testing.T, ing *ingestaFalsa, idf *identificacionFalsa, excluidas identificacion.FuentesExcluidas, usos ...UsoPersistido) (int, error) {
	t.Helper()
	return correrCon(t, ing, idf, &similitudFalsa{}, umbralesPorDefecto(), excluidas, usos...)
}

func correrCon(
	t *testing.T,
	ing *ingestaFalsa,
	idf *identificacionFalsa,
	sim *similitudFalsa,
	par *parametroEnFechaFalso,
	excluidas identificacion.FuentesExcluidas,
	usos ...UsoPersistido,
) (int, error) {
	t.Helper()
	ing.usos = usos
	r := ResolverUsos{
		Usos:              ing,
		Identificacion:    idf,
		Similitud:         sim,
		Parametros:        par,
		FueraDeRepertorio: excluidas,
	}
	return r.ResolverUsos(t.Context(), "2024")
}

// soloONI: la unica escritura fue marcar la fila ONI sin obra.
func soloONI(t *testing.T, idf *identificacionFalsa, usoID string) {
	t.Helper()
	if len(idf.guardadosAlias) != 0 {
		t.Fatalf("una fila no resuelta no aprende alias: %+v", idf.guardadosAlias)
	}
	if len(idf.guardadosCandidatos) != 0 {
		t.Fatalf("sin candidatos no se escribe la bandeja: %+v", idf.guardadosCandidatos)
	}
	if len(idf.guardadosMatch) != 1 {
		t.Fatalf("se esperaba una escritura de ONI, hubo %d: %+v", len(idf.guardadosMatch), idf.guardadosMatch)
	}
	m := idf.guardadosMatch[0]
	if m.UsoID != usoID || m.R.Escalon != identificacion.EscalonONI || m.R.ObraID != "" || !m.R.ONI {
		t.Fatalf("se esperaba ONI sin obra para %q: %+v", usoID, m)
	}
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

// Regresion: una fila que solo trae un identificador global (sin par local)
// resuelve igual por escalon 2 y no intenta aprender un alias que no tiene id
// de fuente al que asociar. Antes de este fix, GuardarAlias se llamaba con
// valor="" -el adaptador lo rechaza por el CHECK de alias_obra- y ese error
// abortaba la corrida entera (D8), dejando sin procesar tambien las filas
// siguientes del lote.
func TestResolverUsosPorIDGlobalSinParLocalNoAprendeAlias(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{
		porIDGlobal: map[string]string{"||tt0100001": "obra-45"},
	}

	u := usoPendiente("u-1", "caracol", "imdb=tt0100001")
	n, err := correr(t, ing, idf, nil, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	if len(idf.guardadosAlias) != 0 {
		t.Fatalf("no hay par local que aprender, pero se guardaron %d alias: %+v",
			len(idf.guardadosAlias), idf.guardadosAlias)
	}
	if len(idf.guardadosMatch) != 1 || idf.guardadosMatch[0].R.Escalon != identificacion.EscalonIDGlobal {
		t.Fatalf("match mal armado: %+v", idf.guardadosMatch)
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
	soloONI(t, idf, "u-1")
}

func TestResolverUsosExcluyeSinSondearYGuardaLaExclusion(t *testing.T) {
	ing := &ingestaFalsa{}
	// El alias SI pegaria si se sondeara: la prueba es que ni se intenta.
	idf := &identificacionFalsa{alias: map[string]string{"canal-deportes|id_ficha|1": "obra-45"}}

	u := usoPendiente("u-1", "canal-deportes", "id_ficha=1")
	n, err := correr(t, ing, idf, identificacion.FuentesExcluidas{"canal-deportes"}, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0: excluida no es resuelta", n)
	}
	if len(idf.llamadasAlias) != 0 || len(idf.llamadasIDGlobal) != 0 {
		t.Fatal("una fuente fuera de repertorio no puede sondear nada (R-27)")
	}
	if len(idf.guardadosAlias) != 0 {
		t.Fatal("una fuente fuera de repertorio no puede aprender alias")
	}
	// Criterio 4: la exclusion se persiste para que la fila salga de ONI.
	if len(idf.guardadosMatch) != 1 || idf.guardadosMatch[0].UsoID != "u-1" ||
		idf.guardadosMatch[0].R.Escalon != identificacion.EscalonExcluido || idf.guardadosMatch[0].R.ObraID != "" {
		t.Fatalf("se esperaba un GuardarMatch con escalon excluido y sin obra: %+v", idf.guardadosMatch)
	}
}

func TestResolverUsosPropagaElErrorDeGuardarLaExclusion(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{errGuardarMatch: errors.New("fallo al guardar")}

	u := usoPendiente("u-1", "canal-deportes", "id_ficha=1")
	if _, err := correr(t, ing, idf, identificacion.FuentesExcluidas{"canal-deportes"}, u); err == nil {
		t.Fatal("se esperaba un error: una exclusion que no se guarda no puede pasar en silencio (D8)")
	}
}

// D4: una fila excluida en una corrida anterior se reevalua. Si su fuente
// sigue excluida no se sondea ni se reescribe: la corrida es idempotente.
func TestResolverUsosExcluidaQueSigueExcluidaNoSeReescribe(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{alias: map[string]string{"canal-deportes|id_ficha|1": "obra-45"}}

	u := usoPendiente("u-1", "canal-deportes", "id_ficha=1")
	u.Escalon, u.ONI = identificacion.EscalonExcluido, false
	n, err := correr(t, ing, idf, identificacion.FuentesExcluidas{"canal-deportes"}, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 || len(idf.llamadasAlias) != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatalf("una excluida que sigue excluida no se toca: n=%d sondeos=%v matches=%+v",
			n, idf.llamadasAlias, idf.guardadosMatch)
	}
}

// D4: si la lista de exclusion estaba mal y se corrige, la siguiente corrida
// devuelve la fila a la cascada y la resuelve. La escritura es condicional al
// escalon 'excluido' con que se leyo.
func TestResolverUsosExcluidaCuyaFuenteYaNoLoEstaSeResuelve(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|1": "obra-1"}}

	u := usoPendiente("u-1", "caracol", "id_ficha=1")
	u.Escalon, u.ONI = identificacion.EscalonExcluido, false
	n, err := correr(t, ing, idf, nil, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	if len(idf.guardadosMatch) != 1 {
		t.Fatalf("se esperaba un match: %+v", idf.guardadosMatch)
	}
	m := idf.guardadosMatch[0]
	if m.EscalonPrevio != identificacion.EscalonExcluido || m.R.Escalon != identificacion.EscalonAlias || m.R.ObraID != "obra-1" {
		t.Fatalf("match mal armado: %+v", m)
	}
}

// D4: una excluida cuya fuente ya no lo esta pero que la cascada no resuelve
// vuelve a pendiente, para que la vean el difuso (#32) y ONI.
func TestResolverUsosExcluidaQueYaNoLoEstaYNoResuelveVuelveAPendiente(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}

	u := usoPendiente("u-1", "caracol", "id_ficha=1")
	u.Escalon, u.ONI = identificacion.EscalonExcluido, false
	n, err := correr(t, ing, idf, nil, u)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0: volver a pendiente no es resolver", n)
	}
	if len(idf.guardadosMatch) != 1 {
		t.Fatalf("se esperaba una escritura: %+v", idf.guardadosMatch)
	}
	m := idf.guardadosMatch[0]
	// Antes del escalon 3 esto volvia a 'pendiente', porque el difuso todavia
	// no existia y habia que dejarsela. Ahora la cascada esta completa: si
	// vuelve al repertorio y aun asi no la reconoce nadie, el estado honesto es
	// ONI (RD 13.8), no "todavia no se ha mirado".
	if m.EscalonPrevio != identificacion.EscalonExcluido || m.R.Escalon != identificacion.EscalonONI || m.R.ObraID != "" {
		t.Fatalf("se esperaba ONI sin obra: %+v", m)
	}
}

// Hallazgo 3: si otro proceso cambio la fila entre la lectura y la escritura,
// GuardarMatch no escribe (ErrNoEncontrado) y la corrida sigue con las demas
// sin error y sin contarla como resuelta.
func TestResolverUsosSaltaUnaFilaQueCambioDuranteLaCorrida(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{
		alias:     map[string]string{"caracol|id_ficha|1": "obra-1"},
		cambiadas: map[string]bool{"u-1": true},
	}

	n, err := correr(t, ing, idf, nil,
		usoPendiente("u-1", "caracol", "id_ficha=1"),
		usoPendiente("u-2", "caracol", "id_ficha=1"))
	if err != nil {
		t.Fatalf("una fila cambiada por otro no es un error de la corrida: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1 (solo u-2)", n)
	}
	if len(idf.guardadosMatch) != 1 || idf.guardadosMatch[0].UsoID != "u-2" ||
		idf.guardadosMatch[0].EscalonPrevio != identificacion.EscalonPendiente {
		t.Fatalf("se esperaba escribir solo u-2, condicionado a pendiente: %+v", idf.guardadosMatch)
	}
}

// Una automatica no pisa una humana: la fila 'manual' es la que importa.
func TestResolverUsosNoRehaceLoYaDecidido(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|1": "obra-1"}}

	yaAlias := usoPendiente("u-1", "caracol", "id_ficha=1")
	yaAlias.Escalon = identificacion.EscalonAlias
	yaDifuso := usoPendiente("u-2", "caracol", "id_ficha=1")
	yaDifuso.Escalon = identificacion.EscalonDifuso
	yaManual := usoPendiente("u-3", "caracol", "id_ficha=1")
	yaManual.Escalon = "manual"
	pendiente := usoPendiente("u-4", "caracol", "id_ficha=1")

	n, err := correr(t, ing, idf, nil, yaAlias, yaDifuso, yaManual, pendiente)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1 (solo la pendiente)", n)
	}
	if len(idf.guardadosMatch) != 1 || idf.guardadosMatch[0].UsoID != "u-4" {
		t.Fatalf("se toco una fila ya decidida: %+v", idf.guardadosMatch)
	}
}

// Las ONI SI se reintentan: el catalogo crece y una obra de alta hoy
// identifica usos que el mes pasado no se parecian a nada (D6).
func TestResolverUsosReintentaLasONICuandoCreceElCatalogo(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|1": "obra-1"}}

	yaOni := usoPendiente("u-1", "caracol", "id_ficha=1")
	yaOni.Escalon = identificacion.EscalonONI

	n, err := correr(t, ing, idf, nil, yaOni)
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1: la ONI se reintenta", n)
	}
	m := idf.guardadosMatch[0]
	if m.EscalonPrevio != identificacion.EscalonONI || m.R.Escalon != identificacion.EscalonAlias {
		t.Fatalf("se esperaba resolver la ONI por alias: %+v", m)
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
	soloONI(t, idf, "u-1")
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

	r := ResolverUsos{
		Usos:           ing,
		Identificacion: idf,
		Similitud:      &similitudFalsa{},
		Parametros:     umbralesPorDefecto(),
	}
	n, err := r.ResolverUsos(t.Context(), "2024-06")
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
			idsFuente: "netflix_id=81003997\nseries_id=81004793\nshow_id=80141259",
			quiero:    identificacion.Entrada{Fuente: "netflix", TipoID: "show_id", ValorID: "80141259"},
		},
		{
			nombre:    "netflix sin show_id no produce par aunque traiga netflix_id",
			fuente:    "netflix",
			idsFuente: "netflix_id=81003997",
			quiero:    identificacion.Entrada{Fuente: "netflix"},
		},
		{
			nombre:    "los tres globales se leen",
			fuente:    "caracol",
			idsFuente: "ida=IDA-1\neidr=EIDR-1\nimdb=tt1",
			quiero:    identificacion.Entrada{Fuente: "caracol", IDA: "IDA-1", EIDR: "EIDR-1", IMDB: "tt1"},
		},
		{
			nombre:    "espacios alrededor de clave y valor se recortan",
			fuente:    "caracol",
			idsFuente: "  id_ficha  =  871732  \n imdb = tt0100001 ",
			quiero:    identificacion.Entrada{Fuente: "caracol", TipoID: "id_ficha", ValorID: "871732", IMDB: "tt0100001"},
		},
		{
			nombre:    "valor de solo espacios se ignora: no produce par",
			fuente:    "caracol",
			idsFuente: "id_ficha=   \nimdb=tt0100001",
			quiero:    identificacion.Entrada{Fuente: "caracol", IMDB: "tt0100001"},
		},
		{
			nombre:    "valor sin clave se ignora (contrato estricto)",
			fuente:    "caracol",
			idsFuente: "871732",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "linea basura no pisa un id valido",
			fuente:    "caracol",
			idsFuente: "id_ficha=871732\nimdb=tt0100001\nN/A",
			quiero:    identificacion.Entrada{Fuente: "caracol", TipoID: "id_ficha", ValorID: "871732", IMDB: "tt0100001"},
		},
		{
			nombre:    "clave con otra grafia se ignora (contrato estricto)",
			fuente:    "caracol",
			idsFuente: "ID_Ficha=871732",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "clave fuera del contrato se ignora",
			fuente:    "caracol",
			idsFuente: "foo=bar",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "clave sin valor se ignora",
			fuente:    "caracol",
			idsFuente: "id_ficha=",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "valor con igual vacio a la izquierda se ignora",
			fuente:    "caracol",
			idsFuente: "=871732",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "ids_fuente vacio no produce par ni globales",
			fuente:    "caracol",
			idsFuente: "",
			quiero:    identificacion.Entrada{Fuente: "caracol"},
		},
		{
			nombre:    "clave repetida: gana la ultima",
			fuente:    "caracol",
			idsFuente: "id_ficha=1\nid_ficha=2",
			quiero:    identificacion.Entrada{Fuente: "caracol", TipoID: "id_ficha", ValorID: "2"},
		},
		{
			nombre:    "fuente sin mapeo con una sola clave local la usa",
			fuente:    "sondeo-local",
			idsFuente: "id_pelicula=PX-1",
			quiero:    identificacion.Entrada{Fuente: "sondeo-local", TipoID: "id_pelicula", ValorID: "PX-1"},
		},
		{
			// La regresion que protege: el cine trae hoy una sola clave, pero
			// el dia que el archivo real emita una segunda el fallback
			// alfabetico elegiria id_ficha y los alias bajo id_pelicula
			// dejarian de casar en silencio. El mapeo canonico gana siempre.
			nombre:    "cine con dos claves usa la canonica, no la alfabetica",
			fuente:    "cine",
			idsFuente: "id_ficha=9\nid_pelicula=PX-1",
			quiero:    identificacion.Entrada{Fuente: "cine", TipoID: "id_pelicula", ValorID: "PX-1"},
		},
		{
			nombre:    "fuente sin mapeo con varias claves: la alfabetica",
			fuente:    "otra",
			idsFuente: "show_id=2\nid_pelicula=1",
			quiero:    identificacion.Entrada{Fuente: "otra", TipoID: "id_pelicula", ValorID: "1"},
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

// ---------------------------------------------------------------------------
// U13-U21: el escalon 3 (#32)

const tituloDePrueba = "Titulo de prueba"

func conCandidatos(cs ...identificacion.Candidato) *similitudFalsa {
	return &similitudFalsa{porTitulo: map[string][]identificacion.Candidato{tituloDePrueba: cs}}
}

func cand(obraID, puntaje string) identificacion.Candidato {
	return identificacion.Candidato{ObraID: obraID, Puntaje: decimal.RequireFromString(puntaje)}
}

// ---------------------------------------------------------------------------
// D11: el titulo original tambien se consulta (review de PR #146, B1)

// usoConOriginal es una fila pendiente con titulo emitido y original.
func usoConOriginal(emitido, original string) UsoPersistido {
	u := usoPendiente("u-1", "caracol", "id_ficha=871732")
	u.Titulo = emitido
	u.TituloOrig = original
	return u
}

func TestEntradaDesdeUsoLlevaElTituloOriginal(t *testing.T) {
	e := entradaDesdeUso(usoConOriginal("Sin Tetas No Hay Paraiso", "Without Breasts There Is No Paradise"))
	if e.Titulo != "Sin Tetas No Hay Paraiso" || e.TituloOrig != "Without Breasts There Is No Paradise" {
		t.Fatalf("la entrada perdio un titulo: %+v", e)
	}
}

func TestResolverUsosConsultaElEmitidoYElOriginalYUneLosCandidatos(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := &similitudFalsa{porTitulo: map[string][]identificacion.Candidato{
		"Sin Tetas": {cand("obra-2", "0.50"), cand("obra-1", "0.40")},
		"Without":   {cand("obra-1", "0.52"), cand("obra-3", "0.46")},
	}}

	n, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil, usoConOriginal("Sin Tetas", "Without"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d: nadie llega al umbral 0.60", n)
	}
	// Las dos consultas, el emitido primero, las dos con el piso de la banda.
	if !slices.Equal(sim.llamadas, []string{"Sin Tetas", "Without"}) {
		t.Fatalf("consultas = %v, se esperaba [Sin Tetas Without]", sim.llamadas)
	}
	for _, p := range sim.pisos {
		if !p.Equal(decimal.RequireFromString("0.45")) {
			t.Fatalf("piso = %s, se esperaba el de la banda 0.45", p)
		}
	}

	// Union por obra con el mejor puntaje, en orden total: obra-1 gana con 0.52
	// (venia del original), obra-2 con 0.50, obra-3 con 0.46.
	if len(idf.guardadosCandidatos) != 1 {
		t.Fatalf("se esperaba una bandeja, hubo %+v", idf.guardadosCandidatos)
	}
	quiero := []identificacion.Candidato{
		{ObraID: "obra-1", Puntaje: decimal.RequireFromString("0.52"), TituloConsultado: "Without"},
		{ObraID: "obra-2", Puntaje: decimal.RequireFromString("0.50"), TituloConsultado: "Sin Tetas"},
		{ObraID: "obra-3", Puntaje: decimal.RequireFromString("0.46"), TituloConsultado: "Without"},
	}
	tengo := idf.guardadosCandidatos[0].Candidatos
	if len(tengo) != len(quiero) {
		t.Fatalf("candidatos = %+v, se esperaba %+v", tengo, quiero)
	}
	for i := range quiero {
		if tengo[i].ObraID != quiero[i].ObraID || !tengo[i].Puntaje.Equal(quiero[i].Puntaje) ||
			tengo[i].TituloConsultado != quiero[i].TituloConsultado {
			t.Fatalf("candidato %d = %+v, se esperaba %+v", i, tengo[i], quiero[i])
		}
	}
}

// El mismo titulo dos veces es una consulta de mas en el bucle caro.
func TestResolverUsosNoConsultaElOriginalSiEsElMismoTitulo(t *testing.T) {
	casos := []struct {
		nombre   string
		emitido  string
		original string
	}{
		{"igual", "El Tercer Acto", "El Tercer Acto"},
		{"otras mayusculas", "El Tercer Acto", "EL TERCER ACTO"},
		{"espacios de mas", "El Tercer Acto", "  el   tercer acto "},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			sim := &similitudFalsa{}
			_, err := correrCon(t, &ingestaFalsa{}, &identificacionFalsa{}, sim, umbralesPorDefecto(), nil,
				usoConOriginal(c.emitido, c.original))
			if err != nil {
				t.Fatalf("ResolverUsos: %v", err)
			}
			if len(sim.llamadas) != 1 || sim.llamadas[0] != "El Tercer Acto" {
				t.Fatalf("consultas = %v, se esperaba una sola, la del emitido", sim.llamadas)
			}
		})
	}
}

// El caso que motivo el review: el titulo localizado no se parece a nada y el
// original si. Sin consultar los dos, esta fila iba a ONI.
func TestResolverUsosIdentificaPorElOriginalYLaEvidenciaLoDice(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := &similitudFalsa{porTitulo: map[string][]identificacion.Candidato{
		"Sin Tetas No Hay Paraiso":             {cand("obra-9", "0.10")},
		"Without Breasts There Is No Paradise": {cand("obra-45", "0.88")},
	}}

	n, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
		usoConOriginal("Sin Tetas No Hay Paraiso", "Without Breasts There Is No Paradise"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}
	m := idf.guardadosMatch[0]
	if m.R.Escalon != identificacion.EscalonDifuso || m.R.ObraID != "obra-45" {
		t.Fatalf("match mal armado: %+v", m)
	}
	if !strings.Contains(m.R.Evidencia, "Without Breasts There Is No Paradise") {
		t.Fatalf("la evidencia no dice que caso por el original: %q", m.R.Evidencia)
	}
	if strings.Contains(m.R.Evidencia, "Sin Tetas") {
		t.Fatalf("la evidencia nombra el titulo que NO caso: %q", m.R.Evidencia)
	}
}

// D8: si la segunda consulta falla la corrida aborta, aunque la primera haya ido
// bien, y el error dice que titulo fue.
func TestResolverUsosAbortaSiFallaLaConsultaDelOriginal(t *testing.T) {
	idf := &identificacionFalsa{}
	sim := &similitudFalsa{
		porTitulo:    map[string][]identificacion.Candidato{"Sin Tetas": {cand("obra-45", "0.90")}},
		errPorTitulo: map[string]error{"Without": errors.New("base caida")},
	}

	n, err := correrCon(t, &ingestaFalsa{}, idf, sim, umbralesPorDefecto(), nil, usoConOriginal("Sin Tetas", "Without"))
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if n != 0 || len(idf.guardadosMatch) != 0 {
		t.Fatalf("una fila a medias no se escribe: n=%d %+v", n, idf.guardadosMatch)
	}
	if !strings.Contains(err.Error(), `"Without"`) || !strings.Contains(err.Error(), "escalon 3") {
		t.Fatalf("el error no nombra el titulo que fallo: %v", err)
	}
}

func TestResolverUsosSinTituloNiOriginalNoConsultaYVaAONI(t *testing.T) {
	idf := &identificacionFalsa{}
	sim := &similitudFalsa{}

	if _, err := correrCon(t, &ingestaFalsa{}, idf, sim, umbralesPorDefecto(), nil, usoConOriginal("  ", "")); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if len(sim.llamadas) != 0 {
		t.Fatalf("se consulto con la cadena vacia: %v", sim.llamadas)
	}
	soloONI(t, idf, "u-1")
}

// Solo el original: una fuente que trae el emitido vacio y el original poblado
// se identifica igual.
func TestResolverUsosConsultaSoloElOriginalSiElEmitidoEstaVacio(t *testing.T) {
	idf := &identificacionFalsa{}
	sim := &similitudFalsa{porTitulo: map[string][]identificacion.Candidato{"Rebelde": {cand("obra-7", "0.95")}}}

	n, err := correrCon(t, &ingestaFalsa{}, idf, sim, umbralesPorDefecto(), nil, usoConOriginal("", "Rebelde"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 || !slices.Equal(sim.llamadas, []string{"Rebelde"}) {
		t.Fatalf("n=%d consultas=%v", n, sim.llamadas)
	}
}

func TestResolverUsosResuelvePorDifusoYAprendeAlias(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := conCandidatos(cand("obra-45", "0.83"))

	n, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
		usoPendiente("u-1", "caracol", "id_ficha=871732"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, se esperaba 1", n)
	}

	m := idf.guardadosMatch[0]
	if m.R.Escalon != identificacion.EscalonDifuso || m.R.ObraID != "obra-45" {
		t.Fatalf("match mal armado: %+v", m)
	}
	if !m.R.Puntaje.Equal(decimal.RequireFromString("0.83")) {
		t.Fatalf("el puntaje no llego a la fila: %s", m.R.Puntaje)
	}
	if m.R.ONI {
		t.Fatal("una fila identificada no es ONI")
	}

	// El aprendizaje es lo que hace que la cola encoja (D4).
	if len(idf.guardadosAlias) != 1 {
		t.Fatalf("un match difuso tiene que dejar alias: %+v", idf.guardadosAlias)
	}
	a := idf.guardadosAlias[0]
	if a.Fuente != "caracol" || a.Tipo != "id_ficha" || a.Valor != "871732" ||
		a.ObraID != "obra-45" || a.Quien != quienCascada {
		t.Fatalf("alias mal aprendido: %+v", a)
	}
	// D5: primero el alias, despues el match.
	if !slices.Equal(idf.orden, []string{"alias", "match:u-1"}) {
		t.Fatalf("orden de escritura = %v, se esperaba [alias match:u-1]", idf.orden)
	}
}

func TestResolverUsosBandaAmbiguaNoAsignaYAdjuntaCandidatos(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := conCandidatos(cand("obra-45", "0.52"), cand("obra-99", "0.47"))

	n, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
		usoPendiente("u-1", "caracol", "id_ficha=871732"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0: la banda no asigna", n)
	}

	m := idf.guardadosMatch[0]
	if m.R.ObraID != "" || !m.R.ONI || m.R.Escalon != identificacion.EscalonONI {
		t.Fatalf("la banda ambigua no puede asignar obra: %+v", m)
	}
	// Nunca a ciegas (ADR 0007): ni siquiera el mejor candidato de la banda.
	if len(idf.guardadosAlias) != 0 {
		t.Fatalf("la banda ambigua no aprende alias: %+v", idf.guardadosAlias)
	}

	if len(idf.guardadosCandidatos) != 1 {
		t.Fatalf("se esperaba una escritura de candidatos: %+v", idf.guardadosCandidatos)
	}
	c := idf.guardadosCandidatos[0]
	if c.UsoID != "u-1" || len(c.Candidatos) != 2 {
		t.Fatalf("candidatos mal guardados: %+v", c)
	}
	if c.Candidatos[0].ObraID != "obra-45" || !c.Candidatos[0].Puntaje.Equal(decimal.RequireFromString("0.52")) {
		t.Fatalf("el mejor candidato no llego con su puntaje: %+v", c.Candidatos[0])
	}

	// Candidatos antes del match (D5): al reves quedaria una ONI con la
	// bandeja vacia.
	if !slices.Equal(idf.orden, []string{"candidatos:u-1", "match:u-1"}) {
		t.Fatalf("orden de escritura = %v, se esperaba [candidatos:u-1 match:u-1]", idf.orden)
	}
}

func TestResolverUsosPorDebajoDeLaBandaEsONISinCandidatos(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := conCandidatos(cand("obra-45", "0.20"))

	n, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
		usoPendiente("u-1", "caracol", "id_ficha=871732"))
	if err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	soloONI(t, idf, "u-1")
}

// Criterio de aceptacion: por debajo del umbral, ONI y NO una asignacion
// equivocada. Un falso positivo paga a quien no corresponde (R-05).
func TestResolverUsosNuncaAsignaPorDebajoDelUmbral(t *testing.T) {
	for _, puntaje := range []string{"0.59999", "0.55", "0.45", "0.10"} {
		t.Run(puntaje, func(t *testing.T) {
			ing := &ingestaFalsa{}
			idf := &identificacionFalsa{}
			sim := conCandidatos(cand("obra-45", puntaje))

			if _, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
				usoPendiente("u-1", "caracol", "id_ficha=871732")); err != nil {
				t.Fatalf("ResolverUsos: %v", err)
			}
			if got := idf.guardadosMatch[0].R.ObraID; got != "" {
				t.Fatalf("puntaje %s asigno la obra %q por debajo del umbral 0.60", puntaje, got)
			}
		})
	}
}

// El escalon 3 es la parte cara: si ya hay match, consultarlo es tirarlo.
func TestResolverUsosNoConsultaElDifusoSiYaHayMatch(t *testing.T) {
	casos := []struct {
		nombre string
		idf    *identificacionFalsa
	}{
		{"alias", &identificacionFalsa{alias: map[string]string{"caracol|id_ficha|1": "obra-1"}}},
		{"id global", &identificacionFalsa{porIDGlobal: map[string]string{"||tt0100001": "obra-1"}}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ing := &ingestaFalsa{}
			sim := conCandidatos(cand("obra-99", "0.99"))

			if _, err := correrCon(t, ing, c.idf, sim, umbralesPorDefecto(), nil,
				usoPendiente("u-1", "caracol", "id_ficha=1\nimdb=tt0100001")); err != nil {
				t.Fatalf("ResolverUsos: %v", err)
			}
			if len(sim.llamadas) != 0 {
				t.Fatalf("se consulto el difuso teniendo match: %v", sim.llamadas)
			}
			if idObra := c.idf.guardadosMatch[0].R.ObraID; idObra != "obra-1" {
				t.Fatalf("un parecido de 0.99 desbanco a una igualdad: %q", idObra)
			}
		})
	}
}

func TestResolverUsosNoConsultaElDifusoSinTitulo(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := conCandidatos(cand("obra-45", "0.99"))

	u := usoPendiente("u-1", "caracol", "id_ficha=1")
	u.Titulo = ""

	if _, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil, u); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if len(sim.llamadas) != 0 {
		t.Fatalf("se comparo la cadena vacia contra el catalogo: %v", sim.llamadas)
	}
	soloONI(t, idf, "u-1")
}

// D8: un fallo del motor no es "no hay match". Tragarselo mandaria a ONI una
// fila que quiza se identificaba sola.
func TestResolverUsosAbortaSiFallaElMotorDeSimilitud(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{}
	sim := &similitudFalsa{err: errors.New("base caida")}

	n, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
		usoPendiente("u-1", "caracol", "id_ficha=1"))
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if n != 0 {
		t.Fatalf("n = %d, se esperaba 0", n)
	}
	if len(idf.guardadosMatch) != 0 {
		t.Fatalf("no se puede marcar nada con el motor caido: %+v", idf.guardadosMatch)
	}
	if !strings.Contains(err.Error(), "escalon 3") {
		t.Fatalf("el error no dice que escalon fallo: %v", err)
	}
}

func TestResolverUsosPropagaElErrorDeGuardarCandidatos(t *testing.T) {
	ing := &ingestaFalsa{}
	idf := &identificacionFalsa{errGuardarCandidatos: errors.New("disco lleno")}
	sim := conCandidatos(cand("obra-45", "0.50"))

	_, err := correrCon(t, ing, idf, sim, umbralesPorDefecto(), nil,
		usoPendiente("u-1", "caracol", "id_ficha=1"))
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if len(idf.guardadosMatch) != 0 {
		t.Fatal("sin candidatos escritos no se puede marcar la fila ONI: la bandeja quedaria vacia")
	}
}

// ---------------------------------------------------------------------------
// U22-U25: los umbrales como parametro normativo

// Contra la fecha del PERIODO, no del reloj: lo que se defiende en una
// reclamacion es el criterio vigente cuando se decidio (D3).
func TestResolverUsosLeeLosUmbralesContraLaFechaDelPeriodo(t *testing.T) {
	casos := []struct {
		periodo string
		quiero  time.Time
	}{
		{"2024", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"2024-06", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"2024-12", time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range casos {
		t.Run(c.periodo, func(t *testing.T) {
			ing := &ingestaFalsa{}
			par := umbralesPorDefecto()
			r := ResolverUsos{
				Usos:           ing,
				Identificacion: &identificacionFalsa{},
				Similitud:      &similitudFalsa{},
				Parametros:     par,
			}
			if _, err := r.ResolverUsos(t.Context(), c.periodo); err != nil {
				t.Fatalf("ResolverUsos: %v", err)
			}
			for _, l := range par.llamadas {
				if !l.Fecha.Equal(c.quiero) {
					t.Fatalf("%s se pidio en %s, se esperaba %s", l.Clave, l.Fecha, c.quiero)
				}
			}
		})
	}
}

// Una vez por corrida: por fila, un cambio a mitad de lote partiria la corrida
// en dos criterios sin que nada lo registrara.
func TestResolverUsosLeeLosUmbralesUnaSolaVez(t *testing.T) {
	ing := &ingestaFalsa{}
	par := umbralesPorDefecto()

	usos := make([]UsoPersistido, 0, 5)
	for _, id := range []string{"u-1", "u-2", "u-3", "u-4", "u-5"} {
		usos = append(usos, usoPendiente(id, "caracol", "id_ficha=1"))
	}

	if _, err := correrCon(t, ing, &identificacionFalsa{}, &similitudFalsa{}, par, nil, usos...); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if len(par.llamadas) != 2 {
		t.Fatalf("se leyeron los parametros %d veces, se esperaban 2 (una por clave): %+v",
			len(par.llamadas), par.llamadas)
	}
}

// ADR 0004: ausente es ausente. Con cero por defecto, el umbral asignaria la
// primera obra que se pareciera en algo.
func TestResolverUsosAbortaSiFaltaUnUmbral(t *testing.T) {
	for _, falta := range []string{ClaveUmbralMatch, ClaveUmbralBanda} {
		t.Run(falta, func(t *testing.T) {
			par := umbralesPorDefecto()
			delete(par.valores, falta)

			ing := &ingestaFalsa{}
			idf := &identificacionFalsa{}
			n, err := correrCon(t, ing, idf, &similitudFalsa{}, par, nil,
				usoPendiente("u-1", "caracol", "id_ficha=1"))
			if err == nil {
				t.Fatal("se esperaba un error")
			}
			if n != 0 || len(idf.guardadosMatch) != 0 {
				t.Fatal("sin umbral no se puede tocar ninguna fila")
			}
			if !strings.Contains(err.Error(), falta) {
				t.Fatalf("el error no nombra la clave que falta (%s): %v", falta, err)
			}
			// Ni siquiera se leen los usos: fallar antes evita una corrida a
			// medias que haya que deshacer.
			if len(ing.usosLlamadas) != 0 {
				t.Fatal("se leyeron los usos sin tener los umbrales")
			}
		})
	}
}

// Piso por encima del umbral: nada podria caer en la banda, y filas que
// merecian revision saldrian a ONI ciega.
func TestResolverUsosAbortaSiLaBandaEstaPorEncimaDelUmbral(t *testing.T) {
	par := &parametroEnFechaFalso{valores: map[string]string{
		ClaveUmbralMatch: "0.40",
		ClaveUmbralBanda: "0.70",
	}}
	idf := &identificacionFalsa{}

	_, err := correrCon(t, &ingestaFalsa{}, idf, &similitudFalsa{}, par, nil,
		usoPendiente("u-1", "caracol", "id_ficha=1"))
	if err == nil {
		t.Fatal("se esperaba un error por parametros incoherentes")
	}
	if len(idf.guardadosMatch) != 0 {
		t.Fatal("no se puede tocar ninguna fila con los umbrales incoherentes")
	}
}

func TestResolverUsosExigeMotorYParametros(t *testing.T) {
	casos := []struct {
		nombre string
		r      ResolverUsos
	}{
		{"sin motor", ResolverUsos{Usos: &ingestaFalsa{}, Identificacion: &identificacionFalsa{}, Parametros: umbralesPorDefecto()}},
		{"sin parametros", ResolverUsos{Usos: &ingestaFalsa{}, Identificacion: &identificacionFalsa{}, Similitud: &similitudFalsa{}}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if _, err := c.r.ResolverUsos(t.Context(), "2024"); err == nil {
				t.Fatal("se esperaba un error: una cascada incompleta mandaria a ONI lo que nadie intento identificar")
			}
		})
	}
}

func TestFechaDePeriodoRechazaLoQueNoTieneForma(t *testing.T) {
	for _, malo := range []string{"", "24-06", "2024/06", "junio", "2024-6"} {
		t.Run(malo, func(t *testing.T) {
			if _, err := fechaDePeriodo(malo); err == nil {
				t.Fatalf("fechaDePeriodo(%q) no fallo", malo)
			}
		})
	}
}

// El piso del motor es el MISMO parametro que aplica la cascada: configurado
// aparte, un cambio de parametro no tendria efecto sobre lo que se recupera.
func TestResolverUsosConsultaElMotorConElPisoVigente(t *testing.T) {
	ing := &ingestaFalsa{}
	sim := &similitudFalsa{}
	par := &parametroEnFechaFalso{valores: map[string]string{
		ClaveUmbralMatch: "0.80",
		ClaveUmbralBanda: "0.33",
	}}

	if _, err := correrCon(t, ing, &identificacionFalsa{}, sim, par, nil,
		usoPendiente("u-1", "caracol", "id_ficha=1")); err != nil {
		t.Fatalf("ResolverUsos: %v", err)
	}
	if len(sim.pisos) != 1 {
		t.Fatalf("se esperaba una consulta al motor: %+v", sim.pisos)
	}
	if !sim.pisos[0].Equal(decimal.RequireFromString("0.33")) {
		t.Fatalf("el motor se consulto con piso %s, se esperaba 0.33", sim.pisos[0])
	}
}
