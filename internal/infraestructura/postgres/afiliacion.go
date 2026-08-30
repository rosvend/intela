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
const columnasUsuario = `id, email, nombre, rol, COALESCE(titular_id, ''), password_hash`

// escanearUsuario devuelve el hash aparte, igual que el puerto.
func escanearUsuario(fila pgx.Row) (aplicacion.Usuario, string, error) {
	var (
		u    aplicacion.Usuario
		rol  string
		hash string
	)
	// rol se escanea a string y se convierte: pgx sabe desenvolver un
	// `type Rol string`, pero la autorizacion de cada caso de uso se decide
	// contra este campo y no conviene que dependa de que plan de escaneo elija
	// la libreria.
	err := fila.Scan(&u.ID, &u.Email, &u.Nombre, &rol, &u.TitularID, &hash)
	u.Rol = aplicacion.Rol(rol)
	return u, hash, err
}

// UsuarioPorEmail es el unico camino por el que sale un hash de contrasena.
//
// La busqueda es exacta sobre email. El esquema no tiene citext ni indice
// sobre lower(email), asi que el puerto no especifica plegado de mayusculas:
// si el formulario de #16 normaliza a minusculas, la decision se toma alli y
// se acompana de su migracion, no con un lower() silencioso aqui.
func (s *Store) UsuarioPorEmail(ctx context.Context, email string) (aplicacion.Usuario, string, error) {
	fila := s.pool.QueryRow(ctx,
		`SELECT `+columnasUsuario+` FROM usuarios WHERE email = $1`, email)

	u, hash, err := escanearUsuario(fila)
	if err != nil {
		return aplicacion.Usuario{}, "", traducirError(err, "usuario por email %q", email)
	}
	return u, hash, nil
}

// UsuarioPorID descarta el hash a proposito: el puerto solo lo entrega desde
// UsuarioPorEmail, que es el camino de la credencial. Aqui no hace falta y por
// tanto no sale.
func (s *Store) UsuarioPorID(ctx context.Context, id string) (aplicacion.Usuario, error) {
	fila := s.pool.QueryRow(ctx,
		`SELECT `+columnasUsuario+` FROM usuarios WHERE id = $1`, id)

	u, _, err := escanearUsuario(fila)
	if err != nil {
		return aplicacion.Usuario{}, traducirError(err, "usuario por id %q", id)
	}
	return u, nil
}
