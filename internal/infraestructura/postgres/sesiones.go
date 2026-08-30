package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

var _ aplicacion.Sesiones = (*Store)(nil)

// resumir devuelve el SHA-256 hexadecimal de un token.
//
// # Por que se guarda un resumen y no el token
//
// Quien pueda leer `sesiones` -una copia de seguridad, un volcado para
// depurar, una inyeccion SQL de solo lectura- se haria pasar por cualquiera
// que tenga la sesion abierta. Con el resumen, esa lectura no sirve de nada:
// de aqui no se vuelve al token.
//
// # Por que SHA-256 y no bcrypt
//
// Parece la opcion obvia porque es lo que usa el adaptador de contrasenas de
// al lado, y es la equivocada, por dos razones:
//
//   - bcrypt lleva sal aleatoria, asi que el resumen NO es determinista y no
//     se puede buscar por clave primaria. Habria que traerse todas las sesiones
//     vivas y compararlas una a una en cada peticion autenticada.
//   - bcrypt existe para estirar secretos de baja entropia, que es lo que es
//     una contrasena elegida por una persona. Este token trae 256 bits de
//     crypto/rand: no hay diccionario que lo alcance, y encarecer el resumen no
//     compra nada. Aqui el hash solo tiene que ser de un solo sentido.
//
// Esta escrito en el ADR 0013.
//
// # Por que hashea el adaptador y no el caso de uso
//
// "No guardar el secreto en claro" es una propiedad del ALMACENAMIENTO. Los
// tres metodos del puerto siguen significando "token en claro", y el nucleo no
// aprende que existe SHA-256 -igual que no sabe que existe bcrypt-.
func resumir(token string) string {
	suma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(suma[:])
}

// Crear abre una sesion. Recibe el token EN CLARO y guarda su resumen.
//
// `creada` se deja en su DEFAULT now(): el CHECK sesion_expira_despues compara
// las dos columnas, asi que rellenarla desde el reloj inyectado mientras la
// otra viene del servidor es pedir que una diferencia de reloj rechace un
// login legitimo.
func (s *Store) Crear(ctx context.Context, token, usuarioID string, expira time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sesiones (token, usuario_id, expira) VALUES ($1, $2, $3)`,
		resumir(token), usuarioID, expira)
	return traducirError(err, "crear sesion de %q", usuarioID)
}

// PorToken resuelve un token vigente al Usuario que lo presenta.
//
// La caducidad se filtra en el WHERE y no despues, en Go: asi un token
// caducado y uno que no existe son indistinguibles desde fuera -los dos son
// ErrNoEncontrado-, que es lo que el caso de uso convierte en 401.
//
// `ahora` llega como parametro y no de now(): el ADR 0005 quiere el instante
// inyectado para que la caducidad se pueda probar moviendolo en vez de
// esperando.
//
// Trae el Usuario entero de una vez porque el middleware lo pone en el
// contexto y #17 decide el rol contra el; resolver el token y luego pedir el
// usuario por id serian dos viajes en cada peticion autenticada.
//
// No pide password_hash: resolver una sesion no necesita la credencial, y
// leerla en cada peticion autenticada para tirarla seria sacar de la base algo
// secreto sin motivo. Por eso columnasUsuario ya no la trae.
//
// Subconsulta y no JOIN para poder reutilizar columnasUsuario tal cual: con un
// JOIN habria que calificar las cinco columnas con el alias de la tabla, y esa
// proyeccion se comparte con afiliacion.go justamente para que no diverjan.
func (s *Store) PorToken(ctx context.Context, token string, ahora time.Time) (aplicacion.Usuario, error) {
	fila := s.pool.QueryRow(ctx,
		`SELECT `+columnasUsuario+` FROM usuarios
		  WHERE id = (SELECT usuario_id FROM sesiones
		               WHERE token = $1 AND expira > $2)`,
		resumir(token), ahora)

	u, err := escanearUsuario(fila, nil)
	if err != nil {
		// El token no entra en el mensaje, ni siquiera recortado: los errores
		// acaban en los logs, y un log con credenciales dentro es la fuga que
		// el resumen de arriba estaba evitando.
		return aplicacion.Usuario{}, traducirError(err, "resolver sesion")
	}
	return u, nil
}

// Revocar cierra una sesion. Es idempotente por contrato: borrar cero filas no
// es un error.
//
// Un cliente que reintenta el logout no merece un 500, y responder "ese token
// no existia" le cuenta a quien pregunte si un token ajeno esta vivo.
func (s *Store) Revocar(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sesiones WHERE token = $1`, resumir(token))
	return traducirError(err, "revocar sesion")
}
