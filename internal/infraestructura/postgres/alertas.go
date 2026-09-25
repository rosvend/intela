package postgres

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.RepositorioAlertas = (*Store)(nil)

// columnasAlerta es la proyeccion canonica de una alerta.
//
// id::text por lo mismo que en bitacora.go: la columna es UUID nativo y el
// cast es lo que deja escanear a un string de Go sin arrastrar un tipo de pgx
// hasta `aplicacion`, que no sabe que existe un UUID.
//
// COALESCE sobre resuelta_por y no un tipo nullable: el modelo del nucleo usa
// la cadena vacia para "nadie", igual que `Asiento.ActorID`. resuelta_en SI es
// puntero en el modelo, y por eso NO lleva COALESCE: un cero de time.Time
// seria un instante del ano 1 indistinguible de un dato mal escrito.
const columnasAlerta = `id::text, periodo, tipo, ref_tipo, ref_id, ref_titular, detalle,
	detectada, resuelta, COALESCE(resuelta_por, ''), resuelta_en, nota, autocerrada`

// GuardarAlertas escribe el lote en UNA sentencia (INSERT ... SELECT FROM unnest) y devuelve cuantas entraron.
//
// ON CONFLICT sobre la clave natural hace la idempotencia en la base, sin carrera. Una alerta que el
// sistema autocerro y vuelve a aparecer se REABRE (cuenta como nueva y se devuelve en reabiertas, para su
// asiento); una que cerro una persona no se toca. `xmax = 0` distingue la fila insertada de la reabierta.
// Participa en la unidad de trabajo abierta si la hay (enTransaccionDe).
func (s *Store) GuardarAlertas(ctx context.Context, alertas []aplicacion.Alerta) (int, []aplicacion.Alerta, error) {
	alertas = sinClavesRepetidas(alertas)
	if len(alertas) == 0 {
		return 0, nil, nil
	}
	c := columnasDeLote(alertas)

	nuevas := 0
	var reabiertas []aplicacion.Alerta
	err := s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx,
			`INSERT INTO alertas (periodo, tipo, ref_tipo, ref_id, ref_titular, detalle, detectada)
			 SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::timestamptz[])
			 ON CONFLICT ON CONSTRAINT alerta_unica_por_hallazgo DO UPDATE
			    SET resuelta = FALSE, autocerrada = FALSE, resuelta_por = NULL, resuelta_en = NULL,
			        nota = '', detalle = EXCLUDED.detalle, detectada = EXCLUDED.detectada
			  WHERE alertas.autocerrada
			 RETURNING `+columnasAlerta+`, NOT (xmax = 0)`,
			c.periodos, c.tipos, c.refTipos, c.refIDs, c.refTitulares, c.detalles, c.detectadas)
		if err != nil {
			return traducirError(err, "guardar %d alertas del periodo %q", len(alertas), alertas[0].Periodo)
		}
		defer filas.Close()
		for filas.Next() {
			var a aplicacion.Alerta
			var reabierta bool
			if err := filas.Scan(
				&a.ID, &a.Periodo, &a.Tipo, &a.RefTipo, &a.RefID, &a.RefTitular, &a.Detalle,
				&a.Detectada, &a.Resuelta, &a.ResueltaPor, &a.ResueltaEn, &a.Nota, &a.Autocerrada, &reabierta,
			); err != nil {
				return traducirError(err, "escanear alerta guardada")
			}
			nuevas++
			if reabierta {
				reabiertas = append(reabiertas, a)
			}
		}
		if err := filas.Err(); err != nil {
			return traducirError(err, "guardar %d alertas del periodo %q", len(alertas), alertas[0].Periodo)
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	return nuevas, reabiertas, nil
}

