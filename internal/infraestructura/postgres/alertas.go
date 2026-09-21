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

// GuardarAlertas escribe las alertas que todavia no estaban.
//
// # ON CONFLICT DO NOTHING sobre la clave natural
//
// La idempotencia de EvaluarAnomalias se decide AQUI, en la base, y no con un
// SELECT previo en el caso de uso: entre la consulta y el INSERT cabe otra
// pasada -- el scheduler y una persona pueden evaluar el mismo periodo a la
// vez -- y la unica comprobacion de unicidad sin carrera es la de la base. Es
// el mismo criterio que [esClaveDuplicada] explica para `reportes`.
//
// Y con DO NOTHING en vez de DO UPDATE: si la alerta ya existe y alguien la
// resolvio, volver a detectarla NO la reabre. Ver el contrato del puerto.
//
// # El lote es una transaccion
//
// Se apoya en [Store.enTransaccionDe], asi que una llamada suelta abre la
// suya y una llamada desde dentro de una unidad de trabajo -- que es lo que
// hace [aplicacion.Anomalias.Evaluar], para que las alertas y su asiento sean
// un solo hecho -- participa en la que ya hay. Sin esto, el asiento podria
// revertirse dejando las alertas escritas.
//
// Devuelve cuantas filas ENTRARON, que no es len(alertas): es lo unico que
// permite decir "esta pasada encontro tres anomalias que antes no estaban".
func (s *Store) GuardarAlertas(ctx context.Context, alertas []aplicacion.Alerta) (int, error) {
	if len(alertas) == 0 {
		return 0, nil
	}

	nuevas := 0
	err := s.enTransaccionDe(ctx, func(tx pgx.Tx) error {
		for _, a := range alertas {
			etiqueta, err := tx.Exec(ctx,
				`INSERT INTO alertas (periodo, tipo, ref_tipo, ref_id, ref_titular, detalle, detectada)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)
				 ON CONFLICT ON CONSTRAINT alerta_unica_por_hallazgo DO NOTHING`,
				a.Periodo, a.Tipo, a.RefTipo, a.RefID, a.RefTitular, a.Detalle, a.Detectada)
			if err != nil {
				return traducirError(err, "guardar la alerta %q sobre %s %q",
					a.Tipo, a.RefTipo, a.RefID)
			}
			nuevas += int(etiqueta.RowsAffected())
		}
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
// el ADR 0005 no admite de nada que alimente una decision.
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

	filas, err := s.ejecutorDe(ctx).Query(ctx,
		`SELECT `+columnasAlerta+` FROM alertas
		  WHERE ($1 = '' OR periodo = $1)
		    AND ($2 = '' OR tipo = $2)
		    AND (NOT $3 OR resuelta = $4)
		  ORDER BY detectada DESC, id`,
		f.Periodo, f.Tipo, filtraResueltas, resueltas)
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
