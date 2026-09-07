// Package cripto adapta el puerto aplicacion.Hasher.
//
// bcrypt vivia dentro de internal/aplicacion, que es nucleo. El algoritmo con
// el que se protege una credencial es una decision de adaptador: el nucleo
// decide que hay que verificar, no con que.
package cripto

import (
	"golang.org/x/crypto/bcrypt"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Bcrypt usa el coste por defecto de la libreria, que sube con las versiones.
type Bcrypt struct {
	// Coste opcional. Cero usa bcrypt.DefaultCost.
	Coste int
}

func (b Bcrypt) coste() int {
	if b.Coste == 0 {
		return bcrypt.DefaultCost
	}
	return b.Coste
}

// Verificar compara en tiempo constante, que es lo que hace la libreria.
func (b Bcrypt) Verificar(hash, clave string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(clave)) == nil
}

func (b Bcrypt) Hash(clave string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(clave), b.coste())
	return string(h), err
}

// largoHashBcrypt es el largo exacto de lo que produce [Bcrypt.Hash]: 7 de
// cabecera (`$2a$10$`), 22 de sal y 31 de resumen.
//
// Se comprueba a mano porque la libreria NO lo comprueba, y es el unico hueco
// que quedaba: `bcrypt.Cost` valida la CABECERA, no el largo total, asi que un
// hash truncado a 59 caracteres lo pasa -- devuelve coste 10 y ningun error --
// y despues `CompareHashAndPassword` falla para siempre, con la clave correcta
// y con cualquier otra. Medido: 59 pasa, 58 y menos ya los rechaza la libreria.
//
// Antes de escribir esta constante se probo delegarlo, que es lo que pide el
// comentario de EsHash: `CompareHashAndPassword` sobre el hash de 59 devuelve
// ErrMismatchedHashAndPassword, indistinguible de un hash bien formado con la
// clave equivocada. No hay a quien preguntar; esta comprobacion hay que
// escribirla.
//
// Igualdad exacta y no `>=`, aunque un hash con caracteres de sobra (61, 65)
// SI verifica correctamente -- la libreria ignora lo que sobra, tambien
// medido. Justo por eso: `>=` aceptaria en silencio un hash con basura pegada
// detras, y lo que produce Hash() mide 60 exactos. Cualquier otro largo
// significa que el valor se estropeo por el camino, que es la clase de fallo
// de la que esto defiende. Se rechaza en la validacion, en voz alta y antes de
// escribir nada, no en el login tres pasos despues.
const largoHashBcrypt = 60

// EsHash dice si una cadena es un hash de este hasher.
//
// Dos comprobaciones, y las dos hacen falta:
//
//  1. La CABECERA la valida la libreria (`bcrypt.Cost`): prefijo, coste de dos
//     digitos, sal. Se le pregunta a ella y no se escribe aqui una expresion
//     regular con `$2[aby]$`, para que esto siga siendo cierto el dia que
//     bcrypt acepte un prefijo nuevo. La regla es "lo que esta libreria sabe
//     verificar", no "lo que se parece a lo que yo recuerdo de bcrypt".
//  2. El LARGO no lo valida nadie mas. Ver [largoHashBcrypt].
func (b Bcrypt) EsHash(posible string) bool {
	if len(posible) != largoHashBcrypt {
		return false
	}
	_, err := bcrypt.Cost([]byte(posible))
	return err == nil
}

var _ aplicacion.Hasher = Bcrypt{}
