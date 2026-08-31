package aplicacion

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Dobles de los puertos. Stdlib y nada mas, como el resto del repositorio.
//
// Estan aqui y no en un paquete aparte porque son la forma de este caso de
// uso: en cuanto otro los necesite se sacan, no antes.

type repoUsuarios struct {
	usuario Usuario
	hash    string
	err     error
}

func (r repoUsuarios) UsuarioPorEmail(context.Context, string) (Usuario, string, error) {
	return r.usuario, r.hash, r.err
}

func (r repoUsuarios) UsuarioPorID(context.Context, string) (Usuario, error) {
	return r.usuario, r.err
}

// hasherEspia cuenta las llamadas a Verificar. El contador es lo que permite
// comprobar sin cronometro que el login no tiene canal lateral de tiempo.
type hasherEspia struct {
	valido       bool
	verificaunas int
	ultimoHash   string
}

func (h *hasherEspia) Verificar(hash, _ string) bool {
	h.verificaunas++
	h.ultimoHash = hash
	return h.valido
}

func (h *hasherEspia) Hash(clave string) (string, error) { return clave, nil }

type sesionesMemoria struct {
	guardadas map[string]string
	expira    map[string]time.Time
	usuario   Usuario
	errCrear  error
	errPor    error
	revocados []string
}

func nuevasSesiones() *sesionesMemoria {
	return &sesionesMemoria{
		guardadas: map[string]string{},
		expira:    map[string]time.Time{},
	}
}

func (s *sesionesMemoria) Crear(_ context.Context, token, usuarioID string, expira time.Time) error {
	if s.errCrear != nil {
		return s.errCrear
	}
	s.guardadas[token] = usuarioID
	s.expira[token] = expira
	return nil
}

func (s *sesionesMemoria) PorToken(_ context.Context, token string, ahora time.Time) (Usuario, error) {
	if s.errPor != nil {
		return Usuario{}, s.errPor
	}
	if _, hay := s.guardadas[token]; !hay {
		return Usuario{}, ErrNoEncontrado
	}
	if !s.expira[token].After(ahora) {
		return Usuario{}, ErrNoEncontrado
	}
	return s.usuario, nil
}

func (s *sesionesMemoria) Revocar(_ context.Context, token string) error {
	s.revocados = append(s.revocados, token)
	delete(s.guardadas, token)
	return nil
}

type tokensFijos struct {
	valor string
	err   error
}

func (t tokensFijos) Generar() (string, error) { return t.valor, t.err }

type relojFijo struct{ instante time.Time }

func (r relojFijo) Ahora() time.Time { return r.instante }

// ---------------------------------------------------------------------------

var momento = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func nuevaAutenticacion(repo repoUsuarios, h *hasherEspia, s *sesionesMemoria) Autenticacion {
	return Autenticacion{
		Usuarios: repo,
		Claves:   h,
		Sesiones: s,
		Reloj:    relojFijo{instante: momento},
		Tokens:   tokensFijos{valor: "token-de-prueba"},
		TTL:      12 * time.Hour,
	}
}

func TestIniciarSesionDevuelveTokenConCredencialesValidas(t *testing.T) {
	repo := repoUsuarios{
		usuario: Usuario{ID: "usr-1", Email: "ana@redes.co", Rol: RolTitular},
		hash:    "$2a$10$hash-almacenado",
	}
	h := &hasherEspia{valido: true}
	s := nuevasSesiones()

	sesion, err := nuevaAutenticacion(repo, h, s).IniciarSesion(
		context.Background(), "ana@redes.co", "la-clave")
	if err != nil {
		t.Fatalf("IniciarSesion: %v", err)
	}

	if sesion.Token != "token-de-prueba" {
		t.Fatalf("Token = %q, se esperaba el del generador", sesion.Token)
	}
	if sesion.Usuario.ID != "usr-1" {
		t.Fatalf("Usuario.ID = %q", sesion.Usuario.ID)
	}
	// El TTL sale del reloj inyectado, no de time.Now(): sin eso la caducidad
	// no se puede probar sin esperar doce horas.
	if quiero := momento.Add(12 * time.Hour); !sesion.Expira.Equal(quiero) {
		t.Fatalf("Expira = %v, se esperaba %v", sesion.Expira, quiero)
	}
	if s.guardadas["token-de-prueba"] != "usr-1" {
		t.Fatalf("la sesion no quedo guardada: %v", s.guardadas)
	}
	// El hash que se verifica es el que vino del repositorio, no otro.
	if h.ultimoHash != "$2a$10$hash-almacenado" {
		t.Fatalf("se verifico contra %q", h.ultimoHash)
	}
}

