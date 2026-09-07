package postgres

import (
	"context"

	"github.com/rosvend/intela/internal/aplicacion"
)

// CrearPrimerAdministrador inserta la cuenta inicial, y solo si no hay ninguna.
//
// # La guarda y la escritura son la misma sentencia
//
// `WHERE NOT EXISTS (SELECT 1 FROM usuarios)` va DENTRO del INSERT, no en una
// consulta previa. Con un recuento antes y un INSERT despues, entre las dos
// cabe otra invocacion, y el resultado son dos cuentas de administrador en una
// instalacion que solo pidio una. Aqui la condicion se evalua en la misma
// instantanea que la escritura.
//
// Lo que NO sirve de guarda: ni la clave primaria ni el UNIQUE del email. Las
// dos dejan pasar una segunda cuenta con otro id y otro correo, que es
// exactamente el caso que hay que impedir -- y es lo que fija
// TestCrearPrimerAdministradorEsDeUnaSolaVez.
//
// Y mira la tabla ENTERA, no las filas con rol administrador: una instalacion
// con titulares ya cargados no esta vacia, y abrir esta via ahi seria una
// forma de anadirse un administrador saltandose el alta normal.
//
// # Cero filas no es un fallo
//
// Si la tabla ya tenia alguien, el INSERT afecta a cero filas sin error. Eso
// se traduce a aplicacion.ErrYaHayUsuarios, que quien invoca puede distinguir
// de "no se pudo escribir": la operacion es de una sola vez y ya se hizo, asi
// que un reintento es inocuo y no tiene por que verse como un error.
//
// titular_id se queda NULL: lo pone el esquema por defecto y el CHECK
// titular_tiene_titular_id solo lo exige para el rol titular. Un administrador
// con titular_id se leeria como si representara a un socio.
func (s *Store) CrearPrimerAdministrador(ctx context.Context, u aplicacion.Usuario, hash string) error {
	etiqueta, err := s.pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
		 SELECT $1, $2, $3, $4, $5
		 WHERE NOT EXISTS (SELECT 1 FROM usuarios)`,
		u.ID, u.Email, u.Nombre, string(u.Rol), hash)
	if err != nil {
		return traducirError(err, "crear el primer administrador %q", u.Email)
	}
	if etiqueta.RowsAffected() == 0 {
		return aplicacion.ErrYaHayUsuarios
	}
	return nil
}
