package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

// traducirError lleva un error de pgx al vocabulario de aplicacion.
//
// pgx.ErrNoRows significa "la consulta fue bien y no hay fila", que es justo
// lo que nombra aplicacion.ErrNoEncontrado. Cualquier otro error -red,
// timeout, violacion de constraint- sube con su causa intacta: tragarse un
// fallo transitorio como si fuera "no hay fila" es el error caro que describe
// internal/aplicacion/errores.go.
//
// Se envuelve con contexto en vez de devolver el centinela pelado porque un
// "no encontrado" a secas no dice cual de las consultas fue. errors.Is y
// errors.As siguen funcionando: %w conserva la cadena.
//
// errors.Is y no ==: pgx.CollectOneRow y algunas rutas de Row.Scan devuelven
// pgx.ErrNoRows ya envuelto.
//
// Devuelve nil ante nil para que cada sitio de llamada pueda escribir
// `return u, traducirError(err, "...")` sin un if de por medio.
func traducirError(err error, formato string, args ...any) error {
	if err == nil {
		return nil
	}
	contexto := fmt.Sprintf(formato, args...)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", contexto, aplicacion.ErrNoEncontrado)
	}
	return fmt.Errorf("%s: %w", contexto, err)
}
