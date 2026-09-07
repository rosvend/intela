package postgres

import (
	"errors"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// primerAdmin es el usuario que provisiona una instalacion vacia.
var primerAdmin = aplicacion.Usuario{
	ID:     "usr-bootstrap",
	Email:  "bootstrap@redes.co",
	Nombre: "Bootstrap",
	Rol:    aplicacion.RolAdministrador,
}

// vacia devuelve un Store contra una base migrada y SIN usuarios.
//
// No usa sembrar(): esa helper inserta dos usuarios, que es justo la
// precondicion que estas pruebas necesitan que NO se cumpla.
func vacia(t *testing.T) *Store {
	t.Helper()
	return &Store{pool: testhelp.Pool(t)}
}

func TestCrearPrimerAdministradorEnUnaBaseVacia(t *testing.T) {
	s := vacia(t)

	if err := s.CrearPrimerAdministrador(t.Context(), primerAdmin, hashBcrypt); err != nil {
		t.Fatalf("CrearPrimerAdministrador: %v", err)
	}

	// Se comprueba por el mismo camino que usa el login, no con un SELECT
	// propio: lo que importa no es que haya una fila, es que esa fila sirva
	// para entrar.
	u, hash, err := s.UsuarioPorEmail(t.Context(), primerAdmin.Email)
	if err != nil {
		t.Fatalf("UsuarioPorEmail tras provisionar: %v", err)
	}
	if u.ID != primerAdmin.ID {
		t.Errorf("ID = %q, se esperaba %q", u.ID, primerAdmin.ID)
	}
	if u.Rol != aplicacion.RolAdministrador {
		t.Errorf("Rol = %q, se esperaba %q", u.Rol, aplicacion.RolAdministrador)
	}
	if hash != hashBcrypt {
		t.Errorf("hash = %q, se esperaba el que se paso", hash)
	}
	// titular_id NULL -> "" por el COALESCE de columnasUsuario.
	if u.TitularID != "" {
		t.Errorf("TitularID = %q, se esperaba vacio", u.TitularID)
	}
}

func TestCrearPrimerAdministradorSeNiegaSiYaHayAlguien(t *testing.T) {
	// sembrar() deja dos usuarios: la instalacion ya esta provisionada.
	s, pool := sembrar(t)

	err := s.CrearPrimerAdministrador(t.Context(), primerAdmin, hashBcrypt)
	if !errors.Is(err, aplicacion.ErrYaHayUsuarios) {
		t.Fatalf("err = %v, se esperaba ErrYaHayUsuarios", err)
	}

	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM usuarios WHERE id = $1`, primerAdmin.ID).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Errorf("filas insertadas = %d, se esperaba 0: el rechazo no debe escribir", n)
	}
}

// La invariante es "exactamente uno, nunca dos", y la unica forma de que sea
// cierta es que la comprobacion y la escritura sean la MISMA sentencia. Esta
// prueba fija el efecto: la segunda invocacion no crea una cuenta mas.
func TestCrearPrimerAdministradorEsDeUnaSolaVez(t *testing.T) {
	s := vacia(t)

	if err := s.CrearPrimerAdministrador(t.Context(), primerAdmin, hashBcrypt); err != nil {
		t.Fatalf("primera invocacion: %v", err)
	}

	// Segundo intento con OTRO id y OTRO email: si la guarda fuera la clave
	// primaria o el UNIQUE del email en vez del "solo si esta vacia", este
	// pasaria y la instalacion acabaria con dos administradores.
	otro := aplicacion.Usuario{
		ID:     "usr-segundo",
		Email:  "segundo@redes.co",
		Nombre: "Segundo",
		Rol:    aplicacion.RolAdministrador,
	}
	err := s.CrearPrimerAdministrador(t.Context(), otro, hashBcrypt)
	if !errors.Is(err, aplicacion.ErrYaHayUsuarios) {
		t.Fatalf("segunda invocacion: err = %v, se esperaba ErrYaHayUsuarios", err)
	}

	var total int
	if err := s.pool.QueryRow(t.Context(), `SELECT count(*) FROM usuarios`).Scan(&total); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if total != 1 {
		t.Errorf("usuarios = %d, se esperaba 1", total)
	}
}

// Un titular tambien cuenta como "ya hay usuarios". La guarda mira la tabla
// entera y no el rol: si mirara solo administradores, una instalacion con
// titulares cargados aceptaria una cuenta de administrador nueva por esta via.
func TestCrearPrimerAdministradorCuentaCualquierRol(t *testing.T) {
	s := vacia(t)
	ctx := t.Context()

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO titulares (id, nombre, ipi, persona_natural, clase)
		 VALUES ('tit-x', 'Titular X', 'IPI-00000009', TRUE, 'socio')`); err != nil {
		t.Fatalf("sembrar titular: %v", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash)
		 VALUES ('usr-x', 'x@redes.co', 'X', 'titular', 'tit-x', $1)`, hashBcrypt); err != nil {
		t.Fatalf("sembrar usuario titular: %v", err)
	}

	err := s.CrearPrimerAdministrador(ctx, primerAdmin, hashBcrypt)
	if !errors.Is(err, aplicacion.ErrYaHayUsuarios) {
		t.Fatalf("err = %v, se esperaba ErrYaHayUsuarios", err)
	}
}

// La asercion de compilacion del puerto. Va aqui y no en provision.go para que
// el fichero de produccion no cargue con una declaracion que solo existe para
// que el compilador avise.
var _ aplicacion.RepositorioProvisionInicial = (*Store)(nil)
