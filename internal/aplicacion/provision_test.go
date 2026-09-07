package aplicacion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// repoProvision es el doble del puerto de provision inicial. Guarda lo que se
// le manda para que las pruebas afirmen sobre el Usuario COMPUESTO, que es lo
// unico que este caso de uso decide.
type repoProvision struct {
	recibido Usuario
	hash     string
	llamadas int
	err      error
}

func (r *repoProvision) CrearPrimerAdministrador(_ context.Context, u Usuario, hash string) error {
	r.llamadas++
	r.recibido = u
	r.hash = hash
	return r.err
}

const hashValido = "$2a$10$0123456789012345678901234567890123456789012345678901"

func TestPrimerAdministradorSiempreEsAdministrador(t *testing.T) {
	repo := &repoProvision{}
	p := Provision{Usuarios: repo}

	u, err := p.CrearPrimerAdministrador(context.Background(),
		"usr-admin", "admin@redes.co", "Admin", hashValido)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}

	// El rol NO es un parametro. Quien invoca esta superficie no puede pedir
	// un titular ni un auditor: es la unica cuenta que existe y tiene que
	// poder crear las demas.
	if u.Rol != RolAdministrador {
		t.Errorf("rol = %q, se esperaba %q", u.Rol, RolAdministrador)
	}
	if repo.recibido.Rol != RolAdministrador {
		t.Errorf("rol persistido = %q, se esperaba %q", repo.recibido.Rol, RolAdministrador)
	}
	// titular_id NULL: el CHECK titular_tiene_titular_id solo lo exige para
	// el rol titular, y un administrador con titular_id se leeria como si
	// representara a un socio.
	if repo.recibido.TitularID != "" {
		t.Errorf("titular_id = %q, se esperaba vacio", repo.recibido.TitularID)
	}
	if repo.hash != hashValido {
		t.Errorf("hash = %q, se esperaba el que se paso", repo.hash)
	}
}

func TestPrimerAdministradorRecortaLosBlancos(t *testing.T) {
	repo := &repoProvision{}
	p := Provision{Usuarios: repo}

	if _, err := p.CrearPrimerAdministrador(context.Background(),
		"  usr-admin  ", "  admin@redes.co  ", "  Admin  ", "  "+hashValido+"  "); err != nil {
		t.Fatalf("crear: %v", err)
	}

	// Misma razon que en la ingesta: si el recorte no ocurre una sola vez y
	// arriba, el email con espacios entra en la base y despues no casa con el
	// del login, que llega limpio del formulario.
	if repo.recibido.Email != "admin@redes.co" {
		t.Errorf("email = %q, se esperaba recortado", repo.recibido.Email)
	}
	if repo.recibido.ID != "usr-admin" {
		t.Errorf("id = %q, se esperaba recortado", repo.recibido.ID)
	}
	if repo.recibido.Nombre != "Admin" {
		t.Errorf("nombre = %q, se esperaba recortado", repo.recibido.Nombre)
	}
	if repo.hash != hashValido {
		t.Errorf("hash = %q, se esperaba recortado", repo.hash)
	}
}

func TestPrimerAdministradorExigeLosCuatroCampos(t *testing.T) {
	casos := map[string]struct{ id, email, nombre, hash string }{
		"sin id":       {"", "admin@redes.co", "Admin", hashValido},
		"sin email":    {"usr-admin", "", "Admin", hashValido},
		"sin nombre":   {"usr-admin", "admin@redes.co", "", hashValido},
		"sin hash":     {"usr-admin", "admin@redes.co", "Admin", ""},
		"id en blanco": {"   ", "admin@redes.co", "Admin", hashValido},
		"email sin @":  {"usr-admin", "admin.redes.co", "Admin", hashValido},
		// El CHECK del esquema exige length(password_hash) >= 20. Comprobarlo
		// aqui convierte un 23514 que aborta la invocacion en un error que
		// dice que campo esta mal.
		"hash corto": {"usr-admin", "admin@redes.co", "Admin", "$2a$10$corto"},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			repo := &repoProvision{}
			p := Provision{Usuarios: repo}

			_, err := p.CrearPrimerAdministrador(context.Background(), c.id, c.email, c.nombre, c.hash)
			if !errors.Is(err, ErrUsuarioInvalido) {
				t.Fatalf("err = %v, se esperaba ErrUsuarioInvalido", err)
			}
			// No se toca la base con una peticion mal formada.
			if repo.llamadas != 0 {
				t.Errorf("llamadas al repositorio = %d, se esperaba 0", repo.llamadas)
			}
		})
	}
}

func TestPrimerAdministradorNombraElCampoQueFalla(t *testing.T) {
	repo := &repoProvision{}
	p := Provision{Usuarios: repo}

	_, err := p.CrearPrimerAdministrador(context.Background(),
		"usr-admin", "admin.redes.co", "Admin", hashValido)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	// El mensaje tiene que decir QUE esta mal: esta superficie se invoca a
	// mano desde una terminal y el operador no tiene un formulario que le
	// marque el campo.
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("mensaje = %q, se esperaba que nombrara el email", err.Error())
	}
}

func TestPrimerAdministradorPropagaQueYaHabiaUsuarios(t *testing.T) {
	repo := &repoProvision{err: ErrYaHayUsuarios}
	p := Provision{Usuarios: repo}

	_, err := p.CrearPrimerAdministrador(context.Background(),
		"usr-admin", "admin@redes.co", "Admin", hashValido)
	// Centinela y no un error cualquiera: quien invoca tiene que poder
	// distinguir "ya estaba provisionada" -que no es un fallo- de "no se pudo
	// escribir".
	if !errors.Is(err, ErrYaHayUsuarios) {
		t.Fatalf("err = %v, se esperaba ErrYaHayUsuarios", err)
	}
}