// AutocerrarAlertas cierra, a nombre del sistema, las abiertas del periodo que no estan en vigentes.
func (s *Store) AutocerrarAlertas(
	ctx context.Context, periodo string, vigentes []aplicacion.Alerta, nota string, cuando time.Time,
) ([]aplicacion.Alerta, error) {
	c := columnasDeLote(vigentes)
	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`UPDATE alertas a
		    SET resuelta = TRUE, autocerrada = TRUE, resuelta_en = $6, nota = $7
		  WHERE a.periodo = $1 AND NOT a.resuelta
		    AND NOT EXISTS (
		      SELECT 1 FROM unnest($2::text[], $3::text[], $4::text[], $5::text[]) AS v(tipo, ref_tipo, ref_id, ref_titular)
		       WHERE v.tipo = a.tipo AND v.ref_tipo = a.ref_tipo AND v.ref_id = a.ref_id AND v.ref_titular = a.ref_titular)
		  RETURNING `+columnasAlerta,
		periodo, c.tipos, c.refTipos, c.refIDs, c.refTitulares, cuando, nota)
	if err != nil {
		return nil, traducirError(err, "autocerrar alertas de %q", periodo)
	}
	defer filas.Close()

	cerradas := make([]aplicacion.Alerta, 0)
	for filas.Next() {
		a, err := escanearAlerta(filas)
		if err != nil {
			return nil, traducirError(err, "escanear alerta autocerrada")
		}
		cerradas = append(cerradas, a)
	}
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "autocerrar alertas de %q", periodo)
	}
	return cerradas, nil
}

// BloquearAlertasDePeriodo toma el cerrojo de aviso de las alertas de un periodo hasta que la transaccion termine.
func (s *Store) BloquearAlertasDePeriodo(ctx context.Context, periodo string) error {
	tx, hay := txDe(ctx)
	if !hay {
		return fmt.Errorf("bloquear las alertas de %s: %w", periodo, errFueraDeUnidad)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, claveCerrojoAlertas(periodo)); err != nil {
		return traducirError(err, "bloquear las alertas de %s", periodo)
	}
	return nil
}

// claveCerrojoAlertas es la clave de aviso del periodo, con namespace propio (mismo criterio que claveCerrojoPeriodo).
func claveCerrojoAlertas(periodo string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("alertas\x00" + periodo))
	return int64(h.Sum64())
}

// loteDeAlertas son las columnas de un lote, una por arreglo, para unnest.
type loteDeAlertas struct {
	periodos, tipos, refTipos, refIDs, refTitulares, detalles []string
	detectadas                                                []time.Time
}

func columnasDeLote(alertas []aplicacion.Alerta) loteDeAlertas {
	n := len(alertas)
	c := loteDeAlertas{
		periodos: make([]string, n), tipos: make([]string, n), refTipos: make([]string, n),
		refIDs: make([]string, n), refTitulares: make([]string, n), detalles: make([]string, n),
		detectadas: make([]time.Time, n),
	}
	for i, a := range alertas {
		c.periodos[i], c.tipos[i] = a.Periodo, a.Tipo
		c.refTipos[i], c.refIDs[i], c.refTitulares[i] = a.RefTipo, a.RefID, a.RefTitular
		c.detalles[i], c.detectadas[i] = a.Detalle, a.Detectada
	}
	return c
}

// sinClavesRepetidas deja la primera alerta de cada clave natural: ON CONFLICT DO UPDATE no admite tocar la misma fila dos veces.
func sinClavesRepetidas(alertas []aplicacion.Alerta) []aplicacion.Alerta {
	vistas := make(map[[5]string]bool, len(alertas))
	out := make([]aplicacion.Alerta, 0, len(alertas))
	for _, a := range alertas {
		k := [5]string{a.Periodo, a.Tipo, a.RefTipo, a.RefID, a.RefTitular}
		if vistas[k] {
			continue
		}
		vistas[k] = true
		out = append(out, a)
	}
	return out
}

