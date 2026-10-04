package postgres

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var (
	_ aplicacion.RepositorioLiquidacion = (*Store)(nil)
	_ aplicacion.RepositorioIngresos    = (*Store)(nil)
)

// columnasOrden va en una constante y no repetida en cada consulta porque
// escanearOrden lee POSICIONALMENTE: si una consulta cambiara el orden de las
// columnas, el escaneo seguiria compilando y meteria el periodo en el circuito.
const columnasOrden = `id, proceso_id, procesos, titular_id, periodo, circuito, ` +
	`bruto, neto, estado, enviada, arrastres`

const claveSMMLV = "smmlv"

// errFueraDeUnidad es lo que devuelven los metodos que toman cerrojos (los de
// liquidacion y el de alertas) cuando se los llama sin transaccion en curso.
//
// No es defensa decorativa. `pg_advisory_xact_lock` se suelta al terminar la
// transaccion, y contra el pool cada sentencia es su propia transaccion: el
// cerrojo se tomaria y se soltaria antes de volver de esta funcion, y todo lo
// que viniera despues correria sin proteccion. `FOR UPDATE` igual. Fallar es lo
// unico que distingue la serializacion real de una que nadie nota que no esta.
var errFueraDeUnidad = errors.New(
	"cerrojo pedido fuera de una unidad de trabajo: " +
		"un cerrojo de transaccion se suelta antes de la escritura que protege")

func (s *Store) DeTitular(ctx context.Context, titularID string) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 WHERE titular_id = $1
		 ORDER BY periodo, id`, titularID)
	if err != nil {
		return nil, traducirError(err, "liquidaciones de titular %q", titularID)
	}
	return escanearOrdenes(ctx, s, filas, "liquidaciones de titular %q", titularID)
}

func (s *Store) Listar(ctx context.Context) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 ORDER BY periodo, titular_id, id`)
	if err != nil {
		return nil, traducirError(err, "listar liquidaciones")
	}
	return escanearOrdenes(ctx, s, filas, "listar liquidaciones")
}

// DeProceso busca por la corrida de referencia Y por la lista de contribuyentes.
//
// El `= ANY(procesos)` no es redundante con el `proceso_id = $1`: desde el
// ADR 0019 una orden agrega varias corridas y solo UNA de ellas queda en
// `proceso_id`, asi que preguntar solo por esa columna diria que las demas no
// produjeron ninguna orden.
func (s *Store) DeProceso(ctx context.Context, procesoID string) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 WHERE proceso_id = $1 OR $1 = ANY(procesos)
		 ORDER BY titular_id, id`, procesoID)
	if err != nil {
		return nil, traducirError(err, "liquidaciones del proceso %q", procesoID)
	}
	return escanearOrdenes(ctx, s, filas, "liquidaciones del proceso %q", procesoID)
}

func (s *Store) DePeriodoCircuito(
	ctx context.Context, periodo string, circuito reparto.Circuito,
) ([]liquidacion.OrdenDePago, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 WHERE periodo = $1 AND circuito = $2
		 ORDER BY titular_id, id`, periodo, string(circuito))
	if err != nil {
		return nil, traducirError(err, "liquidaciones de %s/%s", periodo, circuito)
	}
	return escanearOrdenes(ctx, s, filas, "liquidaciones de %s/%s", periodo, circuito)
}

