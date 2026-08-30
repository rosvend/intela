package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.RepositorioAfiliacion = (*Store)(nil)

// Las dos consultas comparten proyeccion para que no puedan divergir: si una
// crece una columna y la otra no, el escaneo compartido deja de cuadrar y
// falla al compilar, no en produccion.
//
// COALESCE en vez de un *string: titular_id es NULL para los roles que no son
// titular, y como referencia a titulares(id) nunca puede ser la cadena vacia.
// Asi que la cadena vacia de aplicacion.Usuario.TitularID significa
// exactamente lo mismo que NULL, sin meter un tipo nullable en el adaptador.
// columnasUsuario NO incluye password_hash: lo pide aparte quien lo necesita,
// que es una sola consulta.
//
// Cuando estaba dentro, las otras dos que comparten esta proyeccion
// -UsuarioPorID y la resolucion de sesion de sesiones.go- lo leian para
// tirarlo acto seguido. Es decir, sacaban una credencial de la base en CADA
// peticion autenticada, para nada, y de paso desmentian la frase que encabeza
// UsuarioPorEmail aqui abajo.
const columnasUsuario = `id, email, nombre, rol, COALESCE(titular_id, '')`

// columnasUsuarioConClave anade el hash. Se escribe a partir de la otra y no
// suelta para que las dos proyecciones no puedan divergir en el orden.
const columnasUsuarioConClave = columnasUsuario + `, password_hash`

// escanearUsuario lee columnasUsuario, y tambien el hash si se le pasa donde
// dejarlo.
//
// El puntero opcional en vez de dos funciones: el orden de escaneo queda
// definido UNA vez. Con dos funciones, cada columna nueva hay que anadirla en
// dos sitios, y el dia que solo se anada en uno el desajuste no se ve hasta que
// una consulta devuelve los campos corridos.
func escanearUsuario(fila pgx.Row, hash *string) (aplicacion.Usuario, error) {
	var (
		u   aplicacion.Usuario
		rol string
	)
	// rol se escanea a string y se convierte: pgx sabe desenvolver un
	// `type Rol string`, pero la autorizacion de cada caso de uso se decide
	// contra este campo y no conviene que dependa de que plan de escaneo elija
	// la libreria.
	destinos := []any{&u.ID, &u.Email, &u.Nombre, &rol, &u.TitularID}
	if hash != nil {
		destinos = append(destinos, hash)
	}
	err := fila.Scan(destinos...)
	u.Rol = aplicacion.Rol(rol)
	return u, err
}

// UsuarioPorEmail es el unico camino por el que sale un hash de contrasena.
//
// La busqueda es exacta sobre email. El esquema no tiene citext ni indice
// sobre lower(email), asi que el puerto no especifica plegado de mayusculas:
// si el formulario de #16 normaliza a minusculas, la decision se toma alli y
// se acompana de su migracion, no con un lower() silencioso aqui.
func (s *Store) UsuarioPorEmail(ctx context.Context, email string) (aplicacion.Usuario, string, error) {
	fila := s.pool.QueryRow(ctx,
		`SELECT `+columnasUsuarioConClave+` FROM usuarios WHERE email = $1`, email)

	var hash string
	u, err := escanearUsuario(fila, &hash)
	if err != nil {
		return aplicacion.Usuario{}, "", traducirError(err, "usuario por email %q", email)
	}
	return u, hash, nil
}

// UsuarioPorID ni siquiera pide el hash: el puerto solo lo entrega desde
// UsuarioPorEmail, que es el camino de la credencial. Antes lo leia y lo
// descartaba, que no es lo mismo -una credencial que no sale de la base no se
// puede filtrar por el camino-.
func (s *Store) UsuarioPorID(ctx context.Context, id string) (aplicacion.Usuario, error) {
	fila := s.pool.QueryRow(ctx,
		`SELECT `+columnasUsuario+` FROM usuarios WHERE id = $1`, id)

	u, err := escanearUsuario(fila, nil)
	if err != nil {
		return aplicacion.Usuario{}, traducirError(err, "usuario por id %q", id)
	}
	return u, nil
}
