package postgres

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
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

// juegoCompleto son las trece clausulas que [clausulasDelSnapshot] exige, mas
// las dos tasas del sembrador.
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
	id, snap, faltan := armarSnapshot(consumidos(juegoCompleto()))
	if len(faltan) > 0 {
		t.Fatalf("con el juego completo no puede faltar nada, faltaron %v", faltan)
	}
	if !strings.HasPrefix(id, prefijoSnapshot) || len(id) != len(prefijoSnapshot)+64 {
		t.Fatalf("id = %q, se esperaba %s + 64 hex", id, prefijoSnapshot)
	}

	esperado := map[string]decimal.Decimal{
		"AdminPct":             snap.AdminPct,
		"SocialPct":            snap.SocialPct,
		"ReservaPct":           snap.ReservaPct,
		"PondCine":             snap.PondCine,
		"PondUnitario":         snap.PondUnitario,
		"PondSerie":            snap.PondSerie,
		"PondSketch":           snap.PondSketch,
		"Wa":                   snap.Wa,
		"Wb":                   snap.Wb,
		"Wc":                   snap.Wc,
		"UmbralMatch":          snap.UmbralMatch,
		"DuracionArtisticaPct": snap.DuracionArtisticaPct,
		"MinutosHoraTV":        snap.MinutosHoraTV,
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
	if !snap.AdminPct.Equal(decimal.RequireFromString("0.20")) {
		t.Errorf("AdminPct = %s, se esperaba 0.20", snap.AdminPct)
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
			id, snap, faltan := armarSnapshot(consumidos(sin(c.clave)))
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

	_, _, faltan := armarSnapshot(consumidos(juego))
	// El orden es el de clausulasDelSnapshot, que es estable entre corridas.
	if quiero := []string{"deduccion.social", "ott.wb"}; !slices.Equal(faltan, quiero) {
		t.Fatalf("faltan = %v, se esperaba %v", faltan, quiero)
	}
}

// Criterio 4, en su forma pura: el id sale del contenido y no del orden de
// llegada ni de la collation de nadie.
func TestElIDNoDependeDelOrdenDeLlegada(t *testing.T) {
	derecho, _, _ := armarSnapshot(consumidos(juegoCompleto()))

	revuelto := juegoCompleto()
	slices.Reverse(revuelto)
	alReves, _, _ := armarSnapshot(consumidos(revuelto))

	if derecho != alReves {
		t.Fatalf("el id cambio con el orden de entrada: %q vs %q", derecho, alReves)
	}
}

// Un valor distinto es OTRO snapshot. Si no, dos repartos calculados con
// deducciones distintas compartirian procedencia.
func TestElIDCambiaSiCambiaUnValor(t *testing.T) {
	antes, _, _ := armarSnapshot(consumidos(juegoCompleto()))

	juego := juegoCompleto()
	for i := range juego {
		if juego[i].clave == "deduccion.administrativa" {
			juego[i].valor = decimal.RequireFromString("0.19")
		}
	}
	despues, _, _ := armarSnapshot(consumidos(juego))

	if antes == despues {
		t.Fatal("cambiar la deduccion administrativa tiene que cambiar el id")
	}
}

// Una tasa cuenta tanto como una ponderacion: cambia la cifra final.
func TestElIDCambiaSiCambiaUnaTasa(t *testing.T) {
	antes, _, _ := armarSnapshot(consumidos(juegoCompleto()))

	juego := juegoCompleto()
	for i := range juego {
		if juego[i].clave == "cambio.USD" {
			juego[i].valor = decimal.RequireFromString("4100")
		}
	}
	despues, _, _ := armarSnapshot(consumidos(juego))

	if antes == despues {
		t.Fatal("cambiar cambio.USD tiene que cambiar el id")
	}
}

// Lo que el snapshot no consume no puede cambiar su id: si entrara en el hash,
// cargar un parametro de otro modulo haria que la misma fecha resolviera a
// otro id sin que ninguna cifra del reparto cambiara.
func TestUnParametroAjenoNoCambiaElID(t *testing.T) {
	solo, _, _ := armarSnapshot(consumidos(juegoCompleto()))

	conAjeno := append(juegoCompleto(),
		unParametro("calendario.corte_rendimientos", "20", organoConsejo, "RD 10.1"))
	acompanado, snap, faltan := armarSnapshot(consumidos(conAjeno))

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
	antes, _, _ := armarSnapshot(consumidos(juegoCompleto()))

	juego := juegoCompleto()
	for i := range juego {
		juego[i].organo = "Consejo Directivo (acta 12)"
	}
	despues, _, _ := armarSnapshot(consumidos(juego))

	if antes != despues {
		t.Fatalf("corregir el organo cambio el id: %q vs %q", antes, despues)
	}
}

// La escala de la columna es la que fija la forma canonica: 0.2 y 0.200000 son
// el mismo valor normativo y tienen que dar el mismo id.
func TestElIDNoDependeDeLaEscalaConQueLlegueElValor(t *testing.T) {
	escueto, _, _ := armarSnapshot(consumidos(juegoCompleto()))

	juego := juegoCompleto()
	for i := range juego {
		if juego[i].clave == "deduccion.administrativa" {
			juego[i].valor = decimal.RequireFromString("0.2000000")
		}
	}
	rellenado, _, _ := armarSnapshot(consumidos(juego))

	if escueto != rellenado {
		t.Fatalf("la escala de entrada cambio el id: %q vs %q", escueto, rellenado)
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

	casos := []struct {
		dia    string
		quiero string
	}{
		{"2024-06-30", "0.2"},
		{"2025-06-30", "0.21"},
		{"2026-06-30", "0.22"},
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
		{"2024-12-31", "0.2"},
		{"2025-01-01", "0.21"},
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
	if !nuevo.AdminPct.Equal(decimal.RequireFromString("0.15")) {
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

// El id es una suma de verificacion: unas filas que no hashean a el no son ese
// snapshot, y servirlas seria "reproducir" una corrida con cifras que nunca se
// pagaron.
func TestSnapshotPorIDRechazaFilasQueNoHasheanASuID(t *testing.T) {
	store, pool := colaVacia(t)
	ctx := t.Context()

	falso := prefijoSnapshot + strings.Repeat("0", 64)
	for _, p := range consumidos(juegoCompleto()) {
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