// BloquearPeriodo toma el cerrojo de aviso de un (periodo, circuito) hasta que
// la transaccion en curso termine.
//
// Es de aviso -- `pg_advisory_xact_lock` -- y no de fila porque lo que hay que
// serializar es la decision de si emitir, y esa se toma cuando todavia no
// existe ninguna fila: dos generaciones a la vez leen las dos "no hay ordenes
// para este periodo" y emiten las dos. El UNIQUE de `ordenes_pago` evitaria la
// fila duplicada, pero no el resto del dano -- dos notificaciones, dos
// asientos, y una diferida arrastrada por la generacion cuya insercion se
// descarta --, porque eso ya paso antes del INSERT.
//
// La clave se calcula en Go y no con `hashtext()`: esa funcion no esta
// documentada y su resultado podria cambiar entre versiones de PostgreSQL, lo
// que rotaria el espacio de cerrojos en una actualizacion sin que nada lo
// dijera. FNV-1a de 64 bits sobre los mismos bytes da el mismo numero en
// cualquier version y en cualquier proceso, que es todo lo que un cerrojo de
// aviso necesita.
func (s *Store) BloquearPeriodo(ctx context.Context, periodo string, circuito reparto.Circuito) error {
	tx, hay := txDe(ctx)
	if !hay {
		return fmt.Errorf("bloquear %s/%s: %w", periodo, circuito, errFueraDeUnidad)
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock($1)`, claveCerrojoPeriodo(periodo, circuito),
	); err != nil {
		return traducirError(err, "bloquear %s/%s", periodo, circuito)
	}
	return nil
}

// claveCerrojoPeriodo mapea (periodo, circuito) al entero de 64 bits que
// `pg_advisory_xact_lock` usa como identidad.
//
// El prefijo del namespace y los separadores NUL estan a proposito: los
// cerrojos de aviso comparten un espacio unico para toda la base, asi que sin
// prefijo este cerrojo podria colisionar con el de otro modulo, y sin
// separador ("2026" + "01" y "202" + "601") dos pares distintos darian la
// misma clave y se serializarian entre si sin razon.
func claveCerrojoPeriodo(periodo string, circuito reparto.Circuito) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("ordenes_pago\x00" + periodo + "\x00" + string(circuito)))
	// La conversion a int64 conserva los 64 bits; que la mitad de las claves
	// salgan negativas es irrelevante, pg_advisory_xact_lock toma un bigint.
	return int64(h.Sum64())
}

// DiferidasDeTitular lee las diferidas de un titular con su fila bloqueada.
//
// Solo del mismo circuito y con periodo estrictamente anterior a antesDe
// (ADR 0019, RD 7.4 / 13.3): el arrastre no cruza circuitos ni se absorbe
// hacia atras.
//
// FOR UPDATE, y por eso exige transaccion: el arrastre de R-11 es un
// leer-modificar-escribir sobre esas filas -- se les suma el neto a otra orden
// y se marcan acumuladas -- y dos generaciones concurrentes del mismo titular
// (dos periodos distintos, que NO comparten el cerrojo de aviso) las leerian
// las dos como diferidas y pagarian el saldo dos veces. Con el cerrojo, la
// segunda espera el commit de la primera y vuelve a evaluar el `estado =
// 'diferida'`, que ya no se cumple.
func (s *Store) DiferidasDeTitular(
	ctx context.Context, titularID string, circuito reparto.Circuito, antesDe string,
) ([]liquidacion.OrdenDePago, error) {
	tx, hay := txDe(ctx)
	if !hay {
		return nil, fmt.Errorf("diferidas de %q: %w", titularID, errFueraDeUnidad)
	}
	// El desglose no se pide aqui: una diferida se arrastra por su NETO, sin
	// volver a deducir (ver liquidacion.IncorporarArrastre), asi que las
	// deducciones de la orden de origen no se leen ni se copian. Por eso esta
	// consulta no pasa por escanearOrdenes.
	filas, err := tx.Query(ctx,
		`SELECT `+columnasOrden+` FROM ordenes_pago
		 WHERE titular_id = $1 AND estado = $2
		   AND circuito = $3 AND periodo < $4
		 ORDER BY periodo, id
		 FOR UPDATE`, titularID, string(liquidacion.EstadoDiferida), string(circuito), antesDe)
	if err != nil {
		return nil, traducirError(err, "diferidas de %q", titularID)
	}
	defer filas.Close()

	ordenes := []liquidacion.OrdenDePago{}
	for filas.Next() {
		o, err := escanearOrden(filas)
		if err != nil {
			return nil, traducirError(err, "diferidas de %q", titularID)
		}
		o.Deducciones = []liquidacion.Deduccion{}
		ordenes = append(ordenes, o)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "diferidas de %q", titularID)
	}
	return ordenes, nil
}

// EmitirOrdenes inserta ordenes nuevas con su desglose y NO pisa lo que ya
// hubiera bajo el mismo id.
//
// `ON CONFLICT (id) DO NOTHING` y no un `DO UPDATE`: el id de una orden es
// estable (`liq-{periodo}-{circuito}-{titular}`), asi que un upsert volveria a
// poner en `enviada` una orden ya aceptada por silencio -- reabriendo un plazo
// de R-10 que ya vencio -- y le devolveria el bruto de antes a una que ya
// habia absorbido un arrastre. Las deducciones solo se escriben si la orden se
// inserto de verdad, por lo mismo: borrarlas y reescribirlas sobre una orden
// que ya existia cambiaria su desglose sin cambiar su neto.
func (s *Store) EmitirOrdenes(ctx context.Context, ordenes []liquidacion.OrdenDePago) error {
	if len(ordenes) == 0 {
		return nil
	}
	return s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		for _, o := range ordenes {
			insertada, err := insertarOrden(ctx, tx, o)
			if err != nil {
				return err
			}
			if !insertada {
				continue
			}
			for _, d := range o.Deducciones {
				if _, err := tx.Exec(ctx,
					`INSERT INTO ordenes_pago_deducciones (orden_id, concepto, monto)
					 VALUES ($1,$2,$3)`, o.ID, d.Concepto, d.Monto); err != nil {
					return traducirError(err, "guardar deduccion %q de %q", d.Concepto, o.ID)
				}
			}
		}
		return nil
	})
}

