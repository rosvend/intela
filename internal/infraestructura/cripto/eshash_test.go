package cripto

import "testing"

// Los valores que NO son un hash de este hasher.
//
// Todos comparten consecuencia: si alguno se guardara en `password_hash`,
// CompareHashAndPassword fallaria con la clave correcta y con cualquier otra, y
// no habria arreglo -- la provision se niega a correr dos veces y ninguna ruta
// HTTP resetea claves.
//
// La lista crecio dos veces, y las dos por un caso real que la version
// anterior dejaba pasar. Primero la clave en claro, que pasaba el unico control
// que habia entonces (`len >= 20`). Despues el hash TRUNCADO a 59, que pasaba
// `bcrypt.Cost` -- la libreria valida la cabecera, no el largo. Conviene
// asumir que la lista volvera a crecer.
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

// El hash TRUNCADO, que es el hueco que quedaba: `bcrypt.Cost` valida la
// cabecera y no el largo, asi que 59 caracteres pasaban su comprobacion.
//
// El borde importa y va medido en las dos direcciones: 59 lo acepta la libreria
// (coste 10, sin error) y 58 ya no. O sea que sin el control de largo el hueco
// era exactamente de un caracter, que es justo lo que se come un copiar/pegar,
// un `scp` cortado o un editor que recorta la ultima linea.
func TestEsHashRechazaUnHashTruncado(t *testing.T) {
	completo, err := Bcrypt{}.Hash("clave-real")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if len(completo) != largoHashBcrypt {
		t.Fatalf("largo del hash = %d, se esperaba %d", len(completo), largoHashBcrypt)
	}

	for _, n := range []int{59, 58, 50, 30, 7} {
		truncado := completo[:n]
		if (Bcrypt{}).EsHash(truncado) {
			t.Errorf("EsHash acepto un hash truncado a %d caracteres", n)
		}
		// La consecuencia, para que se vea por que importa: ni siquiera con la
		// clave correcta entra.
		if (Bcrypt{}).Verificar(truncado, "clave-real") {
			t.Errorf("Verificar entro con un hash truncado a %d", n)
		}
	}
}

// Y con caracteres de sobra tampoco, aunque ese SI verificaria.
//
// Es deliberadamente mas estricto que el minimo: la libreria ignora lo que
// sobra, asi que un hash con basura pegada detras funciona. Pero lo que produce
// Hash() mide 60 exactos, asi que otro largo significa que el valor se estropeo
// por el camino. Se rechaza en la validacion, en voz alta y antes de escribir
// nada, en vez de aceptar un valor mutilado que resulta que funciona.
func TestEsHashRechazaUnHashConSobras(t *testing.T) {
	completo, err := Bcrypt{}.Hash("clave-real")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	for _, sobra := range []string{"x", "\n", " ", "xxxxx"} {
		conSobra := completo + sobra
		if (Bcrypt{}).EsHash(conSobra) {
			t.Errorf("EsHash acepto un hash con %q pegado detras", sobra)
		}
		// Esto es lo que hace que la decision sea una eleccion y no un
		// descuido: verifica, y aun asi se rechaza.
		if !(Bcrypt{}).Verificar(conSobra, "clave-real") {
			t.Logf("nota: con %q pegado ya no verifica", sobra)
		}
	}
}
