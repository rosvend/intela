package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
)

// El limite de transaccion tambien es un puerto desde la #91: ver EnUnidad,
// al final de este fichero.
var _ aplicacion.UnidadDeTrabajo = (*Store)(nil)

// ejecutor es la parte comun entre *pgxpool.Pool y pgx.Tx que necesita un
// repositorio que a veces corre suelto y a veces DENTRO de la transaccion de
// otro puerto -ver [asentar] en bitacora.go-. Las dos implementaciones
// cumplen esta firma sin adaptador de por medio.
type ejecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store es el adaptador de PostgreSQL. Un solo tipo puede satisfacer varios
// puertos; lo que importa es que cada caso de uso declare solo el que usa.
type Store struct {
	pool *pgxpool.Pool
}

// Abrir conecta y comprueba que la base responde.
func Abrir(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("dsn invalido: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("abrir pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return Nuevo(pool), nil
}

// Nuevo envuelve un pool ya abierto. Lo usan las pruebas y cmd/seed, que
// llegan con el pool de testhelp o con uno que acabamos de pinguear.
func Nuevo(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool expone el pool. cmd/seed escribe con SQL directo las tablas que no
// tienen adaptador de escritura -`titulares`, `usuarios` y `parametros`-; el
// Store sigue siendo el dueno de la conexion.
//
// Decia "las tablas cuyo puerto todavia es de solo lectura -RepositorioRepertorio
// y ParametrosNormativos-", y de ParametrosNormativos dejo de ser cierto en la
// #118: [Store.SnapshotEnFecha] ESCRIBE. Lo que escribe es
// `snapshots_parametros` -- el corte congelado que una corrida consume --, no
// `parametros`, que sigue sin puerto de escritura y por eso sigue siendo del
// seed. La distincion importa: cargar una vigencia nueva es un hecho normativo
// que necesita su asiento (ADR 0006), y congelar un corte no lo es, porque no
// decide nada, solo deja constancia de lo que ya regia.
//
// `bolsas` y `usuarios_recaudo` YA tienen adaptador de escritura desde la #27
// ([Store.RegistrarBolsa], [Store.RegistrarUsuario]) y aun asi el seed las
// escribe por aqui. No es un olvido: esos dos metodos asientan en bitacora en
// la misma transaccion (ADR 0006) y el seed tiene que terminar con la bitacora
// vacia, porque `semilla.vaciar` se niega a recargar con SEED_RESET si hay un
// solo asiento. Esta escrito tambien en semilla/cargar.go, donde se decide.
//
// Las `obras` NO son de esas: tienen adaptador de escritura -[Store.Registrar],
// que mete la obra y sus coautores en una transaccion- y el seed pasa por el,
// no por aqui. Una obra escrita con SQL directo se queda sin coautores y
// entonces no la puede leer nadie, porque la lectura la reconstruye con el
// mismo constructor del dominio que la crea.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Ping comprueba la conexion. Lo usa el handler de salud.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// CerrarPool libera el pool. Espera a que terminen las consultas en vuelo.
//
// Se llamaba Cerrar. El nombre lo ocupa ahora aplicacion.ColaTrabajos.Cerrar,
// que cierra un TRABAJO y que este mismo tipo satisface: dos metodos con el
// mismo nombre no caben en un tipo, y de los dos el que tenia que ceder era
// este. "Cerrar" a secas sobre un adaptador que ya no es solo persistencia no
// dice cual de las dos cosas cierra.
func (s *Store) CerrarPool() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// EnTransaccion abre una transaccion, se la pasa a fn y confirma si fn
// termina bien. Si fn devuelve error o entra en panico, revierte.
//
// El limite lo fija quien llama, en aplicacion: el adaptador no decide donde
// empieza ni donde acaba una transaccion. Un caso de uso que escribe en dos
// tablas -un asiento y la fila que el asiento explica- las quiere dentro de
// la misma, y solo el sabe cuales son.
//
// La pgx.Tx viaja como PARAMETRO, nunca como campo de Store. *Store es un
// singleton del proceso, cableado una vez en cmd/api: un campo con la
// transaccion en curso dejaria que dos casos de uso concurrentes se pisaran
// el uno al otro, y el fallo no seria un panico sino una cifra distinta.
//
// El error de fn sube sin envolver: quien llama distingue sus propios
// centinelas.
//
// EnTransaccion abre SIEMPRE una transaccion nueva contra el pool, incluso si
// el contexto ya lleva la de una unidad de trabajo en curso: no mira el
// contexto. Por eso, dentro de una unidad no se usa este metodo sino
// [Store.EnUnidad] -- que es reentrante -- o [Store.enTransaccionDe], que
// participa en la transaccion que el contexto ya trae y solo abre una propia
// cuando no hay ninguna. Llamar aqui desde dentro de una unidad confirmaria
// por separado lo que la unidad todavia podria revertir, y ademas pediria una
// segunda conexion del pool.
func (s *Store) EnTransaccion(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transaccion: %w", err)
	}
	// Rollback tras un Commit correcto devuelve ErrTxClosed y no hace nada;
	// por eso el defer puede ser incondicional. Cubre tambien el panico, que
	// sin esto dejaria la transaccion abierta reteniendo cerrojos.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar transaccion: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// La unidad de trabajo
//
// [Store.EnTransaccion] resuelve el caso de UN puerto que escribe en varias
// tablas: el limite cabe entero dentro del metodo. No resuelve el otro, que es
// el que tiene el catalogo desde la #91: las escrituras de dos puertos
// distintos -- [aplicacion.CatalogoObras] y [aplicacion.BitacoraAuditoria] --
// son un solo hecho, y el limite lo declara el caso de uso que sostiene los
// dos.
//
// La transaccion viaja en el CONTEXTO y no en un parametro porque el puerto
// que la declara vive en `aplicacion`, donde depguard deniega `pgx`: una firma
// con pgx.Tx no se puede ni escribir alli. Y no en un campo de Store, que es
// lo que doc.go dice que no se va a hacer y sigue sin hacerse: *Store es un
// singleton del proceso y un campo mutable dejaria que dos peticiones
// concurrentes se pisaran la transaccion. Un contexto es de la llamada, como
// el parametro que sustituye.

// claveTx es la clave del contexto. Tipo propio y sin exportar: nadie fuera de
// este paquete puede leer ni poner la transaccion, que es lo que impide que
// alguien la saque de aqui y la pasee por el nucleo.
type claveTx struct{}

func conTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, claveTx{}, tx)
}