// insertarOrden devuelve si la fila se inserto. false no es un error: es "ya
// existia", que es la respuesta idempotente que busca quien llama.
func insertarOrden(ctx context.Context, tx pgx.Tx, o liquidacion.OrdenDePago) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO ordenes_pago (
			id, proceso_id, procesos, titular_id, periodo, circuito,
			bruto, neto, estado, enviada, arrastres
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO NOTHING`,
		o.ID, o.ProcesoID, sinNil(o.Procesos), o.TitularID, o.Periodo, o.Circuito,
		o.Bruto, o.Neto, string(o.Estado), o.EnviadaDia, sinNil(o.Arrastres))
	if err != nil {
		return false, traducirError(err, "emitir orden %q", o.ID)
	}
	return tag.RowsAffected() == 1, nil
}

// TransicionarOrdenes mueve el estado de cada orden solo si sigue en `desde`, y
// devuelve las que de verdad cambiaron.
//
// El `WHERE estado = $3` es la condicion que convierte una carrera en un
// no-op en vez de en una sobrescritura: entre la lectura que decidio la
// transicion y este UPDATE cabe otra transaccion que ya movio la orden, y
// escribir sin condicion la devolveria al estado que esta transaccion creia
// vigente. Solo el estado, nunca bruto ni neto ni arrastres: eso no es de una
// transicion, y tocarlo podria deshacer un arrastre recien escrito.
func (s *Store) TransicionarOrdenes(
	ctx context.Context, ordenes []liquidacion.OrdenDePago, desde liquidacion.Estado,
) ([]liquidacion.OrdenDePago, error) {
	aplicadas := []liquidacion.OrdenDePago{}
	if len(ordenes) == 0 {
		return aplicadas, nil
	}
	err := s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		for _, o := range ordenes {
			tag, err := tx.Exec(ctx,
				`UPDATE ordenes_pago SET estado = $2 WHERE id = $1 AND estado = $3`,
				o.ID, string(o.Estado), string(desde))
			if err != nil {
				return traducirError(err, "transicion %s -> %s de %q", desde, o.Estado, o.ID)
			}
			if tag.RowsAffected() == 1 {
				aplicadas = append(aplicadas, o)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return aplicadas, nil
}

func (s *Store) DocumentosDe(ctx context.Context, titularID string) (liquidacion.Documentos, error) {
	todos, err := s.Documentos(ctx)
	if err != nil {
		return liquidacion.Documentos{}, err
	}
	return todos[titularID], nil
}

func (s *Store) Documentos(ctx context.Context) (map[string]liquidacion.Documentos, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT titular_id, tipo FROM documentos_titular ORDER BY titular_id, tipo`)
	if err != nil {
		return nil, traducirError(err, "listar documentos de titulares")
	}
	defer filas.Close()

	docs := map[string]liquidacion.Documentos{}
	for filas.Next() {
		var titularID, tipo string
		if err := filas.Scan(&titularID, &tipo); err != nil {
			return nil, traducirError(err, "escanear documento de titular")
		}
		d := docs[titularID]
		switch tipo {
		case "rut":
			d.RUT = true
		case "certificacion_bancaria":
			d.CertificacionBancaria = true
		}
		docs[titularID] = d
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar documentos de titulares")
	}
	return docs, nil
}

