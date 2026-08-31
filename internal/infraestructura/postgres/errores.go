package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rosvend/intela/internal/aplicacion"
)

// codigoUnicidadViolada es el SQLSTATE 23505, unique_violation.
//
// Literal y no una constante de una libreria de codigos: son cinco caracteres
// fijados por el estandar SQL desde hace decadas, y anadir una dependencia
// entera al modulo para nombrarlos no compra nada.
const codigoUnicidadViolada = "23505"

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

// esUnicidadViolada dice si err es una violacion de UNIQUE o de PRIMARY KEY.
//
// No vive dentro de traducirError, y es deliberado: "ya existe una fila igual"
// no significa lo mismo en todas las tablas. En `reportes` es la deteccion de
// duplicado por huella, que es una respuesta del negocio; en otra tabla puede
// ser un identificador mal generado, que es un fallo. Traducirlo a un unico
// centinela desde el traductor general convertiria el segundo caso en el
// primero sin que nadie lo notara.
//
// Asi que cada sitio de llamada decide: pregunta por esto ANTES de pasar por
// traducirError y pone el nombre que la violacion tiene en SU tabla.
//
// errors.As y no una asercion de tipo: pgx envuelve el error del servidor
// cuando llega desde un lote o desde una transaccion.
func esUnicidadViolada(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnicidadViolada
}
