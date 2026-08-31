package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// enUnaHora evita el CHECK sesion_expira_despues (expira > creada).
//
// `creada` se rellena con el now() del servidor, asi que un `expira` calculado
// desde un reloj fijo en el pasado hace fallar el INSERT por una razon que no
// tiene nada que ver con lo que se esta probando. Se inserta siempre con un
// expira futuro; para probar la caducidad se mueve el `ahora` de PorToken, que
// para eso es un parametro del puerto.
//
// Truncate a microsegundos: PostgreSQL guarda TIMESTAMPTZ con esa resolucion y
// time.Time lleva nanosegundos, asi que lo que vuelve de la base puede ser
// hasta 999 ns anterior a lo que se mando. En las pruebas del borde exacto eso
// es la diferencia entre comparar dos instantes iguales y compararlos
// desiguales, es decir, entre una prueba y una moneda al aire.
func enUnaHora() time.Time {
	return time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
}

func TestSesionCrearYResolver(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Crear(ctx, "token-en-claro", usuarioAdmin, enUnaHora()); err != nil {
		t.Fatalf("Crear: %v", err)
	}

	u, err := s.PorToken(ctx, "token-en-claro", time.Now().UTC())
	if err != nil {
		t.Fatalf("PorToken: %v", err)
	}
	if u.ID != usuarioAdmin {
		t.Fatalf("ID = %q, se esperaba %q", u.ID, usuarioAdmin)
	}
	if u.Email != emailAdmin {
		t.Fatalf("Email = %q, se esperaba %q", u.Email, emailAdmin)
	}
	// PorToken devuelve el Usuario entero, no solo su id: el middleware lo
	// pone en el contexto y #17 decide el rol contra ese campo.
	if u.Rol != aplicacion.RolAdministrador {
		t.Fatalf("Rol = %q, se esperaba %q", u.Rol, aplicacion.RolAdministrador)
	}
}

// El criterio de aceptacion: "Session tokens stored hashed, not in plaintext".
//
// Se comprueba mirando la tabla, no confiando en el codigo: si alguien cambia
// el adaptador para guardar el token tal cual, esta prueba lo caza.
func TestSesionGuardaElTokenHasheado(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	const claro = "token-en-claro-que-no-debe-aparecer"
	if err := s.Crear(ctx, claro, usuarioAdmin, enUnaHora()); err != nil {
		t.Fatalf("Crear: %v", err)
	}

	var guardado string
	if err := pool.QueryRow(ctx, `SELECT token FROM sesiones`).Scan(&guardado); err != nil {
		t.Fatalf("leer la fila: %v", err)
	}

	if guardado == claro {
		t.Fatal("el token esta en claro en la base: quien lea la tabla se puede hacer pasar por el usuario")
	}
	if strings.Contains(guardado, claro) {
		t.Fatalf("el token en claro aparece dentro de lo guardado: %q", guardado)
	}
	// SHA-256 en hexadecimal: 64 caracteres.
	if len(guardado) != 64 {
		t.Fatalf("lo guardado mide %d caracteres, se esperaban 64 de un SHA-256 en hex: %q",
			len(guardado), guardado)
	}
}

// El caso que da nombre al criterio: "expired tokens resolve to
// ErrNoEncontrado".
//
// Sin sleep y sin tocar el reloj del adaptador: se inserta con expira futuro
// -lo exige el CHECK- y se pregunta con un `ahora` posterior.
func TestSesionCaducadaEsNoEncontrada(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	expira := enUnaHora()
	if err := s.Crear(ctx, "token", usuarioAdmin, expira); err != nil {
		t.Fatalf("Crear: %v", err)
	}

	_, err := s.PorToken(ctx, "token", expira.Add(time.Second))
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("un token caducado tiene que ser ErrNoEncontrado, se obtuvo %v", err)
	}
}

// El limite exacto. `expira` es el primer instante en que la sesion ya NO
// vale: la comparacion es estricta.
func TestSesionEnElInstanteDeCaducidadYaNoVale(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	expira := enUnaHora()
	if err := s.Crear(ctx, "token", usuarioAdmin, expira); err != nil {
		t.Fatalf("Crear: %v", err)
	}

	if _, err := s.PorToken(ctx, "token", expira); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("justo en expira la sesion ya no vale, se obtuvo %v", err)
	}
}

