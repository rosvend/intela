package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.ParametroEnFecha = (*Store)(nil)

// ErrParametroSinVigencia: ninguna fila de la clave cubre la fecha. Propio y no
// ErrNoEncontrado porque lo util es QUE clave falta y para cuando.
var ErrParametroSinVigencia = errors.New("parametro sin vigencia en la fecha")

// ParametroVigente resuelve el valor de una clave en una fecha.
//
// La restriccion EXCLUDE de `parametros` (00001_init.sql) garantiza que dos
// filas de la misma clave no se solapen en el tiempo, asi que esta consulta
// devuelve como mucho una fila. Sin esa garantia habria que decidir aqui cual
// de dos gana, y la decision dependeria del orden de lectura -- es decir, la
// misma corrida repetida podria repartir distinto.
//
// El rango es [vigente_desde, vigente_hasta): medio abierto, igual que el
// daterange '[)' de la restriccion. Cerrarlo por arriba haria que el ultimo dia
// de una vigencia tuviera dos valores validos, que es exactamente lo que el
// EXCLUDE impide crear.
//
// Un parametro ausente NO cae a cero. ADR 0004 modela lo ausente como ausente,
// y en este caso concreto un umbral de matching en cero asignaria la primera
// obra que se pareciera en algo a cualquier titulo.
func (s *Store) ParametroVigente(ctx context.Context, clave string, fecha time.Time) (decimal.Decimal, error) {
	var valor decimal.Decimal
	err := s.pool.QueryRow(ctx, `
		SELECT valor FROM parametros
		 WHERE clave = $1
		   AND vigente_desde <= $2
		   AND (vigente_hasta IS NULL OR vigente_hasta > $2)`,
		clave, fecha).Scan(&valor)

	if errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, fmt.Errorf("%q en %s: %w", clave, fecha.Format(time.DateOnly), ErrParametroSinVigencia)
	}
	if err != nil {
		return decimal.Zero, traducirError(err, "leer el parametro %q", clave)
	}
	return valor, nil
}