// MetaDeProceso lee la cabecera de una corrida y TODAS sus firmas.
//
// Todas, de cualquier revision, y no solo las de la vigente: quien comprueba
// la compuerta necesita poder distinguir una corrida que nadie firmo de una
// rechazada cuyas firmas dejaron de contar al subir la revision, y con el
// filtro aqui las dos llegarian como "cero firmas". Y las firmas de la
// verificacion que la corrida ya dejo atras estan, por construccion, en una
// revision anterior a la vigente.
func (s *Store) MetaDeProceso(ctx context.Context, procesoID string) (aplicacion.MetaProceso, error) {
	metas, err := s.metasDeProcesos(ctx, `WHERE p.id = $1`, procesoID)
	if err != nil {
		return aplicacion.MetaProceso{}, traducirError(err, "proceso %q", procesoID)
	}
	if len(metas) == 0 {
		return aplicacion.MetaProceso{}, fmt.Errorf("proceso %q: %w", procesoID, aplicacion.ErrNoEncontrado)
	}
	return metas[0], nil
}

// CorridasDePeriodo lee la cabecera y las firmas de TODAS las corridas de un
// periodo y circuito, en cualquier etapa, ordenadas por id.
//
// Sin filtrar por etapa ni por firmas: cuales ya cerraron la verificacion y
// cuales todavia no es la regla que aplica el caso de uso (ADR 0024). Antes
// ese filtro vivia aqui, en SQL, y pedia las firmas sobre la revision vigente,
// que es justo donde una corrida que salio de verificacion no las tiene (#193).
//
// No comprueba que exista `resultados_proceso`: una corrida lista sin
// resultado es una inconsistencia, y dejarla fuera en silencio la convertiria
// en dinero que nadie liquida. Que falle al pedir su insumo es lo correcto.
func (s *Store) CorridasDePeriodo(
	ctx context.Context, periodo string, circuito reparto.Circuito,
) ([]aplicacion.MetaProceso, error) {
	metas, err := s.metasDeProcesos(ctx, `WHERE p.periodo = $1 AND p.circuito = $2`, periodo, string(circuito))
	if err != nil {
		return nil, traducirError(err, "corridas de %s/%s", periodo, circuito)
	}
	return metas, nil
}

// metasDeProcesos es la lectura comun de [Store.MetaDeProceso] y
// [Store.CorridasDePeriodo]: la cabecera de cada corrida que cumpla filtro y
// todas sus firmas, en dos consultas y no una por corrida.
func (s *Store) metasDeProcesos(ctx context.Context, filtro string, args ...any) ([]aplicacion.MetaProceso, error) {
	ex := s.ejecutorDe(ctx)
	filas, err := ex.Query(ctx,
		`SELECT p.id, p.circuito, p.etapa, p.periodo, p.bolsa_id, p.revision
		   FROM procesos p `+filtro+`
		  ORDER BY p.id`, args...)
	if err != nil {
		return nil, err
	}
	metas := []aplicacion.MetaProceso{}
	indice := map[string]int{}
	for filas.Next() {
		var (
			m               aplicacion.MetaProceso
			circuito, etapa string
		)
		if err := filas.Scan(&m.ID, &circuito, &etapa, &m.Periodo, &m.BolsaID, &m.Revision); err != nil {
			filas.Close()
			return nil, err
		}
		m.Circuito = reparto.Circuito(circuito)
		m.Etapa = reparto.Etapa(etapa)
		m.Firmas = []reparto.Firma{}
		indice[m.ID] = len(metas)
		metas = append(metas, m)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		return metas, nil
	}

	ids := make([]string, len(metas))
	for i, m := range metas {
		ids[i] = m.ID
	}
	firmas, err := ex.Query(ctx,
		`SELECT proceso_id, rol, actor_id, revision FROM firmas
		  WHERE proceso_id = ANY($1)
		  ORDER BY proceso_id, revision, rol`, ids)
	if err != nil {
		return nil, err
	}
	defer firmas.Close()
	for firmas.Next() {
		var (
			procesoID string
			f         reparto.Firma
		)
		if err := firmas.Scan(&procesoID, &f.Rol, &f.ActorID, &f.SobreRev); err != nil {
			return nil, err
		}
		i := indice[procesoID]
		metas[i].Firmas = append(metas[i].Firmas, f)
	}
	return metas, firmas.Err()
}