// ListarAlertas sirve la bandeja, de la mas reciente a la mas antigua.
//
// ORDER BY detectada DESC, id: `detectada` sola no desempata dos alertas de la
// MISMA pasada -- que llevan todas el mismo instante del Reloj, a proposito --
// y sin el desempate el listado cambia de orden entre lecturas, que es lo que
// el ADR 0005 no admite de nada que alimente una decision. Con paginacion deja
// de ser una cuestion de gusto: sin orden total, dos paginas consecutivas
// pueden repetir una fila y saltarse otra.
//
// Los tres filtros van como parametros que pueden venir vacios, no
// concatenando WHERE: una sola sentencia, un solo plan, y ningun camino en el
// que el texto del SQL dependa de la entrada. Es la forma de
// [Store.ListarCargas].
func (s *Store) ListarAlertas(ctx context.Context, f aplicacion.FiltroAlertas) ([]aplicacion.Alerta, error) {
	// El puntero se aplana a dos parametros: si es nil, el primero es FALSE y
	// la condicion entera pasa. Sin aplanar, pgx tendria que decidir el tipo
	// de un nil y la comparacion `resuelta = NULL` nunca seria cierta.
	filtraResueltas := f.Resueltas != nil
	resueltas := filtraResueltas && *f.Resueltas

	// ConDefecto ANTES de componer, igual que [Store.Buscar] y por un motivo
	// que aqui es especialmente caro: `Paginacion` con Limite en cero
	// significa "el por defecto" por contrato, y sin esta linea se traducia a
	// `LIMIT 0`, o sea CERO filas. Un llamador que no diga nada de paginacion
	// -- toda llamada directa al adaptador, y el dia de manana cualquier
	// consumidor interno -- recibiria la lista vacia como si el periodo
	// estuviera limpio. Es exactamente la clase de fallo silencioso que esta
	// PR existe para cazar, y lo cazaron dos pruebas de este mismo paquete.
	p := f.ConDefecto()

	sql := `SELECT ` + columnasAlerta + ` FROM alertas
		  WHERE ($1 = '' OR periodo = $1)
		    AND ($2 = '' OR tipo = $2)
		    AND (NOT $3 OR resuelta = $4)
		  ORDER BY detectada DESC, id`
	args := []any{f.Periodo, f.Tipo, filtraResueltas, resueltas}
	// El LIMIT se OMITE con LimiteSinTope en vez de mandar un centinela a la
	// base, que es la forma de [Store.Buscar]: asi "dame todo" es una rama que
	// se lee, y no un -1 viajando dentro de la sentencia.
	if p.Limite != aplicacion.LimiteSinTope {
		sql += `
		  LIMIT $5 OFFSET $6`
		args = append(args, p.Limite, p.Desplazamiento)
	}

	filas, err := s.ejecutorDe(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, traducirError(err, "listar alertas del periodo %q", f.Periodo)
	}
	defer filas.Close()

	alertas := make([]aplicacion.Alerta, 0)
	for filas.Next() {
		a, err := escanearAlerta(filas)
		if err != nil {
			return nil, traducirError(err, "escanear alerta")
		}
		alertas = append(alertas, a)
	}
	// Obligatorio: un fallo a mitad de stream solo sale por aqui, y sin esto
	// una lista TRUNCADA pasa por lista completa. Sobre este listado eso se
	// lee como "quedan menos anomalias de las que hay".
	if err := filas.Err(); err != nil {
		return nil, traducirError(err, "listar alertas del periodo %q", f.Periodo)
	}
	return alertas, nil
}