func TestSesionDesconocidaEsNoEncontrada(t *testing.T) {
	s, _ := sembrar(t)

	_, err := s.PorToken(t.Context(), "jamas-emitido", time.Now().UTC())
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestSesionRevocada(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Crear(ctx, "token", usuarioAdmin, enUnaHora()); err != nil {
		t.Fatalf("Crear: %v", err)
	}
	if err := s.Revocar(ctx, "token"); err != nil {
		t.Fatalf("Revocar: %v", err)
	}

	_, err := s.PorToken(ctx, "token", time.Now().UTC())
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("un token revocado tiene que ser ErrNoEncontrado, se obtuvo %v", err)
	}
}

// Revocar dos veces, o revocar algo que nunca existio, no es un error: el
// logout es idempotente y decir "ese token no existia" filtra informacion.
func TestRevocarEsIdempotente(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Crear(ctx, "token", usuarioAdmin, enUnaHora()); err != nil {
		t.Fatalf("Crear: %v", err)
	}
	if err := s.Revocar(ctx, "token"); err != nil {
		t.Fatalf("primera revocacion: %v", err)
	}
	if err := s.Revocar(ctx, "token"); err != nil {
		t.Fatalf("revocar dos veces no es un error: %v", err)
	}
	if err := s.Revocar(ctx, "jamas-emitido"); err != nil {
		t.Fatalf("revocar un token inexistente no es un error: %v", err)
	}
}

// Una sesion que nace caducada no se guarda, y el error NO es ErrNoEncontrado:
// es una violacion del CHECK sesion_expira_despues, y sube con su causa.
//
// Deja constancia de la trampa: `creada` es el now() del servidor, asi que un
// expira en el pasado revienta el INSERT. Quien escriba la siguiente prueba con
// un reloj fijo antiguo va a ver este error y sabra por que.
func TestCrearRechazaUnaSesionYaCaducada(t *testing.T) {
	s, _ := sembrar(t)

	err := s.Crear(t.Context(), "token", usuarioAdmin, time.Now().UTC().Add(-time.Hour))
	if err == nil {
		t.Fatal("el CHECK sesion_expira_despues tenia que rechazar el INSERT")
	}
	if errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("una violacion de constraint no es 'no encontrado': %v", err)
	}
}

// Un token que apunta a un usuario que no existe tampoco se guarda: la clave
// ajena lo impide. Y tampoco puede pasar por "no encontrado".
func TestCrearConUsuarioInexistenteNoEsNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)

	err := s.Crear(t.Context(), "token", "usr-que-no-existe", enUnaHora())
	if err == nil {
		t.Fatal("la clave ajena tenia que rechazar el INSERT")
	}
	if errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("una violacion de clave ajena no es 'no encontrado': %v", err)
	}
}

// El error de una sesion nunca puede llevar el token dentro.
//
// Los errores acaban en los logs, y el resumen SHA-256 de la tabla existe justo
// para que un volcado no entregue credenciales vivas; filtrarlas por el log
// deshace ese trabajo. El patron de afiliacion.go interpola el argumento de
// busqueda en el mensaje -"usuario por email %q"-, asi que copiarlo aqui sin
// pensar es exactamente como se cuela.
func TestLosErroresDeSesionNoLlevanElToken(t *testing.T) {
	s, _ := sembrar(t)
	const token = "token-secretisimo-que-no-debe-aparecer"

	_, err := s.PorToken(t.Context(), token, time.Now().UTC())
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("el token aparece en el mensaje de error, y de ahi va al log: %v", err)
	}
}

// Dos sesiones del mismo usuario conviven: alguien con el portatil y el movil
// no deberia echarse a si mismo al entrar por el segundo.
func TestVariasSesionesPorUsuario(t *testing.T) {
	s, _ := sembrar(t)
	ctx := t.Context()

	if err := s.Crear(ctx, "portatil", usuarioAdmin, enUnaHora()); err != nil {
		t.Fatalf("Crear portatil: %v", err)
	}
	if err := s.Crear(ctx, "movil", usuarioAdmin, enUnaHora()); err != nil {
		t.Fatalf("Crear movil: %v", err)
	}

	if err := s.Revocar(ctx, "movil"); err != nil {
		t.Fatalf("Revocar: %v", err)
	}
	if _, err := s.PorToken(ctx, "portatil", time.Now().UTC()); err != nil {
		t.Fatalf("cerrar sesion en el movil no puede cerrar la del portatil: %v", err)
	}
}