func TestIniciarSesionRechazaClaveIncorrecta(t *testing.T) {
	repo := repoUsuarios{
		usuario: Usuario{ID: "usr-1", Email: "ana@redes.co"},
		hash:    "$2a$10$hash-almacenado",
	}
	h := &hasherEspia{valido: false}
	s := nuevasSesiones()

	_, err := nuevaAutenticacion(repo, h, s).IniciarSesion(
		context.Background(), "ana@redes.co", "la-que-no-es")
	if !errors.Is(err, ErrCredenciales) {
		t.Fatalf("se esperaba ErrCredenciales, se obtuvo %v", err)
	}
	if len(s.guardadas) != 0 {
		t.Fatal("no se puede crear una sesion con la clave equivocada")
	}
}

// El requisito de seguridad del issue, hecho asercion determinista.
//
// Con el email desconocido la tentacion es devolver ErrCredenciales de
// inmediato. Eso responde en microsegundos, mientras que un email que si
// existe paga los ~60 ms de bcrypt: la diferencia es medible desde fuera y
// convierte el login en un oraculo de que correos estan dados de alta.
//
// Se comprueba con el contador del espia y no con un cronometro porque una
// asercion sobre tiempo de pared es intermitente y esta no lo es.
func TestIniciarSesionVerificaAunqueElEmailNoExista(t *testing.T) {
	repo := repoUsuarios{err: ErrNoEncontrado}
	h := &hasherEspia{valido: false}
	s := nuevasSesiones()

	_, err := nuevaAutenticacion(repo, h, s).IniciarSesion(
		context.Background(), "nadie@redes.co", "la-clave")

	if !errors.Is(err, ErrCredenciales) {
		t.Fatalf("se esperaba ErrCredenciales, se obtuvo %v", err)
	}
	if h.verificaunas != 1 {
		t.Fatalf("Verificar se llamo %d veces con un email desconocido; "+
			"tiene que llamarse 1 vez para que el tiempo de respuesta no delate "+
			"si el correo existe", h.verificaunas)
	}
	if h.ultimoHash == "" {
		t.Fatal("se verifico contra un hash vacio: bcrypt sale enseguida y el canal lateral sigue abierto")
	}
}

// El mismo error para las dos causas. Si el mensaje distinguiera, el canal
// lateral volveria por el cuerpo de la respuesta en vez de por el reloj.
func TestIniciarSesionNoDistingueQueFactorFallo(t *testing.T) {
	sinUsuario := repoUsuarios{err: ErrNoEncontrado}
	conUsuario := repoUsuarios{
		usuario: Usuario{ID: "usr-1"},
		hash:    "$2a$10$hash-almacenado",
	}

	_, err1 := nuevaAutenticacion(sinUsuario, &hasherEspia{}, nuevasSesiones()).
		IniciarSesion(context.Background(), "nadie@redes.co", "x")
	_, err2 := nuevaAutenticacion(conUsuario, &hasherEspia{valido: false}, nuevasSesiones()).
		IniciarSesion(context.Background(), "ana@redes.co", "x")

	if err1.Error() != err2.Error() {
		t.Fatalf("los mensajes distinguen el factor: %q vs %q", err1, err2)
	}
}

