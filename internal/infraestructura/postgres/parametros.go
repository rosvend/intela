package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
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

	// prefijoSnapshot marca el id como lo que es. El resto son los 64 hex del
	// sha256; el CHECK de la tabla exige exactamente esta forma.
	prefijoSnapshot = "snp-"

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

// clausula es una clave de `parametros` con el hueco de [reparto.Snapshot] que
// llena.
type clausula struct {
	clave string
	en    func(*reparto.Snapshot, decimal.Decimal)
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
	// R-06 (Ley 44/1993 Art. 21) y R-07 (RD 14.5.1).
	{"deduccion.administrativa", func(s *reparto.Snapshot, v decimal.Decimal) { s.AdminPct = v }},
	{"deduccion.social", func(s *reparto.Snapshot, v decimal.Decimal) { s.SocialPct = v }},
	{"reserva.errores_tecnicos", func(s *reparto.Snapshot, v decimal.Decimal) { s.ReservaPct = v }},

	// Ponderacion por tipo de obra, RD 9.1.1.
	{"ponderacion.cinematografica", func(s *reparto.Snapshot, v decimal.Decimal) { s.PondCine = v }},
	{"ponderacion.unitario", func(s *reparto.Snapshot, v decimal.Decimal) { s.PondUnitario = v }},
	{"ponderacion.serie", func(s *reparto.Snapshot, v decimal.Decimal) { s.PondSerie = v }},
	{"ponderacion.sketches", func(s *reparto.Snapshot, v decimal.Decimal) { s.PondSketch = v }},

	// Coeficientes de la formula OTT, RD 9.7. Sin publicar (P-10).
	{"ott.wa", func(s *reparto.Snapshot, v decimal.Decimal) { s.Wa = v }},
	{"ott.wb", func(s *reparto.Snapshot, v decimal.Decimal) { s.Wb = v }},
	{"ott.wc", func(s *reparto.Snapshot, v decimal.Decimal) { s.Wc = v }},

	// Umbral de similitud de la cascada, ADR 0007. No es normativo, pero
	// cambia el resultado de una corrida y por eso entra en el snapshot: sin
	// congelarlo, recalibrarlo reidentificaria obras de un reparto cerrado.
	{"matching.umbral", func(s *reparto.Snapshot, v decimal.Decimal) { s.UmbralMatch = v }},

	// RD 9.1.1(c): 80% artistico y hora televisiva de 48 minutos. Los aplica
	// normalizacion al canonizar la fila, no el motor, pero se congelan aqui
	// por lo mismo que todo lo demas.
	{"duracion.artistica_pct", func(s *reparto.Snapshot, v decimal.Decimal) { s.DuracionArtisticaPct = v }},
	{"duracion.minutos_hora_tv", func(s *reparto.Snapshot, v decimal.Decimal) { s.MinutosHoraTV = v }},
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
// enTransaccionDe y no EnTransaccion: abrir el proceso escribe el snapshot y
// el `procesos.snapshot_id` que lo referencia, y eso es un solo hecho. Si el
// caso de uso abrio una unidad ([aplicacion.UnidadDeTrabajo]), esta escritura
// entra EN ELLA y la confirma quien la abrio; con EnTransaccion el snapshot se
// confirmaria aqui y un fallo posterior dejaria un corte congelado que ninguna
// corrida referencia.
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

		pares := consumidos(filas)
		resuelto, armado, faltan := armarSnapshot(pares)
		if len(faltan) > 0 {
			return &aplicacion.ErrorParametroAusente{Fecha: dia, Claves: faltan}
		}
		if err := congelar(ctx, tx, resuelto, pares); err != nil {
			return err
		}
		id, snap = resuelto, armado
		return nil
	})
	if err != nil {
		return "", reparto.Snapshot{}, err
	}
	return id, snap, nil
}

