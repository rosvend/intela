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

// hasherDeForma es el doble del Hasher para estas pruebas. Solo contesta a la
// pregunta que este caso de uso le hace: "esto tiene forma de hash tuyo".
//
// Un doble y no cripto.Bcrypt porque lo que se prueba aqui es que el nucleo
// PREGUNTA y obedece; que la respuesta sea correcta para bcrypt de verdad lo
// prueba TestEsHashRechazaUnaClaveEnClaro, en el adaptador.
type hasherDeForma struct{ niega bool }

func (h hasherDeForma) Verificar(string, string) bool { return true }
func (h hasherDeForma) Hash(c string) (string, error) { return c, nil }
func (h hasherDeForma) EsHash(posible string) bool    { return !h.niega && posible == hashValido }

func TestPrimerAdministradorSiempreEsAdministrador(t *testing.T) {
	repo := &repoProvision{}
	p := Provision{Usuarios: repo, Claves: hasherDeForma{}}

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
	p := Provision{Usuarios: repo, Claves: hasherDeForma{}}

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
		"hash corto":   {"usr-admin", "admin@redes.co", "Admin", "$2a$10$corto"},
		// EL caso que dejaba produccion cerrada para siempre: una clave EN
		// CLARO de 20 caracteres o mas pasaba el control de longitud y el
		// CHECK del esquema, se guardaba tal cual, y el login fallaba despues
		// con la clave correcta. La longitud no era el control que hacia falta.
		"clave en claro larga": {
			"usr-admin", "admin@redes.co", "Admin",
			"esta-clave-tiene-mas-de-veinte-caracteres",
		},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			repo := &repoProvision{}
			p := Provision{Usuarios: repo, Claves: hasherDeForma{}}

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
	p := Provision{Usuarios: repo, Claves: hasherDeForma{}}

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
	p := Provision{Usuarios: repo, Claves: hasherDeForma{}}

	_, err := p.CrearPrimerAdministrador(context.Background(),
		"usr-admin", "admin@redes.co", "Admin", hashValido)
	// Centinela y no un error cualquiera: quien invoca tiene que poder
	// distinguir "ya estaba provisionada" -que no es un fallo- de "no se pudo
	// escribir".
	if !errors.Is(err, ErrYaHayUsuarios) {
		t.Fatalf("err = %v, se esperaba ErrYaHayUsuarios", err)
	}
}

// El nucleo no decide que es un hash: se lo pregunta al puerto y obedece. Sin
// esto, la regla volveria a ser una comprobacion de longitud escrita aqui, que
// es exactamente la que acepto una clave en claro.
func TestPrimerAdministradorPreguntaAlHasherPorLaForma(t *testing.T) {
	repo := &repoProvision{}
	p := Provision{Usuarios: repo, Claves: hasherDeForma{niega: true}}

	_, err := p.CrearPrimerAdministrador(context.Background(),
		"usr-admin", "admin@redes.co", "Admin", hashValido)
	if !errors.Is(err, ErrUsuarioInvalido) {
		t.Fatalf("err = %v, se esperaba ErrUsuarioInvalido cuando el Hasher niega la forma", err)
	}
	if repo.llamadas != 0 {
		t.Errorf("llamadas = %d, se esperaba 0: no se escribe un hash que el Hasher no reconoce", repo.llamadas)
	}
}

// Sin Hasher no se puede comprobar la forma, y dejar pasar el hash seria peor
// que fallar: es el camino que dejaba la instalacion cerrada para siempre.
func TestPrimerAdministradorSinHasherNoProvisiona(t *testing.T) {
	repo := &repoProvision{}
	_, err := Provision{Usuarios: repo}.CrearPrimerAdministrador(context.Background(),
		"usr-admin", "admin@redes.co", "Admin", hashValido)
	if !errors.Is(err, ErrUsuarioInvalido) {
		t.Fatalf("err = %v, se esperaba ErrUsuarioInvalido", err)
	}
	if repo.llamadas != 0 {
		t.Errorf("llamadas = %d, se esperaba 0", repo.llamadas)
	}
}

// Ningun mensaje lleva el VALOR del email. Estos errores se registran y ademas
// se devuelven desde la Lambda, asi que la plataforma los guarda otra vez.
func TestErroresDeProvisionNoLlevanElEmail(t *testing.T) {
	const email = "admin@redes.co"
	p := Provision{Usuarios: &repoProvision{err: ErrYaHayUsuarios}, Claves: hasherDeForma{}}

	_, err := p.CrearPrimerAdministrador(context.Background(), "usr-admin", email, "Admin", hashValido)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if strings.Contains(err.Error(), email) {
		t.Errorf("el error lleva el email: %q", err.Error())
	}

	// Y el de forma del email tampoco lo lleva, aunque sea el campo que falla.
	_, err = p.CrearPrimerAdministrador(context.Background(), "usr-admin", "admin.redes.co", "Admin", hashValido)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if strings.Contains(err.Error(), "admin.redes.co") {
		t.Errorf("el error lleva el email: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("el error deberia nombrar el campo: %q", err.Error())
	}
}
