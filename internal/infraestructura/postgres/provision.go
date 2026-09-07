package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

// claveProvisionInicial es el identificador del advisory lock.
//
// Un numero cualquiera, pero FIJO: lo que importa es que todo el que provisione
// pida el mismo. Vive aqui, junto a su unico uso, para que no se pueda cambiar
// en un sitio y no en el otro.
const claveProvisionInicial = 0x1E7E1A_9204

// CrearPrimerAdministrador inserta la cuenta inicial, y solo si no hay ninguna.
//
// # El WHERE NOT EXISTS por si solo NO es exclusion mutua
//
// Era el error de la primera version, y es un error sobre PostgreSQL, no sobre
// SQL: bajo READ COMMITTED -- el nivel por defecto, y el que usa este pool --
// la subconsulta del NOT EXISTS NO ve las filas insertadas y aun no confirmadas
// por otra transaccion. Dos invocaciones simultaneas contra una tabla vacia ven
// las dos una tabla vacia, y las dos insertan.
//
// Medido, con cuatro conexiones independientes y una barrera: dos ganadoras y
// dos filas. Ni la clave primaria ni el UNIQUE del email lo frenan, porque una
// segunda cuenta llega con otro id y otro correo.
//
// # pg_advisory_xact_lock, y no SERIALIZABLE ni LOCK TABLE
//
// El lock se pide DENTRO de la transaccion, antes del INSERT, asi que la
// segunda invocacion espera, y cuando entra el NOT EXISTS ya ve la fila
// confirmada de la primera: inserta cero filas y recibe ErrYaHayUsuarios. Es
// exactamente la semantica que hace falta, sin nada mas encima.
//
//   - SERIALIZABLE haria fallar a la perdedora con 40001, que hay que traducir
//     y que invita a un reintento; aqui no hay nada que reintentar.
//   - LOCK TABLE usuarios IN EXCLUSIVE MODE tambien sirve, pero bloquea a
//     cualquier otro escritor de `usuarios` -- hoy la semilla -- por una
//     operacion que no tiene nada que ver con ellos.
//
// El lock es de transaccion (`_xact_`), asi que lo suelta el COMMIT o el
// ROLLBACK. No hay forma de olvidarse de liberarlo.
//
// # Lo que NO sirve de guarda
//
// Ni la PK ni el UNIQUE del email: las dos dejan pasar una segunda cuenta con
// otro id y otro correo, que es justo el caso a impedir. Y la condicion mira la
// tabla ENTERA, no las filas con rol administrador: una instalacion con
// titulares ya cargados no esta vacia, y abrir esta via ahi seria una forma de
// anadirse un administrador saltandose el alta normal.
//
// # Cero filas no es un fallo
//
// Si la tabla ya tenia alguien, el INSERT afecta a cero filas sin error. Eso se
// traduce a aplicacion.ErrYaHayUsuarios, que quien invoca puede distinguir de
// "no se pudo escribir": la operacion es de una sola vez y ya se hizo, asi que
// un reintento es inocuo y no tiene por que verse como un error.
//
// titular_id se queda NULL: lo pone el esquema por defecto y el CHECK
// titular_tiene_titular_id solo lo exige para el rol titular. Un administrador
// con titular_id se leeria como si representara a un socio.
func (s *Store) CrearPrimerAdministrador(ctx context.Context, u aplicacion.Usuario, hash string) error {
	return s.EnTransaccion(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, claveProvisionInicial); err != nil {
			return traducirError(err, "tomar el lock de provision inicial")
		}

		etiqueta, err := tx.Exec(ctx,
			`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
			 SELECT $1, $2, $3, $4, $5
			 WHERE NOT EXISTS (SELECT 1 FROM usuarios)`,
			u.ID, u.Email, u.Nombre, string(u.Rol), hash)
		if err != nil {
			// Sin el email en el mensaje: este error se registra y ademas se
			// devuelve desde la Lambda, que la plataforma vuelve a guardar.
			return traducirError(err, "crear el primer administrador")
		}
		if etiqueta.RowsAffected() == 0 {
			return aplicacion.ErrYaHayUsuarios
		}
		return nil
	})
}