// ResolverAlerta marca una alerta y devuelve como quedo.
//
// # Por que el UPDATE lleva `AND NOT resuelta` y no solo el id
//
// Porque "no existe" y "llegaste segundo" son dos respuestas distintas y el
// UPDATE por id solo no las distingue: pisaria la firma de quien la resolvio
// antes, y el asiento de la bitacora nombraria a quien pulso el boton el
// segundo como si la decision fuera suya. Con la condicion, cero filas
// afectadas puede ser cualquiera de las dos cosas, y por eso se pregunta
// DESPUES cual fue -- dentro de la misma transaccion, asi que la respuesta no
// puede haber cambiado entre medias.
//
// RETURNING y no un SELECT posterior: la fila que se devuelve es exactamente
// la que este UPDATE escribio.
func (s *Store) ResolverAlerta(
	ctx context.Context, id, actorID, nota string, cuando time.Time,
) (aplicacion.Alerta, error) {
	fila := s.ejecutorDe(ctx).QueryRow(ctx,
		`UPDATE alertas
		    SET resuelta = TRUE, resuelta_por = $2, resuelta_en = $3, nota = $4
		  WHERE id = $1::uuid AND NOT resuelta
		  RETURNING `+columnasAlerta,
		id, actorID, cuando, nota)

	a, err := escanearAlerta(fila)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		// La API corta un id no UUID en el handler (400); si otro llamador lo pasa, el cast falla aqui (22P02) y no es ErrNoEncontrado.
		return aplicacion.Alerta{}, traducirError(err, "resolver la alerta %q", id)
	}

	// Cero filas: o no existe, o ya estaba resuelta. La segunda consulta va
	// contra el MISMO ejecutor, asi que dentro de la unidad de trabajo ve
	// exactamente el estado que vio el UPDATE.
	var existe bool
	if err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM alertas WHERE id = $1::uuid)`, id).Scan(&existe); err != nil {
		return aplicacion.Alerta{}, traducirError(err, "comprobar la alerta %q", id)
	}
	if existe {
		return aplicacion.Alerta{}, fmt.Errorf("resolver la alerta %q: %w", id, aplicacion.ErrAlertaYaResuelta)
	}
	return aplicacion.Alerta{}, fmt.Errorf("resolver la alerta %q: %w", id, aplicacion.ErrNoEncontrado)
}

// ContarAlertasSinResolver cuenta las alertas abiertas de un periodo entre los tipos
// que se le pidan. Una lista vacia cuenta todos los tipos.
//
// Los tipos llegan por parametro y no se escriben aqui: cuales bloquean la
// distribucion lo decide `anomalias.EsCritica`, en el dominio. Una lista
// propia en este SQL seria un segundo criterio que nadie mira, y el dia que
// divergieran la compuerta dejaria pasar un periodo que el dominio considera
// bloqueado.
func (s *Store) ContarAlertasSinResolver(ctx context.Context, periodo string, tipos []string) (int, error) {
	var n int
	// COALESCE sobre cardinality, y no `cardinality(...) = 0` a secas: un
	// slice nil de Go viaja como NULL, cardinality(NULL) es NULL, `NULL = 0`
	// es NULL y no FALSE, y la condicion entera se evalua a NULL -- o sea que
	// la consulta devolvia CERO con la lista vacia, que es justo lo contrario
	// de lo que el contrato del puerto promete ("una lista vacia cuenta
	// TODOS"). Un contador que devuelve 0 no falla: deja pasar la compuerta.
	err := s.ejecutorDe(ctx).QueryRow(ctx,
		`SELECT COUNT(*) FROM alertas
		  WHERE periodo = $1 AND NOT resuelta
		    AND (COALESCE(cardinality($2::text[]), 0) = 0 OR tipo = ANY($2::text[]))`,
		periodo, tipos).Scan(&n)
	if err != nil {
		return 0, traducirError(err, "contar alertas abiertas de %q", periodo)
	}
	return n, nil
}

// escanearAlerta lee [columnasAlerta]. Una sola funcion para las dos
// consultas que la comparten, por lo mismo que [escanearUso]: con una por
// consulta, una columna nueva hay que anadirla en dos sitios y el dia que solo
// se anada en uno la fila vuelve con los campos corridos.
//
// pgx.Row y no pgx.Rows para que sirva igual a un QueryRow y a cada vuelta de
// un Query: pgx.Rows satisface Scan con la misma firma.
func escanearAlerta(fila pgx.Row) (aplicacion.Alerta, error) {
	var a aplicacion.Alerta
	err := fila.Scan(
		&a.ID, &a.Periodo, &a.Tipo, &a.RefTipo, &a.RefID, &a.RefTitular, &a.Detalle,
		&a.Detectada, &a.Resuelta, &a.ResueltaPor, &a.ResueltaEn, &a.Nota, &a.Autocerrada,
	)
	return a, err
}
