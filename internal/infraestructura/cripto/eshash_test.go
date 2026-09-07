package cripto

import "testing"

// EL caso que dejaba produccion cerrada para siempre.
//
// Una clave en claro de 20 caracteres o mas pasaba el unico control que habia
// -- la longitud -- y el CHECK (length(password_hash) >= 20) del esquema. Se
// guardaba tal cual, y a partir de ahi CompareHashAndPassword fallaba con la
// clave correcta y con cualquier otra, sin ninguna via de arreglo: esta
// operacion se niega a correr dos veces y ninguna ruta HTTP resetea claves.
func TestEsHashRechazaUnaClaveEnClaro(t *testing.T) {
	claros := map[string]string{
		"larga":            "esta-clave-tiene-mas-de-veinte-caracteres",
		"justo 20":         "12345678901234567890",
		"con pinta de sal": "$2a$10$esto-no-es-un-hash-de-verdad",
		"prefijo solo":     "$2a$10$",
		"vacia":            "",
	}
	for nombre, claro := range claros {
		t.Run(nombre, func(t *testing.T) {
			if (Bcrypt{}).EsHash(claro) {
				t.Fatalf("EsHash(%q) = true, se esperaba false", claro)
			}
		})
	}
}

// Y lo que SI es un hash pasa, incluido uno de otro coste: la regla es "lo que
// esta libreria sabe verificar", no una longitud ni un prefijo concreto.
func TestEsHashAceptaLoQueElMismoProduce(t *testing.T) {
	for _, coste := range []int{0, 4, 6} {
		b := Bcrypt{Coste: coste}
		h, err := b.Hash("clave-de-prueba")
		if err != nil {
			t.Fatalf("Hash(coste=%d): %v", coste, err)
		}
		if !b.EsHash(h) {
			t.Errorf("EsHash rechazo un hash propio (coste=%d): %q", coste, h)
		}
		// La propiedad completa: si EsHash dice que si, Verificar funciona.
		if !b.Verificar(h, "clave-de-prueba") {
			t.Errorf("Verificar fallo sobre un hash que EsHash acepto (coste=%d)", coste)
		}
	}
}