func txDe(ctx context.Context) (pgx.Tx, bool) {
	tx, hay := ctx.Value(claveTx{}).(pgx.Tx)
	return tx, hay
}

// EnUnidad satisface [aplicacion.UnidadDeTrabajo]: abre una transaccion y se
// la pasa a fn dentro del contexto, para que los metodos que fn invoque por
// CUALQUIER puerto de este mismo Store escriban en ella.
//
// Es reentrante: una unidad abierta dentro de otra es la misma unidad, no una
// anidada. Sin esto, un caso de uso que llame a otro abriria una segunda
// transaccion desde el mismo pool, y la de dentro esperaria un cerrojo que
// solo suelta la de fuera al confirmar -- un interbloqueo, no un error --.
//
// Solo participan los metodos que piden su ejecutor con [Store.ejecutorDe];
// ver la nota de doc.go.
func (s *Store) EnUnidad(ctx context.Context, fn func(context.Context) error) error {
	if _, dentro := txDe(ctx); dentro {
		return fn(ctx)
	}
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		return fn(conTx(ctx, tx))
	})
}

// ejecutorDe devuelve la transaccion que viaje en ctx, o el pool si no hay
// ninguna. Es lo que hace que el MISMO metodo sirva suelto y dentro de una
// unidad sin duplicar el SQL ni exponer dos firmas.
func (s *Store) ejecutorDe(ctx context.Context) ejecutor {
	if tx, hay := txDe(ctx); hay {
		return tx
	}
	return s.pool
}

// enTransaccionDe es [Store.EnTransaccion] para un metodo que ya podia correr
// dentro de la unidad de otro: si ctx trae transaccion, corre fn con ELLA y no
// confirma -- confirmar aqui cerraria la unidad de quien la abrio, y un error
// posterior ya no podria revertir esta escritura --. Si no la trae, se
// comporta como siempre.
func (s *Store) enTransaccionDe(ctx context.Context, fn func(pgx.Tx) error) error {
	if tx, hay := txDe(ctx); hay {
		return fn(tx)
	}
	return s.EnTransaccion(ctx, fn)
}
