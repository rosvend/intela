package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ aplicacion.ParametrosNormativos = (*Store)(nil)

const (
	// escalaParametro es la escala de `parametros.valor` y de
	// `snapshots_parametros.valor`: NUMERIC(18,6) en las dos.
	//
	// Esta escrita aqui porque es lo que fija la FORMA CANONICA de un valor, y
	// de esa forma cuelga el id del snapshot. Sin ella el texto que se hashea
	// dependeria de como el driver decida renderizar un numeric -- "0.2" o
	// "0.200000" son el mismo numero y dos hashes distintos --, y el ADR 0005
	// pide que una corrida se reproduzca bit a bit dentro de diez anos, no
	// mientras no cambie la version de pgx.
	//
	// Si algun dia la columna cambia de escala, esto cambia con ella y los ids
	// nuevos dejan de coincidir con los viejos. Eso es correcto: habria
	// cambiado la representacion del contenido.
	escalaParametro = 6

	// prefijoTasa es la familia de claves de cambio de moneda: `cambio.<ISO>`.
	// No hay lista de monedas en Go (ADR 0004, B4): solo se convierte lo que
	// tenga fila, y el euro sin fila propia no hereda la tasa del dolar.
	prefijoTasa = "cambio."

	// monedaBase es la moneda a la que estan expresadas las tasas `cambio.*`.
	//
	// Es un literal y no una fila de `parametros` por una razon de esquema y no
	// de diseno: la columna `valor` es NUMERIC y un codigo ISO no es un numero.
	// Mismo criterio -- y mismo valor -- que [Store.SnapshotNormalizacion]; el
	// dia que haga falta que sea dato, hace falta una columna de texto antes.
	monedaBase = "COP"
)

// columnasParametro es la parte comun de `parametros` y de
// `snapshots_parametros`: el valor y su procedencia.
//
// Las dos tablas la comparten porque las dos lecturas producen lo mismo -- un
// [parametroResuelto] -- y la de un snapshot congelado tiene que devolver
// exactamente lo que devolvio la resolucion que lo congelo.
const columnasParametro = `clave, valor, organo, reglamento, vigente_desde`

// escalaValor dice si `parametros.valor` ya esta en la unidad que
// [reparto.Snapshot] exige, o si hay que convertirlo al armar el snapshot.
//
// Existe porque el Snapshot NO usa una sola unidad (tipos.go:125-129):
// Admin/Social/Reserva, los grupos de canal y la asignacion a terceros son
// 0-100, pero ott.w*, ponderacion.* y duracion.artistica_pct son
// multiplicadores crudos que el motor usa tal cual. Que la unidad sea un
// campo explicito de la clausula, y no algo que cada setter tenga que
// recordar, es lo que evita que una clausula nueva se escriba en la unidad
// equivocada sin que nada lo note: bloqueante 1 de la revision de PR #134 fue
// exactamente eso, y en silencio.
type escalaValor int

const (
	// escalaDirecta: el valor de la columna es ya la unidad del Snapshot.
	escalaDirecta escalaValor = iota
	// escalaFraccionAPorcentaje: la columna trae una fraccion 0-1 ("0.20") y
	// el Snapshot exige 0-100. Solo deduccion.* y reserva.* vienen asi; el
	// sembrador ya sirve grupo.*_pct y asignacion.terceros_pct en 0-100.
	escalaFraccionAPorcentaje
)

// clausula es una clave de `parametros` con el hueco de [reparto.Snapshot] que
// llena.
type clausula struct {
	clave  string
	escala escalaValor
	en     func(*reparto.Snapshot, decimal.Decimal)
}