// Un fallo real de base NO es "credenciales invalidas". Confundirlos manda al
// usuario a reautenticarse contra una base caida y esconde la averia.
func TestIniciarSesionPropagaUnFalloDeInfraestructura(t *testing.T) {
	caida := errors.New("connection refused")
	repo := repoUsuarios{err: caida}

	_, err := nuevaAutenticacion(repo, &hasherEspia{}, nuevasSesiones()).
		IniciarSesion(context.Background(), "ana@redes.co", "x")

	if errors.Is(err, ErrCredenciales) {
		t.Fatal("una caida de la base no puede salir como ErrCredenciales")
	}
	if !errors.Is(err, caida) {
		t.Fatalf("se perdio la causa: %v", err)
	}
}

// Si no hay token no hay sesion. Sin esta comprobacion, un generador roto
// entregaria la cadena vacia como credencial valida para todo el mundo.
func TestIniciarSesionFallaSiNoSePuedeGenerarToken(t *testing.T) {
	repo := repoUsuarios{
		usuario: Usuario{ID: "usr-1"},
		hash:    "$2a$10$hash-almacenado",
	}
	s := nuevasSesiones()
	a := nuevaAutenticacion(repo, &hasherEspia{valido: true}, s)
	a.Tokens = tokensFijos{err: errors.New("sin entropia")}

	if _, err := a.IniciarSesion(context.Background(), "ana@redes.co", "la-clave"); err == nil {
		t.Fatal("se esperaba error al no poder generar el token")
	}
	if len(s.guardadas) != 0 {
		t.Fatal("no se puede guardar una sesion sin token")
	}
}

func TestResolverSesion(t *testing.T) {
	s := nuevasSesiones()
	s.usuario = Usuario{ID: "usr-1", Rol: RolAuditor}
	repo := repoUsuarios{}
	a := nuevaAutenticacion(repo, &hasherEspia{}, s)

	if err := s.Crear(context.Background(), "tok", "usr-1", momento.Add(time.Hour)); err != nil {
		t.Fatalf("Crear: %v", err)
	}

	t.Run("token vigente devuelve el usuario", func(t *testing.T) {
		u, err := a.ResolverSesion(context.Background(), "tok")
		if err != nil {
			t.Fatalf("ResolverSesion: %v", err)
		}
		if u.Rol != RolAuditor {
			t.Fatalf("Rol = %q", u.Rol)
		}
	})

	t.Run("token desconocido es ErrNoEncontrado", func(t *testing.T) {
		_, err := a.ResolverSesion(context.Background(), "otro")
		if !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})

	// El reloj decide la caducidad, no la base: se mueve el instante, no se
	// espera.
	t.Run("token caducado es ErrNoEncontrado", func(t *testing.T) {
		caducado := a
		caducado.Reloj = relojFijo{instante: momento.Add(2 * time.Hour)}
		_, err := caducado.ResolverSesion(context.Background(), "tok")
		if !errors.Is(err, ErrNoEncontrado) {
			t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
		}
	})
}

func TestCerrarSesionRevocaElToken(t *testing.T) {
	s := nuevasSesiones()
	a := nuevaAutenticacion(repoUsuarios{}, &hasherEspia{}, s)

	if err := a.CerrarSesion(context.Background(), "tok"); err != nil {
		t.Fatalf("CerrarSesion: %v", err)
	}
	if len(s.revocados) != 1 || s.revocados[0] != "tok" {
		t.Fatalf("no se revoco el token: %v", s.revocados)
	}
}

// Cerrar una sesion que ya no existe no es un error: un cliente que reintenta
// el logout no merece un 500, y decir "ese token no existia" filtra
// informacion sobre tokens ajenos.
func TestCerrarSesionEsIdempotente(t *testing.T) {
	s := nuevasSesiones()
	a := nuevaAutenticacion(repoUsuarios{}, &hasherEspia{}, s)

	if err := a.CerrarSesion(context.Background(), "inexistente"); err != nil {
		t.Fatalf("revocar un token que no existe no es un error: %v", err)
	}
}