// SnapshotPorID recupera un snapshot ya congelado.
//
// Reconstruye por el MISMO camino que la resolucion -- [consumidos] y
// [armarSnapshot] sobre las filas guardadas -- por la misma razon que
// [fila.entidad] reconstruye una obra con el constructor del dominio: si lo
// guardado no forma un snapshot valido, la lectura FALLA en vez de servir algo
// que la escritura no habria producido.
//
// Y recalcula el id. Estando direccionado por contenido, el id ES la suma de
// verificacion de las filas: comprobarlo cuesta un hash y convierte "estas
// filas son las que se congelaron" en algo demostrado y no supuesto. La
// alternativa -- confiar en la clave primaria -- deja pasar una fila alterada
// por fuera del adaptador, y el resultado seria una corrida "reproducida" con
// cifras que nunca se pagaron.
func (s *Store) SnapshotPorID(ctx context.Context, id string) (reparto.Snapshot, error) {
	filas, err := leerParametros(ctx, s.ejecutorDe(ctx),
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

	pares := consumidos(filas)
	recalculado, snap, faltan := armarSnapshot(pares)
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

	_, err := tx.Exec(ctx,
		`INSERT INTO snapshots_parametros (snapshot_id, `+columnasParametro+`)
		 SELECT $1, u.clave, u.valor::numeric, u.organo, u.reglamento, u.vigente_desde::date
		   FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[])
		     AS u(clave, valor, organo, reglamento, vigente_desde)
		 ON CONFLICT (snapshot_id, clave) DO NOTHING`,
		id, claves, valores, organos, reglamentos, desdes)
	return traducirError(err, "congelar el snapshot %q", id)
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
func consumidos(filas []parametroResuelto) []parametroResuelto {
	out := make([]parametroResuelto, 0, len(filas))
	for _, p := range filas {
		if !loConsume(p.clave) {
			continue
		}
		p.valor = p.valor.Round(escalaParametro)
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b parametroResuelto) int { return strings.Compare(a.clave, b.clave) })
	return out
}

// loConsume dice si esa clave entra en el snapshot: o es una de las clausulas
// exigidas, o es una tasa `cambio.<ISO>` con codigo.
//
// `cambio.` a secas no es una tasa de nada y se descarta: con el prefijo vacio
// acabaria en Tasas[""] y convertiria el importe sin moneda de cualquier fila.
func loConsume(clave string) bool {
	if slices.ContainsFunc(clausulasDelSnapshot, func(c clausula) bool { return c.clave == clave }) {
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
func armarSnapshot(pares []parametroResuelto) (string, reparto.Snapshot, []string) {
	porClave := make(map[string]parametroResuelto, len(pares))
	for _, p := range pares {
		porClave[p.clave] = p
	}

	snap := reparto.Snapshot{
		MonedaBase: monedaBase,
		Tasas:      map[string]decimal.Decimal{},
	}
	var faltan []string
	for _, c := range clausulasDelSnapshot {
		p, hay := porClave[c.clave]
		if !hay {
			faltan = append(faltan, c.clave)
			continue
		}
		c.en(&snap, p.valor)
	}
	// Se devuelven TODAS las que falten, no la primera: enterarse de una por
	// intento son tantos viajes como parametros sin cargar.
	if len(faltan) > 0 {
		return "", reparto.Snapshot{}, faltan
	}

	var reglamentos []string
	for _, p := range pares {
		if codigo, esTasa := strings.CutPrefix(p.clave, prefijoTasa); esTasa {
			snap.Tasas[strings.ToUpper(codigo)] = p.valor
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

	return idDeSnapshot(pares), snap, nil
}

// idDeSnapshot es el sha256 de los pares (clave, valor) canonicos, uno por
// linea.
//
// Solo clave y valor. La procedencia -- organo, reglamento, vigencia -- viaja
// congelada con cada fila y se lee con ella, pero NO entra en el id: corregir
// una errata en el nombre del organo no cambia ni una cifra del reparto, y si
// entrara, cambiaria el id y dejaria huerfana la corrida que lo referencia.
// Lo que el id direcciona es lo que el motor consume.
func idDeSnapshot(pares []parametroResuelto) string {
	var b strings.Builder
	for _, p := range pares {
		fmt.Fprintf(&b, "%s=%s\n", p.clave, p.texto())
	}
	suma := sha256.Sum256([]byte(b.String()))
	return prefijoSnapshot + hex.EncodeToString(suma[:])
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
