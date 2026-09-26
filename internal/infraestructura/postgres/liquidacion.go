package postgres

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
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
	_ aplicacion.RepositorioExplicacion = (*Store)(nil)
)

// columnasOrden va en una constante y no repetida en cada consulta porque
// escanearOrden lee POSICIONALMENTE: si una consulta cambiara el orden de las
// columnas, el escaneo seguiria compilando y meteria el periodo en el circuito.
const columnasOrden = `id, proceso_id, procesos, titular_id, periodo, circuito, ` +
	`bruto, neto, estado, enviada, arrastres`

const claveSMMLV = "smmlv"

// errFueraDeUnidad es lo que devuelven los dos metodos que toman cerrojos
// cuando se los llama sin transaccion en curso.
//
// No es defensa decorativa. `pg_advisory_xact_lock` se suelta al terminar la
// transaccion, y contra el pool cada sentencia es su propia transaccion: el
// cerrojo se tomaria y se soltaria antes de volver de esta funcion, y todo lo
// que viniera despues correria sin proteccion. `FOR UPDATE` igual. Fallar es lo
// unico que distingue la serializacion real de una que nadie nota que no esta.
var errFueraDeUnidad = errors.New(
	"cerrojo de liquidacion pedido fuera de una unidad de trabajo: " +
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
// filtro aqui las dos llegarian como "cero firmas".
func (s *Store) MetaDeProceso(ctx context.Context, procesoID string) (aplicacion.MetaProceso, error) {
	meta := aplicacion.MetaProceso{ID: procesoID}
	var circuito, etapa string
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT circuito, etapa, periodo, revision FROM procesos WHERE id = $1`, procesoID).
		Scan(&circuito, &etapa, &meta.Periodo, &meta.Revision)
	if err != nil {
		return aplicacion.MetaProceso{}, traducirError(err, "proceso %q", procesoID)
	}
	meta.Circuito = reparto.Circuito(circuito)
	meta.Etapa = reparto.Etapa(etapa)

	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT rol, actor_id, revision FROM firmas
		 WHERE proceso_id = $1
		 ORDER BY revision, rol`, procesoID)
	if err != nil {
		return aplicacion.MetaProceso{}, traducirError(err, "firmas del proceso %q", procesoID)
	}
	defer filas.Close()

	meta.Firmas = []reparto.Firma{}
	for filas.Next() {
		var f reparto.Firma
		if err := filas.Scan(&f.Rol, &f.ActorID, &f.SobreRev); err != nil {
			return aplicacion.MetaProceso{}, traducirError(err, "escanear firma del proceso %q", procesoID)
		}
		meta.Firmas = append(meta.Firmas, f)
	}
	if err := filas.Err(); err != nil {
		return aplicacion.MetaProceso{}, traducirError(err, "firmas del proceso %q", procesoID)
	}
	return meta, nil
}

// ProcesosListos son las corridas de un periodo y circuito que ya pasaron la
// compuerta del RD 13.5.
//
// Las dos firmas se piden con `EXISTS` contra `p.revision` y no contra un
// numero fijo: un rechazo sube la revision del proceso y las firmas de la
// anterior dejan de contar (es la razon de que `revision` este en la PK de
// `firmas`). Un `COUNT(*) = 2` sobre la tabla entera aceptaria dos firmas de
// revisiones distintas, que es exactamente la doble firma que el control
// existe para impedir.
//
// No comprueba que exista `resultados_proceso`: una corrida firmada sin
// resultado es una inconsistencia, y dejarla fuera en silencio la convertiria
// en dinero que nadie liquida. Que falle al pedir su insumo es lo correcto.
func (s *Store) ProcesosListos(
	ctx context.Context, periodo string, circuito reparto.Circuito,
) ([]string, error) {
	filas, err := s.ejecutorDe(ctx).Query(ctx, `
		SELECT p.id
		FROM procesos p
		WHERE p.periodo = $1
		  AND p.circuito = $2
		  AND p.etapa = $3
		  AND EXISTS (SELECT 1 FROM firmas f
		              WHERE f.proceso_id = p.id AND f.rol = 'distribucion'
		                AND f.revision = p.revision)
		  AND EXISTS (SELECT 1 FROM firmas f
		              WHERE f.proceso_id = p.id AND f.rol = 'contabilidad'
		                AND f.revision = p.revision)
		ORDER BY p.id`,
		periodo, string(circuito), string(reparto.EtapaLiquidacionFinal))
	if err != nil {
		return nil, traducirError(err, "corridas listas de %s/%s", periodo, circuito)
	}
	defer filas.Close()

	ids := []string{}
	for filas.Next() {
		var id string
		if err := filas.Scan(&id); err != nil {
			return nil, traducirError(err, "escanear corrida lista de %s/%s", periodo, circuito)
		}
		ids = append(ids, id)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "corridas listas de %s/%s", periodo, circuito)
	}
	return ids, nil
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
	var valor decimal.Decimal
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
	return valor, nil
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

