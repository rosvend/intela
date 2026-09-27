package postgres

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Los valores son los del sembrador (semilla/dataset.go). No son datos reales
// -- las deducciones, la reserva y los coeficientes OTT siguen siendo
// sinteticos hasta P-10 --, pero son los que tiene delante quien corre esto en
// local, y eso hace comparables los ids que salen aqui con los de una base
// sembrada.
const (
	reglamentoPond      = "RD 9.1.1"
	reglamentoSintetico = "RD-IX-seed-sintetico"

	organoConsejo  = "Consejo Directivo"
	organoSintetic = "sintetico"
)

// vigenciaBase es el `vigente_desde` de todo el juego: la misma fecha que usa
// el sembrador.
var vigenciaBase = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// juegoCompleto son las diecinueve clausulas que [clausulasDelSnapshot] exige,
// mas las dos tasas del sembrador.
//
// En orden deliberadamente revuelto: el id no puede depender de como lleguen.
func juegoCompleto() []parametroResuelto {
	pond := func(clave, valor string) parametroResuelto {
		return unParametro(clave, valor, organoConsejo, reglamentoPond)
	}
	sint := func(clave, valor string) parametroResuelto {
		return unParametro(clave, valor, organoSintetic, reglamentoSintetico)
	}
	return []parametroResuelto{
		sint("ott.wc", "0.20"),
		pond("ponderacion.cinematografica", "5.0"),
		sint("cambio.USD", "4000"),
		pond("duracion.minutos_hora_tv", "48"),
		sint("deduccion.administrativa", "0.20"),
		pond("ponderacion.serie", "1.3"),
		sint("matching.umbral", "0.60"),
		pond("ponderacion.unitario", "2.8"),
		sint("cambio.EUR", "4300"),
		sint("reserva.errores_tecnicos", "0.05"),
		pond("duracion.artistica_pct", "0.80"),
		sint("ott.wa", "0.50"),
		pond("ponderacion.sketches", "0.8"),
		sint("deduccion.social", "0.10"),
		sint("ott.wb", "0.30"),
		// Grupos de canal (RD 9.5) y asignacion a terceros (RD 9.7), #126.
		// Ya en 0-100, la misma unidad del sembrador (semilla/dataset.go).
		sint("grupo.privados_pct", "50"),
		sint("grupo.regionales_pct", "20"),
		sint("grupo.premium_pct", "10"),
		sint("grupo.lideres_pct", "10"),
		sint("grupo.estandar_pct", "10"),
		sint("asignacion.terceros_pct", "5"),
	}
}

func unParametro(clave, valor, organo, reglamento string) parametroResuelto {
	return parametroResuelto{
		clave:        clave,
		valor:        decimal.RequireFromString(valor),
		organo:       organo,
		reglamento:   reglamento,
		vigenteDesde: vigenciaBase,
	}
}

// sin devuelve el juego completo menos una clausula.
func sin(clave string) []parametroResuelto {
	juego := juegoCompleto()
	return slices.DeleteFunc(juego, func(p parametroResuelto) bool { return p.clave == clave })
}

// ---------------------------------------------------------------------------
// Unidad: la resolucion es una funcion pura y no necesita contenedor
// ---------------------------------------------------------------------------

// Criterio 1: un conjunto completo produce un snapshot poblado con id estable.
func TestArmarSnapshotConElJuegoCompletoLlenaTodosLosHuecos(t *testing.T) {
	id, snap, faltan, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)
	if len(faltan) > 0 {
		t.Fatalf("con el juego completo no puede faltar nada, faltaron %v", faltan)
	}
	if !strings.HasPrefix(id, prefijoSnapshot) || len(id) != len(prefijoSnapshot)+64 {
		t.Fatalf("id = %q, se esperaba %s + 64 hex", id, prefijoSnapshot)
	}

	esperado := map[string]decimal.Decimal{
		"AdminPct":              snap.AdminPct,
		"SocialPct":             snap.SocialPct,
		"ReservaPct":            snap.ReservaPct,
		"PondCine":              snap.PondCine,
		"PondUnitario":          snap.PondUnitario,
		"PondSerie":             snap.PondSerie,
		"PondSketch":            snap.PondSketch,
		"Wa":                    snap.Wa,
		"Wb":                    snap.Wb,
		"Wc":                    snap.Wc,
		"UmbralMatch":           snap.UmbralMatch,
		"DuracionArtisticaPct":  snap.DuracionArtisticaPct,
		"MinutosHoraTV":         snap.MinutosHoraTV,
		"GrupoPrivadosPct":      snap.GrupoPrivadosPct,
		"GrupoRegionalesPct":    snap.GrupoRegionalesPct,
		"GrupoPremiumPct":       snap.GrupoPremiumPct,
		"GrupoLideresPct":       snap.GrupoLideresPct,
		"GrupoEstandarPct":      snap.GrupoEstandarPct,
		"AsignacionTercerosPct": snap.AsignacionTercerosPct,
	}
	// Ningun hueco puede quedarse en cero: el cero de un decimal.Decimal es
	// indistinguible de "no se cableo", y una ponderacion en cero anula la
	// obra entera sin que nada proteste.
	for campo, v := range esperado {
		if v.IsZero() {
			t.Errorf("%s quedo en cero: el hueco no se lleno", campo)
		}
	}
	if !snap.PondCine.Equal(decimal.RequireFromString("5")) {
		t.Errorf("PondCine = %s, se esperaba 5", snap.PondCine)
	}
	// deduccion.* y reserva.* se siembran como fraccion 0-1 ("0.20") y el
	// Snapshot los exige en 0-100 (tipos.go): 20, no 0.20. Es la comprobacion
	// directa del bloqueante 1 -- sin ella, un desalineo de escala pasa esta
	// prueba igual, porque 0.20 tambien es "no cero".
	if !snap.AdminPct.Equal(decimal.RequireFromString("20")) {
		t.Errorf("AdminPct = %s, se esperaba 20 (fraccion 0.20 escalada a porcentaje)", snap.AdminPct)
	}
	if !snap.SocialPct.Equal(decimal.RequireFromString("10")) {
		t.Errorf("SocialPct = %s, se esperaba 10", snap.SocialPct)
	}
	if !snap.ReservaPct.Equal(decimal.RequireFromString("5")) {
		t.Errorf("ReservaPct = %s, se esperaba 5", snap.ReservaPct)
	}
	// duracion.artistica_pct SI se siembra como fraccion ("0.80") y NO se
	// escala: normalizacion la multiplica directo (duracion.go), no pasa por
	// pctDe. Que quede en 0.80 y no en 80 es la mitad de la prueba de la
	// escala: no todo lo que se llama "_pct" se convierte.
	if !snap.DuracionArtisticaPct.Equal(decimal.RequireFromString("0.80")) {
		t.Errorf("DuracionArtisticaPct = %s, se esperaba 0.80 sin escalar", snap.DuracionArtisticaPct)
	}

	if snap.MonedaBase != monedaBase {
		t.Errorf("MonedaBase = %q, se esperaba %q", snap.MonedaBase, monedaBase)
	}
	// Solo se convierte lo que tiene fila (ADR 0004, B4).
	if len(snap.Tasas) != 2 || !snap.Tasas["USD"].Equal(decimal.NewFromInt(4000)) ||
		!snap.Tasas["EUR"].Equal(decimal.NewFromInt(4300)) {
		t.Errorf("Tasas = %v, se esperaban USD=4000 y EUR=4300", snap.Tasas)
	}

	// El reglamento del snapshot son TODOS los que intervinieron, ordenados.
	if quiero := reglamentoPond + "+" + reglamentoSintetico; snap.Reglamento != quiero {
		t.Errorf("Reglamento = %q, se esperaba %q", snap.Reglamento, quiero)
	}
}

