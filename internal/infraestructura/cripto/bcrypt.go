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

// EsHash usa bcrypt.Cost como validador de forma.
//
// La libreria parsea la cabecera -- prefijo, coste de dos digitos, sal -- y
// falla en cuanto algo no cuadra. Preguntarle a ella y no escribir aqui una
// expresion regular con `$2[aby]$` es lo que hace que esto siga siendo cierto
// el dia que bcrypt acepte un prefijo nuevo: la regla es "lo que esta libreria
// sabe verificar", no "lo que se parece a lo que yo recuerdo de bcrypt".
func (b Bcrypt) EsHash(posible string) bool {
	_, err := bcrypt.Cost([]byte(posible))
	return err == nil
}

var _ aplicacion.Hasher = Bcrypt{}