func (s *Store) InsumoDeProceso(ctx context.Context, procesoID string) (aplicacion.InsumoLiquidacion, error) {
	var insumo aplicacion.InsumoLiquidacion
	insumo.ProcesoID = procesoID
	ex := s.ejecutorDe(ctx)
	err := ex.QueryRow(ctx,
		`SELECT p.periodo, r.bruto, r.admin, r.social, r.reserva
		 FROM procesos p
		 JOIN resultados_proceso r ON r.proceso_id = p.id
		 WHERE p.id = $1`, procesoID).
		Scan(&insumo.Periodo, &insumo.Bruto, &insumo.Admin, &insumo.Social, &insumo.Reserva)
	if err != nil {
		return aplicacion.InsumoLiquidacion{}, traducirError(err, "insumo del proceso %q", procesoID)
	}

	filas, err := ex.Query(ctx, `
		SELECT obra_id, titular_id, ipi, porcentaje, importe
		FROM resultados_titular
		WHERE proceso_id = $1
		ORDER BY titular_id, obra_id`, procesoID)
	if err != nil {
		return aplicacion.InsumoLiquidacion{}, traducirError(err, "lineas del proceso %q", procesoID)
	}
	defer filas.Close()

	insumo.Titulares = []reparto.LineaTitular{}
	for filas.Next() {
		var linea reparto.LineaTitular
		if err := filas.Scan(&linea.ObraID, &linea.TitularID, &linea.IPI, &linea.Porcentaje, &linea.Importe); err != nil {
			return aplicacion.InsumoLiquidacion{}, traducirError(err, "escanear linea del proceso %q", procesoID)
		}
		insumo.Titulares = append(insumo.Titulares, linea)
	}
	if err := filas.Err(); err != nil {
		return aplicacion.InsumoLiquidacion{}, traducirError(err, "lineas del proceso %q", procesoID)
	}
	return insumo, nil
}

func (s *Store) SMMLVVigente(ctx context.Context, en time.Time) (decimal.Decimal, error) {
	var valor decimal.NullDecimal
	err := s.ejecutorDe(ctx).QueryRow(ctx, `
		SELECT valor FROM parametros
		WHERE clave = $1
		  AND vigente_desde <= $2
		  AND (vigente_hasta IS NULL OR vigente_hasta > $2)`,
		claveSMMLV, en).Scan(&valor)
	if err != nil {
		traducido := traducirError(err, "smmlv vigente en %s", en.Format("2006-01-02"))
		if errors.Is(traducido, aplicacion.ErrNoEncontrado) {
			return decimal.Zero, fmt.Errorf("%w: %s", aplicacion.ErrParametroAusente, claveSMMLV)
		}
		return decimal.Zero, traducido
	}
	// NULL es una fila textual (migracion 00025): el SMMLV es una cifra.
	if !valor.Valid {
		return decimal.Zero, fmt.Errorf("%w: %s es textual, se esperaba una cifra", aplicacion.ErrParametroInvalido, claveSMMLV)
	}
	return valor.Decimal, nil
}

// sinNil convierte un slice nil en uno vacio: `TEXT[] NOT NULL` rechaza el NULL
// que pgx escribiria de un nil.
func sinNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func escanearOrdenes(ctx context.Context, s *Store, filas pgx.Rows, formato string, args ...any) ([]liquidacion.OrdenDePago, error) {
	defer filas.Close()

	ordenes := []liquidacion.OrdenDePago{}
	ids := make([]string, 0)
	for filas.Next() {
		o, err := escanearOrden(filas)
		if err != nil {
			return nil, traducirError(err, formato, args...)
		}
		ordenes = append(ordenes, o)
		ids = append(ids, o.ID)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, formato, args...)
	}

	deducciones, err := s.deduccionesDe(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range ordenes {
		ds := deducciones[ordenes[i].ID]
		if ds == nil {
			ds = []liquidacion.Deduccion{}
		}
		ordenes[i].Deducciones = ds
	}
	return ordenes, nil
}

func escanearOrden(fila pgx.Row) (liquidacion.OrdenDePago, error) {
	var (
		o       liquidacion.OrdenDePago
		estado  string
		enviada time.Time
	)
	err := fila.Scan(
		&o.ID, &o.ProcesoID, &o.Procesos, &o.TitularID, &o.Periodo, &o.Circuito,
		&o.Bruto, &o.Neto, &estado, &enviada, &o.Arrastres)
	o.Procesos = sinNil(o.Procesos)
	o.Arrastres = sinNil(o.Arrastres)
	o.Estado = liquidacion.Estado(estado)
	o.EnviadaDia = enviada.Format("2006-01-02")
	return o, err
}

