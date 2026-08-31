package cripto

import (
	"strings"
	"testing"
)

// Aviso, no prueba de comportamiento.
//
// Bcrypt usa bcrypt.DefaultCost a proposito, y la documentacion de la libreria
// dice que ese valor SUBE con las versiones. El dia que suba, los hashes nuevos
// del padron costaran mas que el hashSenuelo de internal/aplicacion, que esta
// fijado a coste 10 -y bcrypt lee el coste del propio hash, no de la
// configuracion de quien verifica-.
//
// A partir de ahi, verificar un correo desconocido cuesta cuatro veces menos
// que verificar uno real, y el canal lateral de tiempo que cierra el issue #16
// se reabre en silencio: no falla nada, solo cambia un numero que nadie mira.
//
// Cuando esta prueba se ponga en rojo, la correccion NO es cambiar el numero de
// aqui: es regenerar hashSenuelo con el coste nuevo y luego actualizar esta.
func TestElCosteEmitidoCoincideConElSenuelo(t *testing.T) {
	const costeDelSenuelo = "$2a$10$"

	h, err := Bcrypt{}.Hash("una-clave-cualquiera")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(h, costeDelSenuelo) {
		t.Fatalf("Bcrypt emite %q pero el senuelo de aplicacion es %q. "+
			"Regenera hashSenuelo en internal/aplicacion/autenticacion.go con el "+
			"coste nuevo, o el login volvera a delatar que correos existen",
			h[:7], costeDelSenuelo)
	}
}

func TestVerificarAceptaLaClaveYRechazaLasDemas(t *testing.T) {
	b := Bcrypt{}

	h, err := b.Hash("la-clave")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !b.Verificar(h, "la-clave") {
		t.Fatal("la clave correcta tiene que verificar")
	}
	if b.Verificar(h, "otra-clave") {
		t.Fatal("una clave distinta no puede verificar")
	}
}

// Un hash mal formado es falso, no un panico: si una fila del padron llegara
// corrupta, el login de ese usuario falla, no el proceso entero.
func TestVerificarConHashInvalidoEsFalso(t *testing.T) {
	b := Bcrypt{}
	for _, hash := range []string{"", "no-es-un-hash", "$2a$10$corto"} {
		if b.Verificar(hash, "la-clave") {
			t.Fatalf("un hash invalido (%q) no puede verificar", hash)
		}
	}
}
