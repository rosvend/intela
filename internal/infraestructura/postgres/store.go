package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.UnidadDeTrabajo = (*Store)(nil)

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
	return &Store{pool: pool}, nil
}

// Ping comprueba la conexion. Lo usa el handler de salud.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Cerrar libera el pool. Espera a que terminen las consultas en vuelo.
func (s *Store) Cerrar() {
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

// claveTx es un tipo propio para no chocar con otras claves de contexto.
type claveTx struct{}

// querier es lo que comparten el pool y una transaccion. Los metodos que
// participan en UnidadDeTrabajo leen por q(ctx) y no por s.pool: si el caso
// de uso abrio una transaccion, entran en ella.
type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

func (s *Store) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(claveTx{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

// Ejecutar abre una transaccion y se la deja a fn en el contexto. Es la
// forma que tiene aplicacion de fijar el limite sin nombrar pgx.Tx.
func (s *Store) Ejecutar(ctx context.Context, fn func(context.Context) error) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, claveTx{}, tx))
	})
}