// Criterio 2: cada clausula que falte se nombra. Nada de defaults silenciosos
// (ADR 0004).
func TestArmarSnapshotNombraCadaClausulaQueFalta(t *testing.T) {
	for _, c := range clausulasDelSnapshot {
		t.Run(c.clave, func(t *testing.T) {
			id, snap, faltan, _ := armarSnapshot(consumidos(sin(c.clave), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)
			if !slices.Equal(faltan, []string{c.clave}) {
				t.Fatalf("faltan = %v, se esperaba solo %q", faltan, c.clave)
			}
			// Y no se devuelve un snapshot a medias que alguien pueda usar
			// por descuido: sin id no hay nada que referenciar.
			if id != "" || snap.Reglamento != "" || snap.Tasas != nil {
				t.Fatalf("con una clausula ausente no se devuelve snapshot: id=%q snap=%+v", id, snap)
			}
		})
	}
}

// Faltando varias, se nombran TODAS: enterarse de una por intento son tantos
// viajes como parametros sin cargar.
func TestArmarSnapshotNombraTodasLasQueFaltanYEnOrden(t *testing.T) {
	juego := slices.DeleteFunc(juegoCompleto(), func(p parametroResuelto) bool {
		return p.clave == "ott.wb" || p.clave == "deduccion.social"
	})

	_, _, faltan, _ := armarSnapshot(consumidos(juego, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)
	// El orden es el de clausulasDelSnapshot, que es estable entre corridas.
	if quiero := []string{"deduccion.social", "ott.wb"}; !slices.Equal(faltan, quiero) {
		t.Fatalf("faltan = %v, se esperaba %v", faltan, quiero)
	}
}

// Criterio 4, en su forma pura: el id sale del contenido y no del orden de
// llegada ni de la collation de nadie.
func TestElIDNoDependeDelOrdenDeLlegada(t *testing.T) {
	derecho, _, _, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	revuelto := juegoCompleto()
	slices.Reverse(revuelto)
	alReves, _, _, _ := armarSnapshot(consumidos(revuelto, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	if derecho != alReves {
		t.Fatalf("el id cambio con el orden de entrada: %q vs %q", derecho, alReves)
	}
}

// Un valor distinto es OTRO snapshot. Si no, dos repartos calculados con
// deducciones distintas compartirian procedencia.
func TestElIDCambiaSiCambiaUnValor(t *testing.T) {
	antes, _, _, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	juego := juegoCompleto()
	for i := range juego {
		if juego[i].clave == "deduccion.administrativa" {
			juego[i].valor = decimal.RequireFromString("0.19")
		}
	}
	despues, _, _, _ := armarSnapshot(consumidos(juego, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	if antes == despues {
		t.Fatal("cambiar la deduccion administrativa tiene que cambiar el id")
	}
}

// Una tasa cuenta tanto como una ponderacion: cambia la cifra final.
func TestElIDCambiaSiCambiaUnaTasa(t *testing.T) {
	antes, _, _, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	juego := juegoCompleto()
	for i := range juego {
		if juego[i].clave == "cambio.USD" {
			juego[i].valor = decimal.RequireFromString("4100")
		}
	}
	despues, _, _, _ := armarSnapshot(consumidos(juego, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	if antes == despues {
		t.Fatal("cambiar cambio.USD tiene que cambiar el id")
	}
}

// Lo que el snapshot no consume no puede cambiar su id: si entrara en el hash,
// cargar un parametro de otro modulo haria que la misma fecha resolviera a
// otro id sin que ninguna cifra del reparto cambiara.
func TestUnParametroAjenoNoCambiaElID(t *testing.T) {
	solo, _, _, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	conAjeno := append(juegoCompleto(),
		unParametro("calendario.corte_rendimientos", "20", organoConsejo, "RD 10.1"))
	acompanado, snap, faltan, _ := armarSnapshot(consumidos(conAjeno, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	if len(faltan) > 0 {
		t.Fatalf("un parametro de mas no puede hacer que falte nada: %v", faltan)
	}
	if solo != acompanado {
		t.Fatalf("un parametro que el snapshot no lee cambio el id: %q vs %q", solo, acompanado)
	}
	// Y tampoco se cuela en el reglamento del snapshot.
	if strings.Contains(snap.Reglamento, "RD 10.1") {
		t.Errorf("Reglamento = %q: incluyo un reglamento que el snapshot no consume", snap.Reglamento)
	}
}

// La procedencia NO entra en el id: corregir una errata en el nombre del
// organo no cambia ni una cifra, y si cambiara el id dejaria huerfana a la
// corrida que lo referencia.
func TestLaProcedenciaNoEntraEnElID(t *testing.T) {
	antes, _, _, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	juego := juegoCompleto()
	for i := range juego {
		juego[i].organo = "Consejo Directivo (acta 12)"
	}
	despues, _, _, _ := armarSnapshot(consumidos(juego, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	if antes != despues {
		t.Fatalf("corregir el organo cambio el id: %q vs %q", antes, despues)
	}
}

// La escala de la columna es la que fija la forma canonica: 0.2 y 0.200000 son
// el mismo valor normativo y tienen que dar el mismo id.
func TestElIDNoDependeDeLaEscalaConQueLlegueElValor(t *testing.T) {
	escueto, _, _, _ := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	juego := juegoCompleto()
	for i := range juego {
		if juego[i].clave == "deduccion.administrativa" {
			juego[i].valor = decimal.RequireFromString("0.2000000")
		}
	}
	rellenado, _, _, _ := armarSnapshot(consumidos(juego, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)

	if escueto != rellenado {
		t.Fatalf("la escala de entrada cambio el id: %q vs %q", escueto, rellenado)
	}
}

// Criterio 3, sobre el Snapshot y no sobre el id: todo campo decimal.Decimal
// de [reparto.Snapshot] tiene que tener clausula.
//
// #126 anadio seis campos al struct (GrupoPrivadosPct y companeros) y este
// adaptador siguio compilando sin darles clausula: el motor los consumia como
// cero sin que ErrorParametroAusente saliera nunca, porque para
// clausulasDelSnapshot esas claves sencillamente no existian (bloqueante 3,
// PR #134). La prueba de arriba enumeraba los campos A MANO -- exactamente lo
// que dejo pasar el descuido --; esta recorre el struct con reflect, asi que
// el PROXIMO campo que alguien anada a Snapshot sin clausula rompe aqui en
// vez de resolver a cero en produccion.
func TestClausulasDelSnapshotCubrenTodosLosCamposDecimal(t *testing.T) {
	marca := decimal.RequireFromString("123.456")
	tipoDecimal := reflect.TypeOf(decimal.Decimal{})

	tocados := make(map[string]bool)
	for _, c := range clausulasDelSnapshot {
		var s reparto.Snapshot
		c.en(&s, marca)

		v := reflect.ValueOf(s)
		encontrado := ""
		for _, f := range reflect.VisibleFields(reflect.TypeOf(s)) {
			if f.Type != tipoDecimal {
				continue
			}
			campo := v.FieldByIndex(f.Index).Interface().(decimal.Decimal)
			if campo.Equal(marca) {
				if encontrado != "" {
					t.Fatalf("la clausula %q escribio dos campos a la vez: %s y %s",
						c.clave, encontrado, f.Name)
				}
				encontrado = f.Name
			}
		}
		if encontrado == "" {
			t.Fatalf("la clausula %q no escribio ningun campo decimal.Decimal", c.clave)
		}
		if tocados[encontrado] {
			t.Errorf("%s ya tiene clausula: %q lo vuelve a escribir", encontrado, c.clave)
		}
		tocados[encontrado] = true
	}

	for _, f := range reflect.VisibleFields(reflect.TypeOf(reparto.Snapshot{})) {
		if f.Type != tipoDecimal {
			continue
		}
		if !tocados[f.Name] {
			t.Errorf("reparto.Snapshot.%s no tiene clausula en clausulasDelSnapshot: se queda en cero sin que nada proteste", f.Name)
		}
	}
}

// La moneda base entra en el digest aunque hoy sea una constante de Go: si
// cambiara sin mover el id, SnapshotPorID reinterpretaria en silencio las
// tasas ya congeladas contra otra moneda (bloqueante 6, PR #134).
func TestElIDCambiaSiCambiaLaMonedaBase(t *testing.T) {
	pares := consumidos(juegoCompleto(), clausulasDelSnapshot)
	cop := idDeSnapshot(pares, "COP", prefijoSnapshot)
	usd := idDeSnapshot(pares, "USD", prefijoSnapshot)
	if cop == usd {
		t.Fatal("cambiar la moneda base tiene que cambiar el id")
	}
}

// El prefijo nombra la version del conjunto de clausulas (N1-b, PR #134): dos
// pares IDENTICOS bajo dos versiones distintas tienen que dar ids distintos,
// o un id viejo podria colisionar con uno nuevo que en realidad se armo con
// otro conjunto de clausulas.
func TestElIDCambiaSiCambiaLaVersionDelPrefijo(t *testing.T) {
	pares := consumidos(juegoCompleto(), clausulasDelSnapshot)
	v1 := idDeSnapshot(pares, monedaBase, "snp1-")
	v2 := idDeSnapshot(pares, monedaBase, "snp2-")
	if v1 == v2 {
		t.Fatal("cambiar el prefijo de version tiene que cambiar el id")
	}
}

// cambio.USD y cambio.usd son dos filas legitimas para el esquema -- `clave`
// no tiene collation especial -- pero normalizan al mismo Tasas["USD"]. Sin
// deteccion, la que ordena despues por bytes pisa a la otra en silencio
// (bloqueante 6, PR #134): un factor de conversion que alguien cargo de
// verdad desaparece sin error.
func TestDosClavesDeCambioQueNormalizanAlMismoISOSonAmbiguas(t *testing.T) {
	juego := append(juegoCompleto(), unParametro("cambio.usd", "4050", organoSintetic, reglamentoSintetico))

	_, _, _, err := armarSnapshot(consumidos(juego, clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)
	var ambigua *aplicacion.ErrorTasaAmbigua
	if !errors.As(err, &ambigua) {
		t.Fatalf("se esperaba *aplicacion.ErrorTasaAmbigua, dio: %v", err)
	}
	if !errors.Is(err, aplicacion.ErrTasaAmbigua) {
		t.Errorf("errors.Is(err, ErrTasaAmbigua) tiene que reconocerlo")
	}
	if ambigua.Codigo != "USD" {
		t.Errorf("Codigo = %q, se esperaba USD", ambigua.Codigo)
	}
	if !slices.Contains(ambigua.Claves, "cambio.USD") || !slices.Contains(ambigua.Claves, "cambio.usd") {
		t.Errorf("Claves = %v, se esperaban cambio.USD y cambio.usd", ambigua.Claves)
	}
}

// TestArmarSnapshotEscalaLasDeduccionesParaElMotor es la prueba dorada PURA
// del bloqueante 1: engancha lo que arma este adaptador con lo que
// reparto.Reparto consume, para que un desalineo de escala como el que hubo
// -0.20 fraccion tratado como 0.20 porcentaje- rompa en una prueba y no en una
// bolsa real. Una prueba de armarSnapshot sola solo comprueba SU lado del
// contrato; esta comprueba los dos lados a la vez.
//
// Es deliberadamente PURA -llama a armarSnapshot, no a Store.SnapshotEnFecha-
// para poder correr sin Postgres y dar esta senal en milisegundos. La version
// de punta a punta -datos sembrados, congelados de verdad, releidos por id- es
// [TestSnapshotEnFechaEscalaLasDeduccionesParaElMotorIntegracion], mas abajo,
// en la seccion de integracion.
func TestArmarSnapshotEscalaLasDeduccionesParaElMotor(t *testing.T) {
	_, snap, faltan, err := armarSnapshot(consumidos(juegoCompleto(), clausulasDelSnapshot), clausulasDelSnapshot, prefijoSnapshot)
	if err != nil {
		t.Fatalf("armarSnapshot: %v", err)
	}
	if len(faltan) > 0 {
		t.Fatalf("faltan clausulas: %v", faltan)
	}
	// BaseCineTeatro queda fuera de clausulasDelSnapshot (es texto, no
	// decimal.Decimal; #34 lo cablea) pero puntosCineTeatro lo exige.
	snap.BaseCineTeatro = reparto.BaseEspectadores

	bruto := decimal.RequireFromString("1000000000") // mil millones de pesos
	bolsa, err := recaudo.NuevaBolsa("canal-z", "2024-01", recaudo.Nacional, bruto)
	if err != nil {
		t.Fatalf("NuevaBolsa: %v", err)
	}
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica",
			DuracionMin: decimal.NewFromInt(70), Emisiones: 1, Rating: decimal.NewFromInt(1)},
	}
	decls := []repertorio.Declaracion{{ObraID: "x", Partes: []repertorio.Parte{
		{TitularID: "t", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(100)},
	}}}

	r, err := reparto.Reparto(bolsa, usos, snap, decls, reparto.Opciones{})
	if err != nil {
		t.Fatalf("Reparto: %v", err)
	}

	// R-06 (hasta 20% admin), R-07 (hasta 10% social, RD 14.5.1: hasta 5%
	// reserva). Sobre mil millones eso es 200/100/50 millones -- no
	// 2/1/0.05 millones, que es lo que salia con la fraccion sin escalar.
	casos := map[string]struct {
		got, want decimal.Decimal
	}{
		"Admin":   {r.Admin, decimal.RequireFromString("200000000")},
		"Social":  {r.Social, decimal.RequireFromString("100000000")},
		"Reserva": {r.Reserva, decimal.RequireFromString("50000000")},
	}
	for campo, c := range casos {
		if !c.got.Equal(c.want) {
			t.Errorf("%s = %s, se esperaba %s", campo, c.got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Integracion: contra PostgreSQL de verdad (ADR 0010)
// ---------------------------------------------------------------------------

// sembrarParametros deja el juego completo vigente desde `desde` y sin cierre.
func sembrarParametros(t *testing.T, pool *pgxpool.Pool, desde string) {
	t.Helper()

	for _, p := range juegoCompleto() {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
			 VALUES ($1, $2, $3::date, $4, $5)`,
			p.clave, p.valor, desde, p.organo, p.reglamento); err != nil {
			t.Fatalf("sembrar el parametro %s: %v", p.clave, err)
		}
	}
}

// cerrarVigencia pone fecha de fin a la fila que empezo en `desde`.
func cerrarVigencia(t *testing.T, pool *pgxpool.Pool, clave, desde, hasta string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE parametros SET vigente_hasta = $3::date
		  WHERE clave = $1 AND vigente_desde = $2::date`, clave, desde, hasta); err != nil {
		t.Fatalf("cerrar la vigencia de %s: %v", clave, err)
	}
}

func nuevaVigencia(t *testing.T, pool *pgxpool.Pool, clave, valor, desde, hasta string) {
	t.Helper()
	var fin any
	if hasta != "" {
		fin = hasta
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO parametros (clave, valor, vigente_desde, vigente_hasta, organo, reglamento)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6)`,
		clave, valor, desde, fin, organoSintetic, reglamentoSintetico); err != nil {
		t.Fatalf("insertar la vigencia de %s desde %s: %v", clave, desde, err)
	}
}

func fecha(t *testing.T, dia string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, dia)
	if err != nil {
		t.Fatalf("fecha de prueba %q: %v", dia, err)
	}
	return d
}

// TestSnapshotEnFechaEscalaLasDeduccionesParaElMotorIntegracion es la version
// de PUNTA A PUNTA de [TestArmarSnapshotEscalaLasDeduccionesParaElMotor]: no
// llama a armarSnapshot, llama a Store.SnapshotEnFecha sobre datos sembrados
// en Postgres real, deja que congele, lo relee por id con Store.SnapshotPorID
// -para probar tambien el camino de almacenamiento y recarga, no solo el de
// resolucion- y alimenta reparto.Reparto con las dos copias. Cubre los cuatro
// eslabones que la revision de segunda ronda de PR #134 pidio comprobar
// juntos: datos (sembrados), almacenamiento (congelar), recarga (leer por
// id) y motor (Reparto).
func TestSnapshotEnFechaEscalaLasDeduccionesParaElMotorIntegracion(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	id, congelado, err := store.SnapshotEnFecha(ctx, fecha(t, "2024-06-01"))
	if err != nil {
		t.Fatalf("SnapshotEnFecha: %v", err)
	}
	releido, err := store.SnapshotPorID(ctx, id)
	if err != nil {
		t.Fatalf("SnapshotPorID: %v", err)
	}

	bruto := decimal.RequireFromString("1000000000") // mil millones de pesos
	bolsa, err := recaudo.NuevaBolsa("canal-z", "2024-01", recaudo.Nacional, bruto)
	if err != nil {
		t.Fatalf("NuevaBolsa: %v", err)
	}
	usos := []reparto.Uso{
		{ObraID: "x", Modalidad: reparto.TV, TipoObra: "cinematografica",
			DuracionMin: decimal.NewFromInt(70), Emisiones: 1, Rating: decimal.NewFromInt(1)},
	}
	decls := []repertorio.Declaracion{{ObraID: "x", Partes: []repertorio.Parte{
		{TitularID: "t", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(100)},
	}}}

	// R-06 (hasta 20% admin), R-07 (hasta 10% social, RD 14.5.1: hasta 5%
	// reserva). Sobre mil millones eso es 200/100/50 millones -- no
	// 2/1/0.05 millones, que es lo que salia con la fraccion sin escalar.
	quiero := map[string]decimal.Decimal{
		"Admin":   decimal.RequireFromString("200000000"),
		"Social":  decimal.RequireFromString("100000000"),
		"Reserva": decimal.RequireFromString("50000000"),
	}

	for nombre, snap := range map[string]reparto.Snapshot{"recien resuelto": congelado, "releido por id": releido} {
		t.Run(nombre, func(t *testing.T) {
			// BaseCineTeatro queda fuera de clausulasDelSnapshot (es texto, no
			// decimal.Decimal; #34 lo cablea) pero puntosCineTeatro lo exige.
			snap.BaseCineTeatro = reparto.BaseEspectadores

			r, err := reparto.Reparto(bolsa, usos, snap, decls, reparto.Opciones{})
			if err != nil {
				t.Fatalf("Reparto: %v", err)
			}
			got := map[string]decimal.Decimal{"Admin": r.Admin, "Social": r.Social, "Reserva": r.Reserva}
			for campo, want := range quiero {
				if !got[campo].Equal(want) {
					t.Errorf("%s = %s, se esperaba %s", campo, got[campo], want)
				}
			}
		})
	}
}

// Tres tramos de la misma clausula y tres fechas: cada una tiene que caer en
// el suyo. Es la prueba que un doble no puede dar, porque quien garantiza que
// hay UN solo tramo por fecha es la EXCLUDE del esquema.
func TestSnapshotEnFechaEligeElTramoDeCadaFecha(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")

	// deduccion.administrativa en tres tramos consecutivos y sin hueco.
	cerrarVigencia(t, pool, "deduccion.administrativa", "2024-01-01", "2025-01-01")
	nuevaVigencia(t, pool, "deduccion.administrativa", "0.21", "2025-01-01", "2026-01-01")
	nuevaVigencia(t, pool, "deduccion.administrativa", "0.22", "2026-01-01", "")

	// quiero es AdminPct DESPUES de escalar: `parametros.valor` guarda la
	// fraccion (0.21), el Snapshot la exige en porcentaje (21).
	casos := []struct {
		dia    string
		quiero string
	}{
		{"2024-06-30", "20"},
		{"2025-06-30", "21"},
		{"2026-06-30", "22"},
	}
	ids := map[string]string{}
	for _, c := range casos {
		id, snap, err := store.SnapshotEnFecha(t.Context(), fecha(t, c.dia))
		if err != nil {
			t.Fatalf("SnapshotEnFecha(%s): %v", c.dia, err)
		}
		if !snap.AdminPct.Equal(decimal.RequireFromString(c.quiero)) {
			t.Errorf("en %s AdminPct = %s, se esperaba %s", c.dia, snap.AdminPct, c.quiero)
		}
		ids[id] = c.dia
	}
	// Tres valores distintos son tres snapshots distintos.
	if len(ids) != len(casos) {
		t.Fatalf("tres tramos distintos dieron %d ids: %v", len(ids), ids)
	}
}

// El borde es el del daterange '[)': el dia de inicio ya rige, el de cierre ya
// no. Tiene que ser el MISMO que el de la EXCLUDE, o habria fechas que la
// restriccion considera libres y la consulta considera ocupadas.
func TestSnapshotEnFechaRespetaElBordeSemiabierto(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	cerrarVigencia(t, pool, "deduccion.administrativa", "2024-01-01", "2025-01-01")
	nuevaVigencia(t, pool, "deduccion.administrativa", "0.21", "2025-01-01", "")

	casos := []struct{ dia, quiero string }{
		{"2024-12-31", "20"},
		{"2025-01-01", "21"},
	}
	for _, c := range casos {
		_, snap, err := store.SnapshotEnFecha(t.Context(), fecha(t, c.dia))
		if err != nil {
			t.Fatalf("SnapshotEnFecha(%s): %v", c.dia, err)
		}
		if !snap.AdminPct.Equal(decimal.RequireFromString(c.quiero)) {
			t.Errorf("en %s AdminPct = %s, se esperaba %s", c.dia, snap.AdminPct, c.quiero)
		}
	}
}

// Criterio 4: resolver la misma fecha dos veces da el mismo id, y no deja un
// segundo snapshot.
func TestResolverDosVecesLaMismaFechaDaElMismoID(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	primero, _, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31"))
	if err != nil {
		t.Fatalf("primera resolucion: %v", err)
	}
	// Otro instante del mismo dia: la comparacion es de fecha, no de hora.
	segundo, _, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31").Add(23*time.Hour))
	if err != nil {
		t.Fatalf("segunda resolucion: %v", err)
	}
	if primero != segundo {
		t.Fatalf("la misma fecha dio dos ids: %q y %q", primero, segundo)
	}

	var snapshots int
	if err := pool.QueryRow(ctx,
		`SELECT count(DISTINCT snapshot_id) FROM snapshots_parametros`).Scan(&snapshots); err != nil {
		t.Fatalf("contar snapshots: %v", err)
	}
	if snapshots != 1 {
		t.Fatalf("quedaron %d snapshots congelados, se esperaba 1", snapshots)
	}
}

// Criterio 3: lo que devuelve SnapshotPorID es lo mismo que devolvio la
// resolucion, valor a valor y en su forma canonica.
func TestSnapshotPorIDDevuelveLosMismosValores(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	id, resuelto, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31"))
	if err != nil {
		t.Fatalf("SnapshotEnFecha: %v", err)
	}

	leido, err := store.SnapshotPorID(ctx, id)
	if err != nil {
		t.Fatalf("SnapshotPorID: %v", err)
	}
	compararSnapshots(t, resuelto, leido)
}

// Criterio 5: cambiar vigencias despues de congelar no toca lo congelado.
//
// Es el criterio entero de las dos ADR: si esto falla, una fila cargada hoy
// cambia la cifra de un reparto pagado el ano pasado y nadie se entera.
func TestCambiarVigenciasNoAfectaAUnSnapshotYaCongelado(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	id, congelado, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31"))
	if err != nil {
		t.Fatalf("SnapshotEnFecha: %v", err)
	}

	// La Asamblea aprueba otra deduccion, con efecto retroactivo al periodo ya
	// repartido: se cierra el tramo viejo y se abre uno nuevo que cubre la
	// misma fecha.
	if _, err := pool.Exec(ctx,
		`DELETE FROM parametros WHERE clave = 'deduccion.administrativa'`); err != nil {
		t.Fatalf("retirar la vigencia: %v", err)
	}
	nuevaVigencia(t, pool, "deduccion.administrativa", "0.15", "2024-01-01", "")

	// La resolucion nueva ve el valor nuevo...
	nuevoID, nuevo, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31"))
	if err != nil {
		t.Fatalf("resolver despues del cambio: %v", err)
	}
	if !nuevo.AdminPct.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("la resolucion nueva no vio el cambio: AdminPct = %s", nuevo.AdminPct)
	}
	if nuevoID == id {
		t.Fatal("dos conjuntos de valores distintos no pueden compartir id")
	}

	// ...y el snapshot congelado sigue diciendo lo que decia.
	releido, err := store.SnapshotPorID(ctx, id)
	if err != nil {
		t.Fatalf("SnapshotPorID despues del cambio: %v", err)
	}
	compararSnapshots(t, congelado, releido)
}

// TestSnapshotEnFechaDevuelveLaProcedenciaRealmenteCongeladaYNoLaDeLaResolucion
// cubre el caso normal que dejaba pasar el bloqueante 6: una Asamblea nueva
// ratifica el MISMO valor con otra procedencia (nuevo organo, nuevo
// reglamento, nuevo vigente_desde). El id no cambia -no consume procedencia-,
// asi que congelar cae en ON CONFLICT DO NOTHING y la tabla se queda con la
// procedencia de la PRIMERA congelacion.
//
// Antes del arreglo, SnapshotEnFecha devolvia el Reglamento de la resolucion
// RECIEN hecha -la de la segunda Asamblea-, que SnapshotPorID(mismoID) jamas
// volveria a dar: dos lecturas del mismo id, dos reglamentos. Ahora
// SnapshotEnFecha relee lo que de verdad quedo grabado, asi que las dos
// lecturas coinciden siempre.
func TestSnapshotEnFechaDevuelveLaProcedenciaRealmenteCongeladaYNoLaDeLaResolucion(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	primerID, primero, err := store.SnapshotEnFecha(ctx, fecha(t, "2024-06-01"))
	if err != nil {
		t.Fatalf("primera congelacion: %v", err)
	}

	// Otra Asamblea ratifica el MISMO 10% de deduccion.social, con otro acto y
	// otra fecha de vigencia -no otro valor-.
	cerrarVigencia(t, pool, "deduccion.social", "2024-01-01", "2025-01-01")
	if _, err := pool.Exec(ctx,
		`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
		 VALUES ('deduccion.social', '0.10', '2025-01-01', 'Asamblea General', 'RD 14.5.1 (ratificacion 2025)')`); err != nil {
		t.Fatalf("insertar la ratificacion: %v", err)
	}

	segundoID, segundo, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-06-01"))
	if err != nil {
		t.Fatalf("segunda congelacion: %v", err)
	}
	if segundoID != primerID {
		t.Fatalf("mismo valor tiene que dar el mismo id: %q vs %q", primerID, segundoID)
	}
	// La comprobacion central: el Reglamento que ve quien llama a
	// SnapshotEnFecha por segunda vez tiene que ser el que YA estaba
	// congelado, no el de la ratificacion que perdio el ON CONFLICT.
	if segundo.Reglamento != primero.Reglamento {
		t.Fatalf("Reglamento cambio para el MISMO id: %q vs %q; SnapshotEnFecha tiene que devolver la procedencia realmente congelada",
			primero.Reglamento, segundo.Reglamento)
	}
	if strings.Contains(segundo.Reglamento, "ratificacion 2025") {
		t.Errorf("Reglamento = %q: se colo la procedencia que ON CONFLICT DO NOTHING debio descartar", segundo.Reglamento)
	}

	// Y quien lea por ID mas tarde ve exactamente lo mismo que las dos
	// llamadas anteriores: las tres lecturas del mismo id concuerdan.
	releido, err := store.SnapshotPorID(ctx, segundoID)
	if err != nil {
		t.Fatalf("SnapshotPorID: %v", err)
	}
	compararSnapshots(t, segundo, releido)
}

// TestUnaEscrituraParcialPreviaSeRechazaYNoDejaFilasNuevas es el otro lado del
// bloqueante 6: no solo que ON CONFLICT DO NOTHING resuelva una REPETICION
// completa (los dos tests de arriba), sino que una escritura A MEDIAS -una
// corrida anterior que se corto entre el primer y el ultimo INSERT del mismo
// id, dejando algunas filas congeladas y otras no- se detecte y se rechace, y
// que el rechazo se lleve consigo las filas que ESTA llamada alcanzo a
// insertar antes de fallar. Sin esto, congelar() detecta la escritura parcial
// pero podria dejarla a medio corregir: unas filas de la corrida vieja, otras
// de la nueva, y ninguna transaccion completa.
func TestUnaEscrituraParcialPreviaSeRechazaYNoDejaFilasNuevas(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	// El id que SnapshotEnFecha va a resolver para este juego de datos:
	// idDeSnapshot solo hashea clave y valor (nunca la vigencia), asi que
	// calcularlo aqui, puro y sin tocar la base, da el mismo id que la
	// resolucion real de mas abajo.
	pares := consumidos(juegoCompleto(), clausulasDelSnapshot)
	id, _, faltan, err := armarSnapshot(pares, clausulasDelSnapshot, prefijoSnapshot)
	if err != nil || len(faltan) > 0 {
		t.Fatalf("armarSnapshot: err=%v faltan=%v", err, faltan)
	}

	// Simula la corrida cortada: 3 de las filas de ese id ya estan grabadas,
	// como si un INSERT anterior hubiera llegado hasta ahi y el proceso
	// hubiera muerto antes de terminar. Insertadas por fuera de
	// Store.congelar a proposito: es la unica forma de dejar la tabla en un
	// estado que congelar() nunca produce por si solo, porque su propio
	// INSERT es una sola sentencia atomica.
	parcial := pares[:3]
	for _, p := range parcial {
		if _, err := pool.Exec(ctx,
			`INSERT INTO snapshots_parametros (snapshot_id, clave, valor, organo, reglamento, vigente_desde)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			id, p.clave, p.valor, p.organo, p.reglamento, texto(p.vigenteDesde)); err != nil {
			t.Fatalf("sembrar la fila parcial %s: %v", p.clave, err)
		}
	}

	// La resolucion real choca con esas filas: ON CONFLICT DO NOTHING las
	// salta e inserta el resto, y RowsAffected no da ni 0 ni el total.
	if _, _, err := store.SnapshotEnFecha(ctx, fecha(t, "2024-06-01")); err == nil {
		t.Fatal("una escritura parcial preexistente tiene que rechazar la resolucion, no completarla en silencio")
	} else if !strings.Contains(err.Error(), "escritura parcial") {
		t.Fatalf("el error tiene que nombrar la escritura parcial, dio: %v", err)
	}

	// Y el rollback de la transaccion tiene que deshacer las filas que ESTA
	// llamada alcanzo a insertar antes de que congelar() detectara el numero
	// equivocado: la tabla se queda exactamente como estaba -las 3 de antes,
	// ni una mas-, no con filas nuevas huerfanas de un id que nunca llego a
	// ser un snapshot valido.
	var enTabla int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM snapshots_parametros WHERE snapshot_id = $1`, id).Scan(&enTabla); err != nil {
		t.Fatalf("contar filas del id: %v", err)
	}
	if enTabla != len(parcial) {
		t.Fatalf("quedaron %d filas bajo el id, se esperaban %d (rollback incompleto)", enTabla, len(parcial))
	}

	// Consecuencia directa: ese id sigue sin ser un snapshot valido. Nadie que
	// lo lea por SnapshotPorID lo reconstruye con solo 3 de sus filas.
	if _, err := store.SnapshotPorID(ctx, id); !errors.Is(err, aplicacion.ErrSnapshotCorrupto) {
		t.Fatalf("SnapshotPorID sobre el id parcial tiene que fallar con ErrSnapshotCorrupto, dio: %v", err)
	}
}

// Criterio 2 contra la base: la clausula que falta se nombra, y el error es
// tipado por las dos vias -- errors.Is para "falta un parametro" y errors.As
// para saber cual --.
func TestSnapshotEnFechaConUnaClausulaSinVigenciaFallaNombrandola(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")

	if _, err := pool.Exec(t.Context(),
		`DELETE FROM parametros WHERE clave = 'ott.wc'`); err != nil {
		t.Fatalf("retirar ott.wc: %v", err)
	}

	_, _, err := store.SnapshotEnFecha(t.Context(), fecha(t, "2025-01-31"))
	if !errors.Is(err, aplicacion.ErrParametroAusente) {
		t.Fatalf("se esperaba ErrParametroAusente, dio: %v", err)
	}
	var ausente *aplicacion.ErrorParametroAusente
	if !errors.As(err, &ausente) {
		t.Fatalf("el error tiene que nombrar las clausulas, dio: %v", err)
	}
	if !slices.Equal(ausente.Claves, []string{"ott.wc"}) {
		t.Fatalf("Claves = %v, se esperaba [ott.wc]", ausente.Claves)
	}
	if !strings.Contains(err.Error(), "ott.wc") || !strings.Contains(err.Error(), "2025-01-31") {
		t.Errorf("el mensaje tiene que llevar la clausula y la fecha, dio: %v", err)
	}
	// Y no se congela nada: un snapshot a medias es peor que ninguno.
	var filas int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM snapshots_parametros`).Scan(&filas); err != nil {
		t.Fatalf("contar filas congeladas: %v", err)
	}
	if filas != 0 {
		t.Fatalf("quedaron %d filas congeladas de un snapshot que fallo", filas)
	}
}

// Una vigencia que empieza DESPUES del periodo es una clausula ausente para
// ese periodo, no un valor que se pueda adelantar.
func TestUnaVigenciaPosteriorAlPeriodoCuentaComoAusente(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2026-01-01")

	_, _, err := store.SnapshotEnFecha(t.Context(), fecha(t, "2025-06-30"))
	var ausente *aplicacion.ErrorParametroAusente
	if !errors.As(err, &ausente) {
		t.Fatalf("se esperaba ErrorParametroAusente, dio: %v", err)
	}
	if len(ausente.Claves) != len(clausulasDelSnapshot) {
		t.Fatalf("faltaron %d clausulas, se esperaban las %d",
			len(ausente.Claves), len(clausulasDelSnapshot))
	}
}

func TestSnapshotPorIDDesconocidoEsNoEncontrado(t *testing.T) {
	store, _ := colaVacia(t)

	_, err := store.SnapshotPorID(t.Context(), prefijoSnapshot+strings.Repeat("0", 64))
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, dio: %v", err)
	}
}

// Un id que no tiene ni la FORMA general snp<version>-<hex64> -por ejemplo,
// uno de antes de N1-b, o uno escrito a mano- falla cerrado sin llegar a
// tocar la tabla. No es ErrNoEncontrado: la forma esta mal, no falta la fila.
func TestSnapshotPorIDConFormaInvalidaFallaCerrado(t *testing.T) {
	store, _ := colaVacia(t)

	casos := []string{
		"no-es-un-snapshot",
		"snp-" + strings.Repeat("0", 64), // forma pre-N1-b, sin numero de version
		"snp1-corto",
	}
	for _, id := range casos {
		t.Run(id, func(t *testing.T) {
			_, err := store.SnapshotPorID(t.Context(), id)
			if !errors.Is(err, aplicacion.ErrSnapshotCorrupto) {
				t.Fatalf("se esperaba ErrSnapshotCorrupto, dio: %v", err)
			}
			if errors.Is(err, aplicacion.ErrNoEncontrado) {
				t.Errorf("una forma invalida no es un snapshot ausente: %v", err)
			}
		})
	}
}

// N1-b (revision de PR #134): un id de una version que este binario no
// registra en [clausulasPorVersion] falla cerrado, nombrando la version
// pedida y las que conoce -- nunca reinterpreta las filas con el conjunto de
// clausulas de HOY, que es la forma en que "anadir una clausula" se volveria
// silenciosamente destructiva sobre snapshots ya congelados.
func TestSnapshotPorIDConVersionDesconocidaFallaCerrado(t *testing.T) {
	store, _ := colaVacia(t)

	id := "snp999-" + strings.Repeat("a", 64)
	_, err := store.SnapshotPorID(t.Context(), id)
	if !errors.Is(err, aplicacion.ErrSnapshotCorrupto) {
		t.Fatalf("se esperaba ErrSnapshotCorrupto, dio: %v", err)
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("el mensaje tiene que nombrar la version pedida (999), dio: %v", err)
	}
	if !strings.Contains(err.Error(), "1") {
		t.Errorf("el mensaje tiene que nombrar las versiones conocidas (1), dio: %v", err)
	}
}

// TestSnapshotPorIDReconstruyeConLasClausulasDeSuPropiaVersionYNoConLasActuales
// es la prueba central de N1-b: "preservar la reconstruccion vieja cuando se
// anada una version futura".
//
// Simula ese futuro sin esperar a que exista de verdad: registra una version
// "vieja" (99) con un SUBCONJUNTO de las clausulas actuales -como si
// `asignacion.terceros_pct` no hubiera existido todavia cuando ese id se
// congelo-, planta directamente en `snapshots_parametros` las filas de ese
// subconjunto bajo un id calculado con esa version, y comprueba que
// `SnapshotPorID` lo reconstruye SIN pedir `asignacion.terceros_pct` -que
// haria falta si, por error, reconstruyera contra el conjunto ACTUAL (mas
// ancho) en vez de contra el de la version 99.
func TestSnapshotPorIDReconstruyeConLasClausulasDeSuPropiaVersionYNoConLasActuales(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()

	const versionVieja = 99
	faltante := clausulasDelSnapshot[len(clausulasDelSnapshot)-1]
	clausulasViejas := slices.Clone(clausulasDelSnapshot[:len(clausulasDelSnapshot)-1])

	// Registro temporal: cualquier prueba que corra despues no puede heredar
	// esta version fantasma.
	clausulasPorVersion[versionVieja] = clausulasViejas
	t.Cleanup(func() { delete(clausulasPorVersion, versionVieja) })

	juegoViejo := slices.DeleteFunc(juegoCompleto(), func(p parametroResuelto) bool {
		return p.clave == faltante.clave
	})
	pares := consumidos(juegoViejo, clausulasViejas)
	prefijoViejo := fmt.Sprintf("snp%d-", versionVieja)
	id := idDeSnapshot(pares, monedaBase, prefijoViejo)

	for _, p := range pares {
		if _, err := pool.Exec(ctx,
			`INSERT INTO snapshots_parametros
			   (snapshot_id, clave, valor, organo, reglamento, vigente_desde)
			 VALUES ($1, $2, $3, $4, $5, $6::date)`,
			id, p.clave, p.texto(), p.organo, p.reglamento, texto(p.vigenteDesde)); err != nil {
			t.Fatalf("plantar la fila %s bajo la version vieja: %v", p.clave, err)
		}
	}

	snap, err := store.SnapshotPorID(ctx, id)
	if err != nil {
		t.Fatalf("SnapshotPorID tenia que reconstruir la version %d sin pedir %s: %v",
			versionVieja, faltante.clave, err)
	}
	// Y el campo que la version vieja nunca cableo se queda en cero -es lo
	// correcto para ESA version-, no en un valor inventado.
	if !snap.AsignacionTercerosPct.IsZero() {
		t.Errorf("AsignacionTercerosPct = %s, la version %d nunca la cablea, tenia que quedarse en 0",
			snap.AsignacionTercerosPct, versionVieja)
	}
}

// El id es una suma de verificacion: unas filas que no hashean a el no son ese
// snapshot, y servirlas seria "reproducir" una corrida con cifras que nunca se
// pagaron.
func TestSnapshotPorIDRechazaFilasQueNoHasheanASuID(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()

	falso := prefijoSnapshot + strings.Repeat("0", 64)
	for _, p := range consumidos(juegoCompleto(), clausulasDelSnapshot) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO snapshots_parametros
			   (snapshot_id, clave, valor, organo, reglamento, vigente_desde)
			 VALUES ($1, $2, $3, $4, $5, $6::date)`,
			falso, p.clave, p.texto(), p.organo, p.reglamento, texto(p.vigenteDesde)); err != nil {
			t.Fatalf("plantar la fila %s: %v", p.clave, err)
		}
	}

	_, err := store.SnapshotPorID(ctx, falso)
	if !errors.Is(err, aplicacion.ErrSnapshotCorrupto) {
		t.Fatalf("se esperaba ErrSnapshotCorrupto, dio: %v", err)
	}
	// No se degrada a ErrNoEncontrado: "no existe" invita a volver a resolver
	// la fecha, que es justo lo que no se puede hacer.
	if errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Errorf("un snapshot corrupto no es un snapshot ausente: %v", err)
	}
}

// Un conjunto congelado al que le falta una clausula nunca fue un snapshot
// valido: la lectura falla en vez de servir algo que la escritura no habria
// producido.
func TestSnapshotPorIDRechazaUnConjuntoIncompleto(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	id, _, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31"))
	if err != nil {
		t.Fatalf("SnapshotEnFecha: %v", err)
	}

	// Por fuera del adaptador y saltandose el trigger de inmutabilidad, que es
	// la unica forma de llegar a este estado.
	if _, err := pool.Exec(ctx,
		`ALTER TABLE snapshots_parametros DISABLE TRIGGER snapshots_parametros_inmutables`); err != nil {
		t.Fatalf("desactivar el trigger: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM snapshots_parametros WHERE snapshot_id = $1 AND clave = 'ott.wb'`, id); err != nil {
		t.Fatalf("borrar la clausula: %v", err)
	}

	_, err = store.SnapshotPorID(ctx, id)
	if !errors.Is(err, aplicacion.ErrSnapshotCorrupto) {
		t.Fatalf("se esperaba ErrSnapshotCorrupto, dio: %v", err)
	}
	if !strings.Contains(err.Error(), "ott.wb") {
		t.Errorf("el error tiene que nombrar la clausula que falta, dio: %v", err)
	}
}

// Lo congelado es inmutable POR EL MOTOR, no por convenio: la disciplina de no
// reescribirlo no puede depender de que nadie escriba la sentencia.
func TestUnSnapshotCongeladoNoSePuedeReescribir(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	ctx := t.Context()

	id, _, err := store.SnapshotEnFecha(ctx, fecha(t, "2025-01-31"))
	if err != nil {
		t.Fatalf("SnapshotEnFecha: %v", err)
	}

	sentencias := map[string]string{
		"update":   `UPDATE snapshots_parametros SET valor = 0.99 WHERE snapshot_id = $1`,
		"delete":   `DELETE FROM snapshots_parametros WHERE snapshot_id = $1`,
		"truncate": `TRUNCATE snapshots_parametros`,
	}
	for nombre, sql := range sentencias {
		args := []any{id}
		if nombre == "truncate" {
			args = nil
		}
		if _, err := pool.Exec(ctx, sql, args...); err == nil {
			t.Errorf("%s sobre un snapshot congelado tiene que fallar", nombre)
		} else if !strings.Contains(err.Error(), "ADR 0005") {
			t.Errorf("%s: el error tiene que citar la ADR, dio: %v", nombre, err)
		}
	}
}

// La EXCLUDE de vigencias es la que hace que "el valor vigente en esta fecha"
// tenga UNA respuesta. Sin ella el adaptador tendria que desempatar, y el
// desempate seria una decision de codigo sobre una cifra normativa.
func TestLaBaseRechazaDosVigenciasSolapadasDeLaMismaClave(t *testing.T) {
	_, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")

	// El tramo abierto de 2024 sigue vivo; este pisa desde 2025.
	_, err := pool.Exec(t.Context(),
		`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
		 VALUES ('deduccion.administrativa', 0.21, DATE '2025-01-01', $1, $2)`,
		organoSintetic, reglamentoSintetico)
	if err == nil {
		t.Fatal("un solape de la misma clave tiene que rechazarse")
	}
	if !strings.Contains(err.Error(), "parametro_sin_solape") {
		t.Fatalf("lo tiene que rechazar la EXCLUDE parametro_sin_solape, dio: %v", err)
	}

	// Y dos claves distintas en el mismo rango NO se estorban: la restriccion
	// es por clave, no por fecha.
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
		 VALUES ('ott.wd', 0.10, DATE '2024-01-01', $1, $2)`,
		organoSintetic, reglamentoSintetico); err != nil {
		t.Fatalf("otra clave en el mismo rango tiene que caber: %v", err)
	}
}

func TestVigentesDevuelveLoQueRigeConSuProcedencia(t *testing.T) {
	store, pool := colaVacia(t)
	sembrarParametros(t, pool, "2024-01-01")
	cerrarVigencia(t, pool, "deduccion.administrativa", "2024-01-01", "2025-01-01")
	nuevaVigencia(t, pool, "deduccion.administrativa", "0.21", "2025-01-01", "")

	filas, err := store.Vigentes(t.Context(), fecha(t, "2025-06-30"))
	if err != nil {
		t.Fatalf("Vigentes: %v", err)
	}
	if len(filas) != len(juegoCompleto()) {
		t.Fatalf("Vigentes devolvio %d filas, se esperaban %d", len(filas), len(juegoCompleto()))
	}

	i := slices.IndexFunc(filas, func(f aplicacion.FilaParametro) bool {
		return f.Clave == "deduccion.administrativa"
	})
	if i < 0 {
		t.Fatal("falta deduccion.administrativa en la lista")
	}
	viva := filas[i]
	// El valor va en la forma canonica: la misma que se hashea, para que la
	// lista y lo congelado se puedan comparar caracter a caracter.
	if viva.Valor != "0.210000" {
		t.Errorf("Valor = %q, se esperaba %q", viva.Valor, "0.210000")
	}
	if viva.VigenteHasta != nil {
		t.Errorf("VigenteHasta = %v, el tramo abierto no tiene fin", viva.VigenteHasta)
	}
	if viva.OrganoAprobador == "" || viva.Reglamento == "" {
		t.Errorf("sin organo ni reglamento no es un parametro: %+v", viva)
	}
	if !viva.VigenteDesde.Equal(fecha(t, "2025-01-01")) {
		t.Errorf("VigenteDesde = %v, se esperaba 2025-01-01", viva.VigenteDesde)
	}

	// Un tramo cerrado si lo lleva, y esa es la mitad que dice hasta cuando
	// valio lo anterior.
	antes, err := store.Vigentes(t.Context(), fecha(t, "2024-06-30"))
	if err != nil {
		t.Fatalf("Vigentes en 2024: %v", err)
	}
	j := slices.IndexFunc(antes, func(f aplicacion.FilaParametro) bool {
		return f.Clave == "deduccion.administrativa"
	})
	if j < 0 || antes[j].VigenteHasta == nil {
		t.Fatalf("el tramo cerrado tiene que traer VigenteHasta: %+v", antes)
	}
}

// compararSnapshots exige igualdad campo a campo, no solo de las cifras que
// interesan hoy: un campo nuevo que nadie congele tiene que romper aqui.
func compararSnapshots(t *testing.T, quiero, tengo reparto.Snapshot) {
	t.Helper()

	decimales := map[string][2]decimal.Decimal{
		"AdminPct":             {quiero.AdminPct, tengo.AdminPct},
		"SocialPct":            {quiero.SocialPct, tengo.SocialPct},
		"ReservaPct":           {quiero.ReservaPct, tengo.ReservaPct},
		"PondCine":             {quiero.PondCine, tengo.PondCine},
		"PondUnitario":         {quiero.PondUnitario, tengo.PondUnitario},
		"PondSerie":            {quiero.PondSerie, tengo.PondSerie},
		"PondSketch":           {quiero.PondSketch, tengo.PondSketch},
		"Wa":                   {quiero.Wa, tengo.Wa},
		"Wb":                   {quiero.Wb, tengo.Wb},
		"Wc":                   {quiero.Wc, tengo.Wc},
		"UmbralMatch":          {quiero.UmbralMatch, tengo.UmbralMatch},
		"DuracionArtisticaPct": {quiero.DuracionArtisticaPct, tengo.DuracionArtisticaPct},
		"MinutosHoraTV":        {quiero.MinutosHoraTV, tengo.MinutosHoraTV},
	}
	for campo, par := range decimales {
		// String() y no Equal(): "byte-identicos" es mas fuerte que "iguales",
		// y es lo que hace que un reproceso de dentro de diez anos de la misma
		// cifra y no una equivalente.
		if par[0].String() != par[1].String() {
			t.Errorf("%s = %s, se esperaba %s", campo, par[1], par[0])
		}
	}
	if tengo.Reglamento != quiero.Reglamento {
		t.Errorf("Reglamento = %q, se esperaba %q", tengo.Reglamento, quiero.Reglamento)
	}
	if tengo.MonedaBase != quiero.MonedaBase {
		t.Errorf("MonedaBase = %q, se esperaba %q", tengo.MonedaBase, quiero.MonedaBase)
	}
	if len(tengo.Tasas) != len(quiero.Tasas) {
		t.Fatalf("Tasas = %v, se esperaban %v", tengo.Tasas, quiero.Tasas)
	}
	for iso, v := range quiero.Tasas {
		if tengo.Tasas[iso].String() != v.String() {
			t.Errorf("Tasas[%s] = %s, se esperaba %s", iso, tengo.Tasas[iso], v)
		}
	}
}
