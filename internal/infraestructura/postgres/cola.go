package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.ColaTrabajos = (*Store)(nil)

// columnasTrabajo es la proyeccion que devuelve Tomar. Compartida con nada
// mas por ahora, pero declarada aparte por la misma razon que columnasUsuario:
// el orden de escaneo se define una vez.
const columnasTrabajo = `id, tipo, periodo, corrida, payload, intentos`

// Encolar inserta el trabajo, o no hace nada si ya estaba.
//
// El no-op es un ON CONFLICT DO NOTHING contra la restriccion
// `cola_clave_natural` (tipo, periodo, corrida). Es la base de datos la que
// arbitra, no una lectura previa en Go: entre un SELECT que no encuentra nada
// y el INSERT que le sigue caben dos schedulers, y el resultado de esa carrera
// son dos corridas del mismo periodo. Lo que en este dominio significa pagar
// dos veces.
//
// Devuelve false, sin error, cuando ya estaba: reintentar un encolado es
// normal -el scheduler lo hace cada vez que no llego a marcar el periodo como
// disparado- y no es un fallo del que haya que informar.
//
// La clave se valida antes de tocar la base. Los CHECK de la tabla dicen lo
// mismo, pero el error que devuelven nombra una restriccion de PostgreSQL y no
// el campo que venia mal.
func (s *Store) Encolar(ctx context.Context, clave aplicacion.ClaveTrabajo, payload []byte) (bool, error) {
	if err := clave.Valida(); err != nil {
		return false, fmt.Errorf("encolar: %w", err)
	}
	// La columna es JSONB NOT NULL: un payload vacio es el objeto vacio, no
	// NULL y no la cadena vacia, que no es JSON valido.
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}

	etiqueta, err := s.pool.Exec(ctx,
		`INSERT INTO cola_trabajos (tipo, periodo, corrida, payload)
		      VALUES ($1, $2, $3, $4)
		 ON CONFLICT ON CONSTRAINT cola_clave_natural DO NOTHING`,
		string(clave.Tipo), clave.Periodo, clave.Corrida, payload)
	if err != nil {
		return false, traducirError(err, "encolar %s", clave)
	}
	return etiqueta.RowsAffected() == 1, nil
}

// Tomar reclama un trabajo en exclusiva.
//
// # Por que FOR UPDATE SKIP LOCKED
//
// Es lo que hace que dos workers no procesen el mismo trabajo. El SELECT toma
// el cerrojo de la fila candidata; el que llegue segundo, en vez de esperar a
// que se libere, la SALTA y se lleva la siguiente. Sin SKIP LOCKED los workers
// se serializan sobre la misma fila y el paralelismo desaparece; sin
// FOR UPDATE los dos leen la misma fila pendiente y los dos la procesan.
//
// # Por que una sola sentencia y no una transaccion explicita
//
// El CTE toma el cerrojo y el UPDATE que lo envuelve cambia el estado a
// `en_curso` dentro de la MISMA sentencia, que es su propia transaccion. Para
// cuando el cerrojo se suelta, la fila ya no es `pendiente` y ningun otro
// worker la ve. Envolverlo en Store.EnTransaccion no anadiria garantia y si
// mantendria el cerrojo abierto mientras corre el manejador.
//
// # El orden
//
// Por disponible_en y luego por id: primero lo que lleva mas tiempo esperando,
// y el id desempata para que la cola sea FIFO de verdad y no dependa del plan
// que elija el motor. Es el mismo indice parcial `cola_pendientes`.
//
// # ErrSinTrabajo no es ErrNoEncontrado
//
// Cola vacia es la condicion NORMAL de un worker: pasa en todos los tics menos
// uno al ano. Por eso pgx.ErrNoRows se intercepta aqui y no pasa por
// traducirError, que lo convertiria en ErrNoEncontrado -y el bucle del worker
// registraria un error en cada tic.
func (s *Store) Tomar(ctx context.Context, ahora time.Time) (aplicacion.Trabajo, error) {
	fila := s.pool.QueryRow(ctx,
		`WITH candidato AS (
		   SELECT id
		     FROM cola_trabajos
		    WHERE estado = 'pendiente' AND disponible_en <= $1
		    ORDER BY disponible_en, id
		    LIMIT 1
		    FOR UPDATE SKIP LOCKED
		 )
		 UPDATE cola_trabajos c
		    SET estado = 'en_curso', intentos = c.intentos + 1
		   FROM candidato
		  WHERE c.id = candidato.id
		 RETURNING c.`+columnasTrabajo, ahora)

	var (
		t    aplicacion.Trabajo
		tipo string
	)
	err := fila.Scan(&t.ID, &tipo, &t.Clave.Periodo, &t.Clave.Corrida, &t.Payload, &t.Intentos)
	if errors.Is(err, pgx.ErrNoRows) {
		return aplicacion.Trabajo{}, aplicacion.ErrSinTrabajo
	}
	if err != nil {
		return aplicacion.Trabajo{}, traducirError(err, "tomar trabajo")
	}
	t.Clave.Tipo = aplicacion.TipoTrabajo(tipo)
	return t, nil
}

// Cerrar termina un trabajo en curso, en cualquiera de las tres formas que
// distingue aplicacion.Cierre.
//
// El WHERE exige `estado = 'en_curso'`. Sin esa condicion, un cierre repetido
// -o un cierre que llega tarde, despues de que otro camino reprogramara el
// trabajo- reescribiria en silencio un estado que ya no le pertenece: dejaria
// `hecho` un trabajo que estaba reprogramado, y la corrida no se ejecutaria.
// Con ella, ese cierre no afecta a ninguna fila y sale como ErrNoEncontrado.
//
// disponible_en solo se mueve cuando hay reintento; en los otros dos casos el
// COALESCE la deja como estaba. Guardarla intacta es lo que permite ver, en la
// fila de un trabajo abandonado, cuando se intento por ultima vez.
func (s *Store) Cerrar(ctx context.Context, id int64, c aplicacion.Cierre) error {
	var (
		estado string
		volver *time.Time
	)
	switch {
	case c.Reintentado():
		estado = "pendiente"
		cuando := c.Volver
		volver = &cuando
	case c.Exitoso():
		estado = "hecho"
	default:
		estado = "fallido"
	}

	etiqueta, err := s.pool.Exec(ctx,
		`UPDATE cola_trabajos
		    SET estado = $2,
		        error = $3,
		        disponible_en = COALESCE($4::timestamptz, disponible_en)
		  WHERE id = $1 AND estado = 'en_curso'`,
		id, estado, c.Motivo, volver)
	if err != nil {
		return traducirError(err, "cerrar trabajo %d", id)
	}
	if etiqueta.RowsAffected() == 0 {
		return fmt.Errorf("cerrar trabajo %d: no estaba en curso: %w", id, aplicacion.ErrNoEncontrado)
	}
	return nil
}
