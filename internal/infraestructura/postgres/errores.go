package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rosvend/intela/internal/aplicacion"
)

// codigoUnicidad es el SQLSTATE 23505, unique_violation.
//
// El numero esta en el estandar y PostgreSQL lo respeta; el TEXTO del mensaje
// no, cambia con la version y con el idioma del servidor. Reconocer un
// duplicado por substring del mensaje funciona hasta que alguien despliega con
// otro locale.
const codigoUnicidad = "23505"

// esClaveDuplicada dice si el error es una violacion de clave unica.
//
// Sirve para que el adaptador traduzca "esta fila ya estaba" al vocabulario
// del nucleo en vez de dejarlo subir como un fallo cualquiera. La alternativa
// -un SELECT antes del INSERT- deja una ventana entre la consulta y la
// escritura por la que cabe otra peticion: la unica comprobacion de unicidad
// que no tiene carrera es la que hace la base.
//
// errors.As y no una asercion de tipo: pgx envuelve el *pgconn.PgError cuando
// el error sale de una operacion por lotes.
func esClaveDuplicada(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnicidad
}

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
