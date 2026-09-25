package postgres

import (
	"context"
	"errors"
	"fmt"
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
	detectada, resuelta, COALESCE(resuelta_por, ''), resuelta_en, nota`

// GuardarAlertas escribe el lote en UNA sentencia (INSERT ... SELECT FROM unnest) y devuelve cuantas entraron.
//
// ON CONFLICT DO NOTHING sobre la clave natural hace la idempotencia en la base, sin carrera.
// Participa en la unidad de trabajo abierta si la hay (enTransaccionDe).
func (s *Store) GuardarAlertas(ctx context.Context, alertas []aplicacion.Alerta) (int, error) {
	if len(alertas) == 0 {
		return 0, nil
	}

	n := len(alertas)
	periodos, tipos := make([]string, n), make([]string, n)
	refTipos, refIDs, refTitulares := make([]string, n), make([]string, n), make([]string, n)
	detalles, detectadas := make([]string, n), make([]time.Time, n)
	for i, a := range alertas {
		periodos[i], tipos[i] = a.Periodo, a.Tipo
		refTipos[i], refIDs[i], refTitulares[i] = a.RefTipo, a.RefID, a.RefTitular
		detalles[i], detectadas[i] = a.Detalle, a.Detectada
	}

	nuevas := 0
	err := s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		etiqueta, err := tx.Exec(ctx,
			`INSERT INTO alertas (periodo, tipo, ref_tipo, ref_id, ref_titular, detalle, detectada)
			 SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::timestamptz[])
			 ON CONFLICT ON CONSTRAINT alerta_unica_por_hallazgo DO NOTHING`,
			periodos, tipos, refTipos, refIDs, refTitulares, detalles, detectadas)
		if err != nil {
			return traducirError(err, "guardar %d alertas del periodo %q", n, alertas[0].Periodo)
		}
		nuevas = int(etiqueta.RowsAffected())
		return nil
	})
	if err != nil {
		return 0, err
	}
	return nuevas, nil
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
		// Un id que no es un UUID entra por aqui (SQLSTATE 22P02) y sale como
		// error de formato, no como 404: la ruta es /alertas/{id} y un id
		// inventado no tiene por que parecer un fallo del servidor.
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
		&a.Detectada, &a.Resuelta, &a.ResueltaPor, &a.ResueltaEn, &a.Nota,
	)
	return a, err
}