// clausulasDelSnapshot es el contrato entre la tabla y el tipo del dominio:
// las claves que un snapshot EXIGE, y donde va cada una.
//
// Que sea una lista y no un switch dentro del bucle de escaneo es lo que
// permite responder "cuales faltan" nombrandolas todas: un switch solo sabe
// que hacer con lo que llega, no que no llego.
//
// Ninguna tiene valor por defecto y ninguna es opcional (ADR 0004). Que las
// cifras de deduccion, reserva y OTT esten hoy sembradas como sinteticas no
// las hace prescindibles: las hace provisionales, que es otra cosa -- P-10
// sigue abierta y el dia que llegue el acta de la Asamblea se carga una fila,
// no se despliega.
//
// Las tasas `cambio.*` NO estan aqui: son una familia de tamano variable y
// entran por [prefijoTasa]. Pero entran en el id igual que estas, porque
// cambian el resultado igual que estas.
var clausulasDelSnapshot = []clausula{
	// R-06 (Ley 44/1993 Art. 21) y R-07 (RD 14.5.1). El sembrador las escribe
	// como fraccion 0-1 ("0.20"); el Snapshot las exige en 0-100
	// (tipos.go:125-129) porque asi las consume pctDe en el motor
	// (redondeo.go). escalaFraccionAPorcentaje es esa conversion, hecha aqui
	// -- donde tipos.go dice que tiene que pasar -- y no en el setter, donde
	// no protestaba ni exigirPositivo ni ninguna prueba (bloqueante 1, PR
	// #134).
	{"deduccion.administrativa", escalaFraccionAPorcentaje, func(s *reparto.Snapshot, v decimal.Decimal) { s.AdminPct = v }},
	{"deduccion.social", escalaFraccionAPorcentaje, func(s *reparto.Snapshot, v decimal.Decimal) { s.SocialPct = v }},
	{"reserva.errores_tecnicos", escalaFraccionAPorcentaje, func(s *reparto.Snapshot, v decimal.Decimal) { s.ReservaPct = v }},

	// Ponderacion por tipo de obra, RD 9.1.1. Multiplicadores crudos: no se
	// escalan.
	{"ponderacion.cinematografica", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.PondCine = v }},
	{"ponderacion.unitario", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.PondUnitario = v }},
	{"ponderacion.serie", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.PondSerie = v }},
	{"ponderacion.sketches", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.PondSketch = v }},

	// Coeficientes de la formula OTT, RD 9.7. Sin publicar (P-10). Tambien
	// multiplicadores crudos.
	{"ott.wa", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.Wa = v }},
	{"ott.wb", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.Wb = v }},
	{"ott.wc", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.Wc = v }},

	// Umbral de similitud de la cascada, ADR 0007. No es normativo, pero
	// cambia el resultado de una corrida y por eso entra en el snapshot: sin
	// congelarlo, recalibrarlo reidentificaria obras de un reparto cerrado.
	{"matching.umbral", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.UmbralMatch = v }},

	// RD 9.1.1(c): 80% artistico y hora televisiva de 48 minutos. Los aplica
	// normalizacion al canonizar la fila (duracion.go), multiplicando
	// directo -- no pasan por pctDe --, asi que van directas.
	{"duracion.artistica_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.DuracionArtisticaPct = v }},
	{"duracion.minutos_hora_tv", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.MinutosHoraTV = v }},

	// Porcentajes de grupo de canal (RD 9.5) y asignacion a plataformas de
	// terceros (RD 9.7). #126 anadio estos seis campos a Snapshot
	// (pctGrupo en estrategia.go, AsignacionTercerosPct en motor.go) sin que
	// este adaptador les diera clausula: se quedaban en cero sin que
	// ErrorParametroAusente saliera nunca, porque para el snapshot esas
	// claves sencillamente no existian (bloqueante 3, PR #134). El sembrador
	// ya las sirve en 0-100 -- la misma unidad que exige el motor --, asi que
	// van con escalaDirecta.
	{"grupo.privados_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.GrupoPrivadosPct = v }},
	{"grupo.regionales_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.GrupoRegionalesPct = v }},
	{"grupo.premium_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.GrupoPremiumPct = v }},
	{"grupo.lideres_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.GrupoLideresPct = v }},
	{"grupo.estandar_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.GrupoEstandarPct = v }},
	{"asignacion.terceros_pct", escalaDirecta, func(s *reparto.Snapshot, v decimal.Decimal) { s.AsignacionTercerosPct = v }},
}

// versionClausulasActual es la version del conjunto [clausulasDelSnapshot]
// que ESTE BINARIO congela. Vive en el prefijo de todo id nuevo (ver
// prefijoSnapshot) porque anadir o quitar una clausula es un cambio de
// FORMATO del snapshot, no solo de contenido -- el bloqueante 3 de PR #134
// (seis clausulas nuevas) es exactamente ese cambio, y paso "gratis" solo
// porque hoy no hay ningun snapshot ya congelado. La proxima vez que pase, no
// sera gratis sin esto. Ver ADR 0005, "La identidad del snapshot esta
// versionada".
const versionClausulasActual = 1

// prefijoSnapshot marca el id como lo que es y con que version del conjunto
// de clausulas se congelo: "snp1-", no "snp-". El resto son los 64 hex del
// sha256; el CHECK de `snapshots_parametros` (migracion 00012) exige la forma
// general `snp[0-9]+-[0-9a-f]{64}`, no una version fija, porque tiene que
// seguir aceptando ids mas viejos que dejen de ser "la version actual".
var prefijoSnapshot = fmt.Sprintf("snp%d-", versionClausulasActual)

// clausulasPorVersion es el registro de conjuntos de clausulas: uno por cada
// version que un id de snapshot puede nombrar en su prefijo.
//
// Politica de mantenimiento (ADR 0005): el dia que una clausula se anada, se
// quite o cambie de escala, [clausulasDelSnapshot] NO se edita in situ. Antes
// de tocarlo, el conjunto vigente HASTA ESE MOMENTO se copia a una constante
// nueva nombrada por su version (p.ej. `clausulasDelSnapshotV1` el dia que
// exista una V2) y esa copia se registra aqui bajo su numero. Solo entonces
// `clausulasDelSnapshot` pasa a apuntar al conjunto NUEVO y
// `versionClausulasActual` sube en uno. Hoy solo hay una version: la entrada
// de este mapa y la variable `clausulasDelSnapshot` son el mismo slice, y no
// hace falta el sufijo "V1" hasta que haya un V2 del que distinguirse. La
// entrada vieja, cuando exista, no se borra: se queda mientras dure la
// ventana de retencion de RD 13.2/13.4 (diez anos) o hasta que un cambio
// explicito -citando esta politica, no un descuido de refactor- decida
// retirarla.
//
// SnapshotEnFecha siempre congela bajo `versionClausulasActual`.
// SnapshotPorID nunca reconstruye contra "la version actual": reconstruye
// contra la entrada que el propio id nombra en su prefijo. Un id de una
// version que no esta en este mapa falla cerrado con un mensaje que nombra la
// version pedida y las que este binario conoce (ver
// snapshotDesdeTablaCongelada), en vez de reinterpretarse en silencio con el
// conjunto de clausulas equivocado.
var clausulasPorVersion = map[int][]clausula{
	versionClausulasActual: clausulasDelSnapshot,
}

// versionesConocidas devuelve las versiones registradas, ordenadas -- para
// que el mensaje de una version desconocida sea reproducible y no dependa del
// orden de iteracion de un map.
func versionesConocidas() []int {
	out := make([]int, 0, len(clausulasPorVersion))
	for v := range clausulasPorVersion {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}

// patronIDSnapshot es la forma GENERAL de un id de snapshot: `snp<version>-<hex64>`.
// Es el MISMO patron que el CHECK de `snapshots_parametros.snapshot_id`
// (migrations/00012): los dos admiten cualquier version, no solo la que este
// binario escribe hoy, porque una fila ya escrita no deja de ser valida
// cuando `versionClausulasActual` sube. Quien decide si la version es
// interpretable por este binario es clausulasPorVersion, no la forma.
var patronIDSnapshot = regexp.MustCompile(`^snp([0-9]+)-[0-9a-f]{64}$`)

// versionDeID extrae la version embebida en el prefijo de un id
// ("snp1-<hash>" -> 1, true). ok=false si el id no tiene ni la forma general
// de un snapshot -- eso tambien lo rechaza el CHECK de la tabla en cuanto se
// intenta escribir, pero un id que llega por PARAMETRO (SnapshotPorID) no
// pasa por ese CHECK hasta un INSERT que aqui todavia no se ha hecho.
func versionDeID(id string) (version int, ok bool) {
	m := patronIDSnapshot.FindStringSubmatch(id)
	if m == nil {
		return 0, false
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return v, true
}

// parametroResuelto es una fila de vigencia ya elegida para una fecha, con su
// valor en forma canonica.
type parametroResuelto struct {
	clave        string
	valor        decimal.Decimal
	organo       string
	reglamento   string
	vigenteDesde time.Time
}

// texto es la forma canonica del valor: la que se hashea y la que se congela.
func (p parametroResuelto) texto() string { return p.valor.StringFixed(escalaParametro) }

// SnapshotEnFecha resuelve cada clausula a la fila que rige en esa fecha y
// congela el conjunto. Devuelve el id direccionado por contenido y el snapshot.
//
// # Una sola respuesta por clave
//
// La EXCLUDE de vigencias (`parametro_sin_solape`, migracion 00001) garantiza
// que para una clave y una fecha hay como mucho UNA fila. Por eso aqui no hay
// desempate ni DISTINCT ON: si lo hubiera, estaria tapando que la restriccion
// no se cumple, y el ADR 0005 necesita que la pregunta tenga una sola
// respuesta, no que el codigo elija una.
//
// # La comparacion es de fecha, no de instante
//
// `vigente_desde` y `vigente_hasta` son DATE. Comparar un timestamptz con una
// DATE deja que PostgreSQL convierta usando la zona horaria de la SESION, que
// depende del servidor: la misma fecha de periodo resolveria a una vigencia
// distinta en dos despliegues, y el id del snapshot cambiaria con ella. El
// instante se reduce aqui a fecha en UTC y se compara fecha con fecha. Es el
// mismo criterio que [Store.Pendientes].
//
// Como efecto util, dos llamadas con instantes distintos del MISMO dia
// resuelven identicas y devuelven el mismo id.
//
// # Por que resolver y congelar van juntos
//
// enTransaccionDe y no EnTransaccion: congelar el corte es una escritura de
// quien esta abriendo algo mas, no un hecho suelto. Si el caso de uso abrio
// una unidad ([aplicacion.UnidadDeTrabajo]), esta escritura entra EN ELLA y la
// confirma quien la abrio; con EnTransaccion el snapshot se confirmaria aqui y
// un fallo posterior dejaria un corte congelado que ninguna corrida
// referencia.
//
// Hoy nadie escribe todavia `procesos.snapshot_id` -- la columna existe desde
// la 00001, pero el repositorio de procesos esta declarado sin adaptador y
// [aplicacion.Parametros] no esta cableado en cmd/api --, asi que el unico
// consumidor de la unidad es quien llame a este metodo. La decision no depende
// de esa escritura: lo que la justifica es que el corte no puede confirmarse
// por su cuenta.
func (s *Store) SnapshotEnFecha(ctx context.Context, fechaPeriodo time.Time) (string, reparto.Snapshot, error) {
	dia := enDia(fechaPeriodo)
	diaTexto := texto(dia)

	var (
		id   string
		snap reparto.Snapshot
	)
	err := s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		filas, err := leerParametros(ctx, tx,
			`SELECT `+columnasParametro+`
			   FROM parametros
			  WHERE vigente_desde <= $1::date
			    AND (vigente_hasta IS NULL OR vigente_hasta > $1::date)`,
			fmt.Sprintf("parametros vigentes en %s", diaTexto), diaTexto)
		if err != nil {
			return err
		}

		pares := consumidos(filas, clausulasDelSnapshot)
		resuelto, _, faltan, err := armarSnapshot(pares, clausulasDelSnapshot, prefijoSnapshot)
		if err != nil {
			return err
		}
		if len(faltan) > 0 {
			return &aplicacion.ErrorParametroAusente{Fecha: dia, Claves: faltan}
		}
		if err := congelar(ctx, tx, resuelto, pares); err != nil {
			return err
		}

		// Se relee lo que QUEDO grabado bajo `resuelto` en vez de devolver
		// `armado` -el snapshot de esta resolucion-. Normalmente son el mismo
		// contenido, pero si el id ya existia con otra procedencia -misma
		// cifra, ratificada por otra Asamblea en otra fecha- ON CONFLICT DO
		// NOTHING (en congelar) dejo la procedencia VIEJA en la tabla, y
		// devolver `armado` aqui haria que esta llamada reportara un
		// Reglamento que SnapshotPorID(resuelto) jamas volveria a dar para el
		// mismo id. Releer dentro de la misma transaccion es lo que hace que
		// "lo que se resolvio" y "lo que se puede releer despues" sean
		// siempre la misma respuesta (ADR 0005; bloqueante 6, PR #134).
		congelado, err := snapshotDesdeTablaCongelada(ctx, tx, resuelto)
		if err != nil {
			return err
		}
		id, snap = resuelto, congelado
		return nil
	})
	if err != nil {
		return "", reparto.Snapshot{}, err
	}
	return id, snap, nil
}

// SnapshotPorID recupera un snapshot ya congelado.
func (s *Store) SnapshotPorID(ctx context.Context, id string) (reparto.Snapshot, error) {
	return snapshotDesdeTablaCongelada(ctx, s.ejecutorDe(ctx), id)
}

// snapshotDesdeTablaCongelada relee `snapshots_parametros` bajo `id` y
// reconstruye el snapshot por el MISMO camino que la resolucion --
// [consumidos] y [armarSnapshot] sobre las filas guardadas -- por la misma
// razon que [fila.entidad] reconstruye una obra con el constructor del
// dominio: si lo guardado no forma un snapshot valido, la lectura FALLA en
// vez de servir algo que la escritura no habria producido.
//
// Y recalcula el id. Estando direccionado por contenido, el id ES la suma de
// verificacion de las filas: comprobarlo cuesta un hash y convierte "estas
// filas son las que se congelaron" en algo demostrado y no supuesto. La
// alternativa -- confiar en la clave primaria -- deja pasar una fila alterada
// por fuera del adaptador, y el resultado seria una corrida "reproducida" con
// cifras que nunca se pagaron.
//
// La usan SnapshotPorID -un snapshot de hace anos- y SnapshotEnFecha justo
// despues de congelar -para devolver la procedencia que realmente quedo
// grabada-. Una sola definicion de "que es releer un snapshot" para las dos.
//
// # La version se valida ANTES de tocar la tabla
//
// El prefijo del id nombra la version del conjunto de clausulas con que se
// congelo (ver [versionClausulasActual] y [clausulasPorVersion]). Reconstruir
// contra "la version actual" en vez de contra la version que el id nombra es
// exactamente el fallo de reproducibilidad que N1-b de la revision de PR #134
// senalo: un id viejo, bajo un conjunto de clausulas mas nuevo (con MAS
// campos), no "le faltan clausulas" -que es corregible, cargar la fila que
// falta-; es que este binario esta usando la regla EQUIVOCADA para
// interpretarlo. Fallar antes de leer una sola fila, nombrando la version
// pedida y las que este binario conoce, es lo que distingue "version que no
// conozco" de "estas filas estan corruptas" -- son dos causas distintas y el
// ADR 0005 pide que la respuesta sea honesta sobre cual es.
func snapshotDesdeTablaCongelada(ctx context.Context, ej ejecutor, id string) (reparto.Snapshot, error) {
	version, ok := versionDeID(id)
	if !ok {
		return reparto.Snapshot{}, fmt.Errorf("snapshot %q: %w: no tiene la forma snp<version>-<hex64>",
			id, aplicacion.ErrSnapshotCorrupto)
	}
	clausulas, conocida := clausulasPorVersion[version]
	if !conocida {
		return reparto.Snapshot{}, fmt.Errorf(
			"snapshot %q: %w: version %d desconocida, este binario reconstruye las versiones %v",
			id, aplicacion.ErrSnapshotCorrupto, version, versionesConocidas())
	}
	prefijo := fmt.Sprintf("snp%d-", version)

	filas, err := leerParametros(ctx, ej,
		`SELECT `+columnasParametro+` FROM snapshots_parametros WHERE snapshot_id = $1`,
		fmt.Sprintf("snapshot %q", id), id)
	if err != nil {
		return reparto.Snapshot{}, err
	}
	// Cero filas no es un escaneo fallido: Rows.Err() es nil con cero filas
	// (ver doc.go). "Ese snapshot no se congelo nunca" hay que decirlo aqui.
	if len(filas) == 0 {
		return reparto.Snapshot{}, fmt.Errorf("snapshot %q: %w", id, aplicacion.ErrNoEncontrado)
	}

	pares := consumidos(filas, clausulas)
	recalculado, snap, faltan, err := armarSnapshot(pares, clausulas, prefijo)
	if err != nil {
		// Los dos %w, no uno con %s: un conjunto congelado que resulta
		// ambiguo (dos `cambio.*` que normalizan al mismo ISO, coladas antes
		// de que existiera esta comprobacion) SI esta corrupto -ErrSnapshotCorrupto
		// sigue siendo la causa de fondo-, pero quien relee tambien tiene que
		// poder distinguir *por que* con errors.As(err, &ErrorTasaAmbigua{}),
		// no solo enterarse de que algo esta mal.
		return reparto.Snapshot{}, fmt.Errorf("snapshot %q: %w: %w", id, aplicacion.ErrSnapshotCorrupto, err)
	}
	if len(faltan) > 0 {
		return reparto.Snapshot{}, fmt.Errorf("snapshot %q: %w: le faltan clausulas (%s)",
			id, aplicacion.ErrSnapshotCorrupto, strings.Join(faltan, ", "))
	}
	if recalculado != id {
		return reparto.Snapshot{}, fmt.Errorf("snapshot %q: %w: sus valores hashean a %q",
			id, aplicacion.ErrSnapshotCorrupto, recalculado)
	}
	return snap, nil
}

// Vigentes lista los parametros que rigen en `ahora`, con su procedencia.
//
// Es la lectura de administracion del ADR 0004, no la del motor: por eso
// devuelve [aplicacion.FilaParametro] con la vigencia y el organo, y no un
// snapshot. El valor va en la misma forma canonica que entra en el id, para
// que la lista y lo congelado se puedan comparar caracter a caracter.
//
// ORDER BY aqui es presentacion y puede ir en SQL; el orden del que cuelga el
// id NO, y por eso lo fija [consumidos] en Go. Ver su comentario.
func (s *Store) Vigentes(ctx context.Context, ahora time.Time) ([]aplicacion.FilaParametro, error) {
	dia := texto(enDia(ahora))
	contexto := fmt.Sprintf("parametros vigentes en %s", dia)

	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT clave, valor, vigente_desde, vigente_hasta, organo, reglamento
		   FROM parametros
		  WHERE vigente_desde <= $1::date
		    AND (vigente_hasta IS NULL OR vigente_hasta > $1::date)
		  ORDER BY clave`, dia)
	if err != nil {
		return nil, traducirError(err, "%s", contexto)
	}
	defer filas.Close()

	var out []aplicacion.FilaParametro
	for filas.Next() {
		var (
			f     aplicacion.FilaParametro
			valor decimal.Decimal
		)
		if err := filas.Scan(&f.Clave, &valor, &f.VigenteDesde, &f.VigenteHasta,
			&f.OrganoAprobador, &f.Reglamento); err != nil {
			return nil, traducirError(err, "escanear parametro vigente")
		}
		f.Valor = valor.StringFixed(escalaParametro)
		out = append(out, f)
	}
	// No es opcional: un fallo a mitad de stream sale solo por aqui, y sin
	// esta comprobacion una lista TRUNCADA se devuelve como lista completa.
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "%s", contexto)
	}
	return out, nil
}

// leerParametros escanea las columnas de [columnasParametro]. Sirve a las dos
// tablas porque las dos producen lo mismo.
func leerParametros(ctx context.Context, ej ejecutor, sql, contexto string, args ...any) ([]parametroResuelto, error) {
	filas, err := ej.Query(ctx, sql, args...)
	if err != nil {
		return nil, traducirError(err, "%s", contexto)
	}
	defer filas.Close()

	var out []parametroResuelto
	for filas.Next() {
		var p parametroResuelto
		if err := filas.Scan(&p.clave, &p.valor, &p.organo, &p.reglamento, &p.vigenteDesde); err != nil {
			return nil, traducirError(err, "escanear parametro")
		}
		out = append(out, p)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "%s", contexto)
	}
	return out, nil
}

// congelar escribe el corte bajo su id.
//
// ON CONFLICT DO NOTHING y no un error: el id esta direccionado por contenido,
// asi que una segunda escritura bajo el mismo id lleva exactamente los mismos
// valores. Reintentar la apertura de un proceso no puede fallar por haberla
// intentado antes. Es lo contrario de lo que hace `asientos`, y por el motivo
// opuesto: alli un id determinista convertia un libro append-only en uno que
// descarta appends, aqui un snapshot repetido ES el mismo snapshot.
//
// Los valores viajan como texto y se convierten en el SELECT en vez de ir como
// []decimal.Decimal: el texto es la forma canonica que ya se hasheo, asi que
// lo que se guarda es exactamente lo que el id promete.
func congelar(ctx context.Context, tx pgx.Tx, id string, pares []parametroResuelto) error {
	claves := make([]string, len(pares))
	valores := make([]string, len(pares))
	organos := make([]string, len(pares))
	reglamentos := make([]string, len(pares))
	desdes := make([]string, len(pares))
	for i, p := range pares {
		claves[i] = p.clave
		valores[i] = p.texto()
		organos[i] = p.organo
		reglamentos[i] = p.reglamento
		desdes[i] = texto(p.vigenteDesde)
	}

	tag, err := tx.Exec(ctx,
		`INSERT INTO snapshots_parametros (snapshot_id, `+columnasParametro+`)
		 SELECT $1, u.clave, u.valor::numeric, u.organo, u.reglamento, u.vigente_desde::date
		   FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[])
		     AS u(clave, valor, organo, reglamento, vigente_desde)
		 ON CONFLICT (snapshot_id, clave) DO NOTHING`,
		id, claves, valores, organos, reglamentos, desdes)
	if err != nil {
		return traducirError(err, "congelar el snapshot %q", id)
	}

	// RowsAffected descartado seria indistinguible de un exito: n==0 es "ya
	// estaba TODO" (la carrera que ON CONFLICT DO NOTHING existe para
	// resolver) y n==len(pares) es "no habia nada". Cualquier otro numero es
	// una escritura A MEDIAS -una corrida anterior se corto entre el primer y
	// el ultimo INSERT de este mismo id- y hay que gritarlo ahora: leer ese
	// id despues con menos filas de las que promete el hash da
	// ErrSnapshotCorrupto sin decir por que se corrompio (bloqueante 6, PR
	// #134).
	if n, total := tag.RowsAffected(), int64(len(pares)); n != 0 && n != total {
		return fmt.Errorf("congelar el snapshot %q: escritura parcial (%d de %d filas)", id, n, total)
	}
	return nil
}

// consumidos deja las filas que el snapshot USA, en forma canonica y ordenadas
// por clave.
//
// # Por que se filtra
//
// El id direcciona el CONTENIDO del snapshot. Una fila de `parametros` que el
// snapshot no lee -- un parametro de otro modulo, uno que anada un issue
// futuro -- no cambia el snapshot, asi que no puede cambiar su id: si entrara
// en el hash, cargar un parametro ajeno haria que la misma fecha resolviera a
// otro id y romperia la reproducibilidad sin cambiar ni una cifra.
//
// # Por que se ordena en Go y no en SQL
//
// ORDER BY ordena por la COLLATION de la base, que depende de como se creo:
// con una collation lingüistica, `cambio.USD` y `cambio.usd` se ordenan al
// reves que en bytes. El id quedaria atado a la configuracion regional del
// servidor, y el mismo conjunto de valores daria dos ids distintos en dos
// despliegues. strings.Compare es orden de bytes y no depende de nada.
//
// # Por que se canoniza el valor
//
// Round a la escala de la columna fija la representacion antes de hashearla, y
// devuelve el mismo decimal por los dos caminos -- resolver y leer por id --,
// que es lo que hace que los valores sean identicos y no solo iguales.
// clausulas es EXPLICITO -- nunca el global [clausulasDelSnapshot] -- porque
// esta funcion sirve tanto a una resolucion fresca (siempre contra la version
// actual) como a la reconstruccion de un id viejo (contra la version que ESE
// id nombra). Usar el global aqui haria que reconstruir una version antigua
// filtrara con las clausulas de la version de hoy, que es precisamente el
// fallo de identidad que N1-b de la revision de PR #134 señalo.
func consumidos(filas []parametroResuelto, clausulas []clausula) []parametroResuelto {
	out := make([]parametroResuelto, 0, len(filas))
	for _, p := range filas {
		if !loConsume(p.clave, clausulas) {
			continue
		}
		p.valor = p.valor.Round(escalaParametro)
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b parametroResuelto) int { return strings.Compare(a.clave, b.clave) })
	return out
}

// loConsume dice si esa clave entra en el snapshot bajo el conjunto
// `clausulas` dado: o es una de las clausulas exigidas, o es una tasa
// `cambio.<ISO>` con codigo -- las tasas no estan versionadas, son una
// familia de tamano variable en cualquier version.
//
// `cambio.` a secas no es una tasa de nada y se descarta: con el prefijo vacio
// acabaria en Tasas[""] y convertiria el importe sin moneda de cualquier fila.
func loConsume(clave string, clausulas []clausula) bool {
	if slices.ContainsFunc(clausulas, func(c clausula) bool { return c.clave == clave }) {
		return true
	}
	codigo, esTasa := strings.CutPrefix(clave, prefijoTasa)
	return esTasa && codigo != ""
}

// armarSnapshot construye el snapshot y su id a partir de los pares que
// [consumidos] dejo. Devuelve las clausulas que falten, si falta alguna.
//
// Es una funcion pura sobre los pares y no un metodo de Store: asi la
// resolucion de vigencias se prueba con tabla y sin contenedor, y la misma
// funcion sirve para resolver contra `parametros` y para reconstruir desde
// `snapshots_parametros`. Dos caminos con una sola definicion de que es un
// snapshot.
//
// Devuelve las claves que faltan en vez del error ya formado porque los dos
// sitios de llamada las cuentan distinto: resolver una fecha sin ellas es
// [aplicacion.ErrorParametroAusente] -- hay filas que cargar --, y un conjunto
// CONGELADO al que le falta una es ErrSnapshotCorrupto, porque nunca fue un
// snapshot valido y no hay nada que cargar.
//
// El error de retorno es distinto de "faltan": es que DOS claves entran en
// conflicto entre si (hoy, dos tasas `cambio.*` que normalizan al mismo
// codigo ISO). No es "falta cargar algo" ni "el conjunto esta corrupto", asi
// que no cabe en `faltan` sin que un `errors.Is(err, ErrParametroAusente)`
// aguas abajo lo confunda con lo otro.
//
// `clausulas` y `prefijo` son explicitos por la misma razon que en
// [consumidos]: armar una resolucion fresca siempre usa la version actual
// ([clausulasDelSnapshot], [prefijoSnapshot]), pero reconstruir un id viejo
// tiene que usar el conjunto y el prefijo que ESE id nombra -- ver
// [snapshotDesdeTablaCongelada] y la seccion de ADR 0005 sobre el versionado
// de la identidad.
func armarSnapshot(pares []parametroResuelto, clausulas []clausula, prefijo string) (string, reparto.Snapshot, []string, error) {
	porClave := make(map[string]parametroResuelto, len(pares))
	for _, p := range pares {
		porClave[p.clave] = p
	}

	snap := reparto.Snapshot{
		MonedaBase: monedaBase,
		Tasas:      map[string]decimal.Decimal{},
	}
	var faltan []string
	for _, c := range clausulas {
		p, hay := porClave[c.clave]
		if !hay {
			faltan = append(faltan, c.clave)
			continue
		}
		v := p.valor
		if c.escala == escalaFraccionAPorcentaje {
			v = v.Mul(decimal.NewFromInt(100))
		}
		c.en(&snap, v)
	}
	// Se devuelven TODAS las que falten, no la primera: enterarse de una por
	// intento son tantos viajes como parametros sin cargar.
	if len(faltan) > 0 {
		return "", reparto.Snapshot{}, faltan, nil
	}

	var reglamentos []string
	// isoDeClave recuerda, por codigo ISO ya normalizado, cual clave original
	// lo fijo primero. pares llega ordenado por [consumidos], asi que el
	// resultado no depende de en que orden entraron las filas.
	isoDeClave := make(map[string]string, len(pares))
	for _, p := range pares {
		if codigo, esTasa := strings.CutPrefix(p.clave, prefijoTasa); esTasa {
			iso := strings.ToUpper(codigo)
			// cambio.USD y cambio.usd son DOS filas de `parametros` -el
			// esquema no las distingue de dos monedas legitimas- que colapsan
			// a la misma Tasas["USD"]. Sin esta comprobacion la que ordena
			// despues por bytes gana en silencio y la otra desaparece sin
			// error (bloqueante 6, PR #134): un factor de conversion que
			// alguien cargo de verdad se pierde sin que nada lo diga.
			if otra, ya := isoDeClave[iso]; ya {
				return "", reparto.Snapshot{}, nil, &aplicacion.ErrorTasaAmbigua{
					Codigo: iso,
					Claves: []string{otra, p.clave},
				}
			}
			isoDeClave[iso] = p.clave
			snap.Tasas[iso] = p.valor
		}
		if !slices.Contains(reglamentos, p.reglamento) {
			reglamentos = append(reglamentos, p.reglamento)
		}
	}
	// El reglamento del snapshot es el conjunto de los que intervinieron, no
	// uno elegido entre ellos: las ponderaciones salen de RD 9.1.1 y los
	// coeficientes OTT de otro sitio, y un solo nombre en `Resultado.Reglamento`
	// diria que la cifra se calculo con reglas que no son todas las que se
	// usaron. Ordenados para que la cadena no dependa del orden de llegada.
	slices.Sort(reglamentos)
	snap.Reglamento = strings.Join(reglamentos, "+")

	return idDeSnapshot(pares, monedaBase, prefijo), snap, nil, nil
}

// claveMetaMonedaBase es la clave RESERVADA bajo la que [idDeSnapshot] mete la
// moneda base en el digest. Nunca puede llegar por fila real: empieza por
// "_", y el CHECK de `parametros.clave` y `snapshots_parametros.clave`
// (migracion 00012) exige que el primer caracter sea una letra.
const claveMetaMonedaBase = "_meta.moneda_base"

// idDeSnapshot es el sha256 de los pares (clave, valor) canonicos, uno por
// linea, mas la moneda base a la que estan expresadas las tasas `cambio.*`.
//
// De `pares` solo entran clave y valor. La procedencia -- organo, reglamento,
// vigencia -- viaja congelada con cada fila y se lee con ella, pero NO entra
// en el id: corregir una errata en el nombre del organo no cambia ni una
// cifra del reparto, y si entrara, cambiaria el id y dejaria huerfana la
// corrida que lo referencia.
//
// monedaBase si entra, aunque hoy sea una constante de Go y no una fila
// (ver la constante del mismo nombre): [reparto.Snapshot.MonedaBase] es una
// cifra que el motor consume tanto como cualquier tasa, y dejarla fuera del
// digest significaria que cambiarla reinterpretaria en silencio TODOS los
// snapshots ya congelados -mismo id, tasas que de repente se leen contra otra
// moneda-. Con ella dentro, cambiar la moneda base mueve los ids nuevos y dejaria
// releer uno viejo con la constante nueva devolviendo ErrSnapshotCorrupto en vez
// de una reinterpretacion silenciosa, que es justo lo que el ADR 0005 exige.
//
// `prefijo` es explicito -- nunca el global [prefijoSnapshot] -- para que esta
// funcion pueda recalcular el id de una version VIEJA con SU prefijo (ver
// [armarSnapshot]) sin que el resultado se compare contra el prefijo de la
// version de hoy.
func idDeSnapshot(pares []parametroResuelto, monedaBase, prefijo string) string {
	var b strings.Builder
	for _, p := range pares {
		fmt.Fprintf(&b, "%s=%s\n", p.clave, p.texto())
	}
	fmt.Fprintf(&b, "%s=%s\n", claveMetaMonedaBase, monedaBase)
	suma := sha256.Sum256([]byte(b.String()))
	return prefijo + hex.EncodeToString(suma[:])
}

// enDia reduce un instante a su fecha en UTC. Ver el comentario de
// [Store.SnapshotEnFecha] sobre por que la comparacion no puede ser de
// instante.
func enDia(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// texto es la forma en que una fecha viaja a la base y vuelve: `YYYY-MM-DD`,
// sin zona. Una DATE no tiene hora que convertir.
func texto(t time.Time) string { return t.UTC().Format(time.DateOnly) }

// ---------------------------------------------------------------------------
// Un parametro suelto en una fecha (#32)

var _ aplicacion.ParametroEnFecha = (*Store)(nil)

// ErrParametroSinVigencia: ninguna fila de la clave cubre la fecha. Propio y no
// ErrNoEncontrado porque lo util es QUE clave falta y para cuando.
var ErrParametroSinVigencia = errors.New("parametro sin vigencia en la fecha")

// ParametroVigente resuelve el valor de una clave en una fecha.
//
// La restriccion EXCLUDE de `parametros` (`parametro_sin_solape`, 00001_init.sql)
// garantiza que dos filas de la misma clave no se solapen en el tiempo, asi que
// esta consulta devuelve como mucho una fila. Sin esa garantia habria que decidir
// aqui cual de dos gana, y la decision dependeria del orden de lectura -- es
// decir, la misma corrida repetida podria repartir distinto.
//
// El rango es [vigente_desde, vigente_hasta): medio abierto, igual que el
// daterange '[)' de la restriccion. Cerrarlo por arriba haria que el ultimo dia
// de una vigencia tuviera dos valores validos, que es exactamente lo que el
// EXCLUDE impide crear.
//
// Un parametro ausente NO cae a cero. ADR 0004 modela lo ausente como ausente,
// y en este caso concreto un umbral de matching en cero asignaria la primera
// obra que se pareciera en algo a cualquier titulo.
//
// La comparacion es de FECHA y no de instante, con el mismo criterio que
// [Store.SnapshotEnFecha]: comparar un timestamptz con una DATE dejaria que la
// zona de la sesion decidiera la vigencia.
//
// No congela nada, a diferencia de SnapshotEnFecha: identificar no mueve dinero
// (ADR 0003). `matching.umbral` TAMBIEN entra en el snapshot del reparto; los
// dos leen la misma fila porque los dos resuelven contra la fecha del periodo y
// la EXCLUDE deja una sola respuesta. `matching.umbral_banda` no entra en el
// snapshot: no asigna obras, solo decide que se muestra en la bandeja (#39).
func (s *Store) ParametroVigente(ctx context.Context, clave string, fecha time.Time) (decimal.Decimal, error) {
	dia := texto(enDia(fecha))

	var valor decimal.Decimal
	err := s.ejecutorDe(ctx).QueryRow(ctx, `
		SELECT valor FROM parametros
		 WHERE clave = $1
		   AND vigente_desde <= $2::date
		   AND (vigente_hasta IS NULL OR vigente_hasta > $2::date)`,
		clave, dia).Scan(&valor)

	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, fmt.Errorf("%q en %s: %w", clave, dia, ErrParametroSinVigencia)
	}
	if err != nil {
		return decimal.Zero, traducirError(err, "leer el parametro %q", clave)
	}
	return valor, nil
}
