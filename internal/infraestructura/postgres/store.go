// Package postgres adapta los puertos de persistencia contra PostgreSQL.
//
// Aqui solo esta la conexion. Las implementaciones de cada puerto entran en
// PRs propios, junto con los casos de uso que las usan.
//
// Cuando entren, cada una declara su asercion de compilacion al lado:
//
//	var _ aplicacion.RepositorioRepertorio = (*Store)(nil)
//
// Sin esas aserciones un desajuste entre puerto y adaptador no aparece hasta
// que se compila cmd/api, que es tarde y lejos.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