// PorLinea reconstruye el linaje de una cifra a partir de lo persistido:
// la linea de titular, la corrida, el reporte que pondero, el match, el
// snapshot y las deducciones. No recalcula el motor (ADR 0005): lee.
//
// El prorrateo bruto/deducciones es trabajo del dominio
// ([liquidacion.ProrratearLinea]): aqui solo se deshace la resta de la bolsa
// sobre la linea del titular para mostrarla.
func (s *Store) PorLinea(ctx context.Context, procesoID, obraID, titularID string) (aplicacion.Explicacion, error) {
	var (
		x          aplicacion.Explicacion
		bolsaBruto decimal.Decimal
		admin      decimal.Decimal
		social     decimal.Decimal
		reserva    decimal.Decimal
		bolsaNeto  decimal.Decimal
	)
	err := s.pool.QueryRow(ctx, `
		SELECT
			rt.titular_id, rt.ipi, rt.porcentaje, rt.importe,
			p.id, p.periodo, p.circuito, COALESCE(p.snapshot_id, ''), p.reglamento,
			o.id, o.titulo,
			rp.bruto, rp.admin, rp.social, rp.reserva, rp.neto
		FROM resultados_titular rt
		JOIN procesos p ON p.id = rt.proceso_id
		JOIN obras o ON o.id = rt.obra_id
		JOIN resultados_proceso rp ON rp.proceso_id = rt.proceso_id
		WHERE rt.proceso_id = $1 AND rt.obra_id = $2 AND rt.titular_id = $3`,
		procesoID, obraID, titularID,
	).Scan(
		&x.TitularID, &x.Split.IPI, &x.Split.Porcentaje, &x.Neto,
		&x.Corrida.ProcesoID, &x.Corrida.Periodo, &x.Corrida.Circuito,
		&x.Regla.SnapshotID, &x.Regla.Reglamento,
		&x.Obra.ID, &x.Obra.Titulo,
		&bolsaBruto, &admin, &social, &reserva, &bolsaNeto,
	)
	if err != nil {
		return aplicacion.Explicacion{}, traducirError(err,
			"explicar %s", aplicacion.FormarRef(procesoID, obraID, titularID))
	}
	x.Ref = aplicacion.FormarRef(procesoID, obraID, titularID)
	x.Split.TitularID = x.TitularID
	// resultados_titular no guarda la version de la declaracion con la que
	// se repartio. La abierta de hoy no es esa, y un 1 por defecto pareceria
	// cierto (ADR 0006). Version queda nil hasta que la corrida la persista.

	linea := liquidacion.ProrratearLinea(x.Neto, admin, social, reserva, bolsaNeto)
	x.Bruto = linea.Bruto
	if bolsaNeto.IsZero() {
		x.Deducciones = []aplicacion.Deduccion{}
	} else {
		x.Deducciones = deduccionesDeLinea(linea, admin, social, reserva, bolsaBruto)
	}

	if err := s.origenDeObra(ctx, &x, obraID); err != nil {
		return aplicacion.Explicacion{}, err
	}
	return x, nil
}

// deduccionesDeLinea arma el desglose para ExplicarCifra. Los porcentajes son
// los de la bolsa (tasas normativas del proceso); los montos son los ya
// redondeados de [liquidacion.ProrratearLinea], para que Bruto - Σ = Neto.
func deduccionesDeLinea(
	l liquidacion.Linea,
	adminProc, socialProc, reservaProc, bolsaBruto decimal.Decimal,
) []aplicacion.Deduccion {
	cien := decimal.NewFromInt(100)
	pct := func(parte decimal.Decimal) decimal.Decimal {
		if bolsaBruto.IsZero() {
			return decimal.Zero
		}
		return parte.Div(bolsaBruto).Mul(cien).Round(2)
	}
	return []aplicacion.Deduccion{
		{Concepto: "gastos administrativos", Porcentaje: pct(adminProc), Monto: l.Admin},
		{Concepto: "bienestar social", Porcentaje: pct(socialProc), Monto: l.Social},
		{Concepto: "reserva", Porcentaje: pct(reservaProc), Monto: l.Reserva},
	}
}

// origenDeObra rellena reporte, escalon y puntaje. Sin uso la cifra sigue
// existiendo: el origen queda vacio, no se convierte un 200 en 404.
//
// Solo entran los usos del canal de la bolsa de la corrida, igual que
// [Store.UsosDeCanal]. Varias fuentes se listan (ordenadas, separadas por
// coma) cuando caen en ESE canal; una bolsa distinta del mismo periodo no
// cuenta. id/sha256/escalon/puntaje quedan del uso de mayor puntaje.
func (s *Store) origenDeObra(ctx context.Context, x *aplicacion.Explicacion, obraID string) error {
	filas, err := s.pool.Query(ctx, `
		SELECT r.id, r.fuente, r.sha256, u.escalon, u.puntaje
		FROM usos u
		JOIN reportes r ON r.id = u.reporte_id
		JOIN procesos p ON p.id = $2
		JOIN bolsas b ON b.id = p.bolsa_id
		WHERE u.obra_id = $1
		  AND r.periodo = p.periodo
		  AND u.canal_id = b.usuario_id
		  AND NOT u.oni
		ORDER BY u.puntaje DESC, r.id`,
		obraID, x.Corrida.ProcesoID,
	)
	if err != nil {
		return traducirError(err, "uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
	}
	defer filas.Close()

	var (
		fuentes []string
		visto   = map[string]struct{}{}
		primero = true
	)
	for filas.Next() {
		var (
			id, fuente, sha, escalon string
			puntaje                  decimal.Decimal
		)
		if err := filas.Scan(&id, &fuente, &sha, &escalon, &puntaje); err != nil {
			return traducirError(err, "escanear uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
		}
		if primero {
			x.Reporte.ID = id
			x.Reporte.SHA256 = sha
			x.Obra.Escalon = escalon
			x.Obra.Puntaje = puntaje
			primero = false
		}
		if _, ok := visto[fuente]; !ok {
			visto[fuente] = struct{}{}
			fuentes = append(fuentes, fuente)
		}
	}
	if err := filas.Err(); err != nil {
		return traducirError(err, "uso de obra %q periodo %q", obraID, x.Corrida.Periodo)
	}
	if len(fuentes) > 0 {
		// Mismo criterio que IngresosDe (string_agg ORDER BY fuente).
		slices.Sort(fuentes)
		x.Reporte.Fuente = strings.Join(fuentes, ", ")
	}
	return nil
}
