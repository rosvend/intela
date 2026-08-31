package cripto

import (
	"encoding/base64"
	"testing"
)

// 256 bits es lo que pide el issue. Se comprueba sobre los bytes decodificados
// y no sobre la longitud de la cadena: la codificacion podria cambiar sin que
// cambie la entropia, y es la entropia lo que importa.
func TestTokenTiene256BitsDeEntropia(t *testing.T) {
	tok, err := TokensAleatorios{}.Generar()
	if err != nil {
		t.Fatalf("Generar: %v", err)
	}

	crudo, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		t.Fatalf("el token no es base64url sin relleno: %v", err)
	}
	if len(crudo) != 32 {
		t.Fatalf("%d bytes de entropia, se esperaban 32 (256 bits)", len(crudo))
	}
}

// Viaja en una cabecera Authorization y puede acabar en una URL: sin
// caracteres que haya que escapar.
func TestTokenEsSeguroEnURLYCabeceras(t *testing.T) {
	tok, err := TokensAleatorios{}.Generar()
	if err != nil {
		t.Fatalf("Generar: %v", err)
	}
	for _, c := range tok {
		esValido := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'
		if !esValido {
			t.Fatalf("el token lleva %q, que hay que escapar en una URL: %q", c, tok)
		}
	}
}

// Dos tokens iguales serian dos sesiones con la misma clave primaria: la
// segunda sobrescribiria a la primera y dos personas compartirian sesion.
func TestTokensNoSeRepiten(t *testing.T) {
	const cuantos = 1000
	vistos := make(map[string]struct{}, cuantos)

	g := TokensAleatorios{}
	for i := range cuantos {
		tok, err := g.Generar()
		if err != nil {
			t.Fatalf("Generar (%d): %v", i, err)
		}
		if _, repetido := vistos[tok]; repetido {
			t.Fatalf("token repetido en la iteracion %d: %q", i, tok)
		}
		vistos[tok] = struct{}{}
	}
}
