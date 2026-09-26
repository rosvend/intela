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

// esClaveDuplicada dice si el error es una violacion de UNIQUE o de PRIMARY KEY.
//
// Sirve para que el adaptador traduzca "esta fila ya estaba" al vocabulario
// del nucleo en vez de dejarlo subir como un fallo cualquiera. La alternativa
// -un SELECT antes del INSERT- deja una ventana entre la consulta y la
// escritura por la que cabe otra peticion: la unica comprobacion de unicidad
// que no tiene carrera es la que hace la base.
//
// No vive dentro de traducirError, y es deliberado: "ya existe una fila igual"
// no significa lo mismo en todas las tablas. En `reportes` es la deteccion de
// duplicado por huella, que es una respuesta del negocio; en `obras` es un alta
// repetida; en la publicacion ONI es ErrYaPublicado (republicar reescribiria
// el ancla de R-19); en otra tabla puede ser un identificador mal generado,
// que si es un fallo. Traducirlo a un unico centinela desde el traductor
// general convertiria el ultimo caso en los primeros sin que nadie lo notara.
// Asi que cada sitio de llamada decide: pregunta por esto ANTES de pasar por
// traducirError y pone el nombre que la violacion tiene en SU tabla.
//
// errors.As y no una asercion de tipo: pgx envuelve el *pgconn.PgError cuando
// el error sale de un lote o de una transaccion.
func esClaveDuplicada(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnicidad
}

// codigoForanea es el SQLSTATE 23503, foreign_key_violation.
const codigoForanea = "23503"

// esClaveForanea dice si el error es una violacion de FOREIGN KEY.
//
// Misma logica que [esClaveDuplicada] y el mismo motivo para no vivir dentro
// de traducirError: una FK rota no significa lo mismo en todas las tablas -en
// `declaracion_versiones` es "esa obra no existe" (404); en la FK de
// `declaraciones` hacia `titulares` seria "ese titular no existe", que no es
// el mismo caso-. Cada sitio de llamada decide que centinela le corresponde a
// SU tabla.
func esClaveForanea(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoForanea
}

// esClaveForaneaDe es [esClaveForanea] pero para una tabla con MAS de una FK,
// donde "es una violacion de FK" no basta para saber cual: hace falta
// preguntar por el nombre de la restriccion.
//
// Ver [Store.Guardar] en declaraciones.go: `declaraciones` tiene la FK hacia
// `titulares` (declaraciones_titular_id_fkey) y, desde la migracion 00008,
// tambien hacia `declaracion_versiones` (declaraciones_version_fkey). Sin
// discriminar, un fallo en la segunda se traduciria como "titular inexistente"
// -que no es lo que paso- solo porque las dos comparten codigo SQLSTATE.
func esClaveForaneaDe(err error, restriccion string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoForanea && pgErr.ConstraintName == restriccion
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
	// Detail/Where de PgError suelen traer la pista que el Message omite: en
	// un COPY, "COPY usos, line N" vive en Where (o Detail). Sin esto, un
	// lote de miles de filas falla con "copiar el lote del reporte X" y nadie
	// sabe cual fila lo rompio.
	if pista := pistaPgError(err); pista != "" {
		return fmt.Errorf("%s (%s): %w", contexto, pista, err)
	}
	return fmt.Errorf("%s: %w", contexto, err)
}

// esConflictoUnico reconoce una violacion de UNIQUE (23505). El caso de uso
// la traduce a ErrConflicto: "ya hay una solicitud con ese correo" no es un
// 500.
func esConflictoUnico(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// pistaPgError junta Detail y Where no vacios de un *pgconn.PgError. Vacio si
// el error no es de Postgres o no trae ninguna de las dos.
func pistaPgError(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	switch {
	case pgErr.Detail != "" && pgErr.Where != "":
		return pgErr.Detail + "; " + pgErr.Where
	case pgErr.Detail != "":
		return pgErr.Detail
	case pgErr.Where != "":
		return pgErr.Where
	}
	return ""
}