func (s *Store) deduccionesDe(ctx context.Context, ids []string) (map[string][]liquidacion.Deduccion, error) {
	out := map[string][]liquidacion.Deduccion{}
	if len(ids) == 0 {
		return out, nil
	}
	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		SELECT orden_id, concepto, monto
		FROM ordenes_pago_deducciones
		WHERE orden_id = ANY($1)
		ORDER BY orden_id, concepto`, ids)
	if err != nil {
		return nil, traducirError(err, "deducciones de ordenes")
	}
	defer filas.Close()

	for filas.Next() {
		var (
			ordenID string
			d       liquidacion.Deduccion
		)
		if err := filas.Scan(&ordenID, &d.Concepto, &d.Monto); err != nil {
			return nil, traducirError(err, "escanear deduccion")
		}
		out[ordenID] = append(out[ordenID], d)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "deducciones de ordenes")
	}
	return out, nil
}

// IngresosDe lista las lineas netas de un titular, recortadas por el filtro.
//
// El titularID lo pone el caso de uso desde la sesion. Aqui no hay forma de
// pedir "los de otro": la consulta lleva WHERE titular_id = $1.
//
// La fuente es la del canal de la bolsa de ESA corrida (ADR 0019: una
// corrida = una bolsa = un canal), el mismo corte que [Store.UsosDeCanal].
// Filtrar solo por obra y periodo mezclaria el dinero de otra bolsa que
// uso la misma obra en el mismo periodo.
func (s *Store) IngresosDe(ctx context.Context, titularID string, f aplicacion.FiltroIngresos) ([]aplicacion.Ingreso, error) {
	filas, err := s.pool.Query(ctx, `
		SELECT
			rt.proceso_id,
			rt.obra_id,
			rt.titular_id,
			o.titulo,
			p.periodo,
			rt.importe,
			COALESCE((
				SELECT string_agg(DISTINCT r.fuente, ', ' ORDER BY r.fuente)
				FROM usos u
				JOIN reportes r ON r.id = u.reporte_id
				WHERE u.obra_id = rt.obra_id
				  AND r.periodo = p.periodo
				  AND u.canal_id = b.usuario_id
				  AND NOT u.oni
			), '') AS fuente
		FROM resultados_titular rt
		JOIN procesos p ON p.id = rt.proceso_id
		JOIN bolsas b ON b.id = p.bolsa_id
		JOIN obras o ON o.id = rt.obra_id
		WHERE rt.titular_id = $1
		  AND ($2 = '' OR rt.obra_id = $2)
		  AND ($3 = '' OR p.periodo = $3)
		  AND (
		        $4 = '' OR EXISTS (
		            SELECT 1
		            FROM usos u
		            JOIN reportes r ON r.id = u.reporte_id
		            WHERE u.obra_id = rt.obra_id
		              AND r.periodo = p.periodo
		              AND u.canal_id = b.usuario_id
		              AND r.fuente = $4
		              AND NOT u.oni
		        )
		      )
		ORDER BY p.periodo, o.titulo, rt.obra_id`,
		titularID, f.ObraID, f.Periodo, f.Fuente,
	)
	if err != nil {
		return nil, traducirError(err, "ingresos de titular %q", titularID)
	}
	defer filas.Close()

	ingresos := []aplicacion.Ingreso{}
	for filas.Next() {
		var (
			procesoID, obraID, tit, titulo, periodo, fuente string
			neto                                            decimal.Decimal
		)
		if err := filas.Scan(&procesoID, &obraID, &tit, &titulo, &periodo, &neto, &fuente); err != nil {
			return nil, traducirError(err, "escanear ingreso de titular %q", titularID)
		}
		ingresos = append(ingresos, aplicacion.Ingreso{
			Ref:     aplicacion.FormarRef(procesoID, obraID, tit),
			ObraID:  obraID,
			Obra:    titulo,
			Fuente:  fuente,
			Periodo: periodo,
			Neto:    neto,
		})
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "ingresos de titular %q", titularID)
	}
	return ingresos, nil
}
