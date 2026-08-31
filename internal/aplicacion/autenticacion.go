package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// hashSenuelo es un hash bcrypt valido que no corresponde a ninguna clave.
//
// Existe por una sola razon, y es de seguridad: cuando el email no esta en el
// padron hay que verificar CONTRA ALGO para que la peticion tarde lo mismo que
// una con un email real. Sin esto, un email desconocido responde en
// microsegundos y uno conocido paga los ~60 ms de bcrypt; la diferencia se mide
// desde fuera y convierte el login en un oraculo de que correos existen.
//
// Tiene que ser un hash bien formado: con una cadena vacia o basura, bcrypt
// detecta el formato invalido y sale enseguida, que deja el canal lateral
// exactamente igual de abierto.
//
// Se genero sobre 32 bytes de crypto/rand que no se guardaron, asi que no hay
// clave que case con el. NO se reutiliza el hash de las fixtures: ese es un
// hash publicado, de preimagen conocida, y ver uno de esos en codigo de
// produccion obliga a quien revisa a comprobar si es una credencial filtrada.
//
// El coste esta fijado a 10 porque es lo que emite hoy cripto.Bcrypt, y bcrypt
// lee el coste DEL PROPIO HASH, no de la configuracion de quien verifica. Si
// bcrypt.DefaultCost sube -su documentacion dice que sube con las versiones- y
// este senuelo se queda en 10, verificar contra el pasa a costar cuatro veces
// menos que verificar contra un hash del padron, y la asimetria de tiempo
// vuelve. El aviso esta puesto en cripto: TestElCosteEmitidoCoincideConElSenuelo.
const hashSenuelo = "$2a$10$GW0JkCUud..m0X7aK3Trj.J/YWFwuX2bE1.ih0dRnGgxlruzEk2Da"

// Autenticacion resuelve quien es quien: emite sesiones, las cierra y traduce
// un token en el Usuario que lo presenta.
//
// # Por que un servicio y no un struct por caso de uso
//
// Las tres operaciones giran sobre el mismo agregado -la sesion- y comparten
// dependencias. La segregacion que pide el ADR 0003 se conserva igual: lo que
// se inyecta son puertos estrechos, no un repositorio de 39 metodos.
//
// # Que NO decide
//
// Si el rol basta para una operacion. Eso es autorizacion, es ErrNoAutorizado,
// y vive en el middleware de roles. Aqui solo se responde "quien eres".
type Autenticacion struct {
	Usuarios RepositorioAfiliacion
	Claves   Hasher
	Sesiones Sesiones
	Reloj    Reloj
	Tokens   GeneradorTokens

	// TTL de una sesion. Cero no significa "eterna": ver IniciarSesion.
	TTL time.Duration
}

// ttl evita que una configuracion vacia emita credenciales permanentes.
//
// El puerto Sesiones dice que el TTL es "por contrato" porque una sesion sin
// expiracion es una credencial que nadie puede revocar. Un TTL cero por un
// campo sin cablear seria exactamente eso, y ademas violaria el CHECK
// expira > creada del esquema. Ante la duda, la sesion dura poco.
func (a Autenticacion) ttl() time.Duration {
	if a.TTL <= 0 {
		return 12 * time.Hour
	}
	return a.TTL
}

// IniciarSesion valida unas credenciales y emite una sesion.
//
// Devuelve ErrCredenciales tanto si el email no existe como si la clave no
// cuadra, y con el mismo mensaje: distinguirlos reabre por el cuerpo de la
// respuesta el mismo canal lateral que el hash senuelo cierra por el reloj.
//
// Un fallo de infraestructura NO se traduce: sube con su causa para que el
// adaptador responda 500 y no 401. Decirle "credenciales invalidas" a alguien
// porque la base esta caida lo manda a reautenticarse contra una base que no
// responde, y esconde la averia.
func (a Autenticacion) IniciarSesion(ctx context.Context, email, clave string) (Sesion, error) {
	usuario, hash, err := a.Usuarios.UsuarioPorEmail(ctx, email)
	switch {
	case err == nil:
		// Sigue abajo con el hash real.
	case errors.Is(err, ErrNoEncontrado):
		// Se verifica igualmente, contra el senuelo, para que el tiempo de
		// respuesta no delate que este correo no esta dado de alta. El
		// resultado se descarta: ya se sabe que no hay sesion que emitir.
		a.Claves.Verificar(hashSenuelo, clave)
		return Sesion{}, ErrCredenciales
	default:
		return Sesion{}, fmt.Errorf("buscar usuario: %w", err)
	}

	if !a.Claves.Verificar(hash, clave) {
		return Sesion{}, ErrCredenciales
	}

	token, err := a.Tokens.Generar()
	if err != nil {
		return Sesion{}, fmt.Errorf("generar token de sesion: %w", err)
	}

	expira := a.Reloj.Ahora().Add(a.ttl())
	if err := a.Sesiones.Crear(ctx, token, usuario.ID, expira); err != nil {
		return Sesion{}, fmt.Errorf("crear sesion: %w", err)
	}

	return Sesion{Token: token, Expira: expira, Usuario: usuario}, nil
}

// ResolverSesion traduce un token en el Usuario que lo presenta.
//
// Un token desconocido, revocado o caducado sale como ErrNoEncontrado, que es
// lo que el adaptador convierte en 401. Cualquier otro error sube con su causa
// y acaba en 500: son cosas distintas y tratarlas igual esconde una caida.
//
// La caducidad la decide el reloj inyectado, que se pasa al puerto. Asi se
// prueba moviendo el instante y no esperando.
func (a Autenticacion) ResolverSesion(ctx context.Context, token string) (Usuario, error) {
	if token == "" {
		return Usuario{}, ErrNoEncontrado
	}
	usuario, err := a.Sesiones.PorToken(ctx, token, a.Reloj.Ahora())
	if err != nil {
		return Usuario{}, err
	}
	return usuario, nil
}

// CerrarSesion revoca un token.
//
// Es idempotente por contrato: revocar uno que ya no esta no es un error. Un
// cliente que reintenta el logout no merece un 500, y responder "ese token no
// existia" cuenta algo sobre tokens ajenos a quien pregunte.
func (a Autenticacion) CerrarSesion(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := a.Sesiones.Revocar(ctx, token); err != nil {
		return fmt.Errorf("revocar sesion: %w", err)
	}
	return nil
}
