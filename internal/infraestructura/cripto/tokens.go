package cripto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/rosvend/intela/internal/aplicacion"
)

// bytesToken son 32 bytes, es decir 256 bits, que es lo que pide el issue #16.
//
// No se usa crypto/rand.Text(), que seria mas corto: da 128 bits. Sobran para
// resistir fuerza bruta, pero el requisito escrito son 256 y un token es
// justo el sitio donde no conviene entregar la mitad de lo prometido sin que
// se note.
const bytesToken = 32

// TokensAleatorios adapta el puerto aplicacion.GeneradorTokens con la fuente
// de aleatoriedad del sistema operativo.
//
// Vive en cripto y no en aplicacion por lo mismo que bcrypt: de donde sale la
// entropia es una decision de adaptador. El nucleo pide un token; no elige la
// fuente ni la codificacion.
type TokensAleatorios struct{}

// Generar devuelve un token opaco en base64url sin relleno.
//
// base64url y no hexadecimal ni base64 estandar: el token viaja en una
// cabecera Authorization y puede acabar en una URL, y el alfabeto estandar
// lleva '+' y '/', que ahi hay que escapar. Sin relleno para que no lleve '='.
//
// El error nunca lo va a producir esta implementacion: crypto/rand.Read llena
// el buffer entero o mata el proceso -no devuelve error en ninguna plataforma
// que no sea un Linux antiguo-. Se propaga igual porque la firma es del
// puerto, y es la que permite que un doble simule el fallo en las pruebas del
// caso de uso.
func (TokensAleatorios) Generar() (string, error) {
	b := make([]byte, bytesToken)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("leer entropia del sistema: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

var _ aplicacion.GeneradorTokens = TokensAleatorios{}
