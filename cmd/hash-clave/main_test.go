package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/infraestructura/cripto"
)

// Lo que sale sirve para lo que se va a usar: que `EsHash` lo acepte y que el
// verificador entre con la clave tecleada. Comparar cadenas no probaria nada
// -- es el error que tenia la primera version de la provision.
func TestHashDeStdinVerificaLaClave(t *testing.T) {
	const clave = "una-clave-de-operador"
	var salida, aviso bytes.Buffer

	if err := ejecutar(strings.NewReader(clave+"\n"), &salida, &aviso); err != nil {
		t.Fatalf("ejecutar: %v", err)
	}

	hash := strings.TrimSpace(salida.String())
	if !(cripto.Bcrypt{}).EsHash(hash) {
		t.Fatalf("la salida no tiene forma de hash: %q", hash)
	}
	if !(cripto.Bcrypt{}).Verificar(hash, clave) {
		t.Error("el verificador no entra con la clave tecleada")
	}
	if (cripto.Bcrypt{}).Verificar(hash, "otra-clave") {
		t.Error("el verificador entro con otra clave")
	}
}

// El hash va a stdout y el aviso a stderr, para que redirigir la salida deje
// en el fichero el hash y nada mas.
func TestElAvisoNoContaminaLaSalida(t *testing.T) {
	var salida, aviso bytes.Buffer
	if err := ejecutar(strings.NewReader("clave\n"), &salida, &aviso); err != nil {
		t.Fatalf("ejecutar: %v", err)
	}
	if strings.Count(strings.TrimSpace(salida.String()), "\n") != 0 {
		t.Errorf("stdout trae mas de una linea: %q", salida.String())
	}
	if !strings.Contains(aviso.String(), "clave") {
		t.Errorf("el aviso no salio por stderr: %q", aviso.String())
	}
}

// Sin salto final -- una tuberia sin \n -- tambien vale.
func TestAceptaEntradaSinSaltoFinal(t *testing.T) {
	var salida, aviso bytes.Buffer
	if err := ejecutar(strings.NewReader("clave-sin-salto"), &salida, &aviso); err != nil {
		t.Fatalf("ejecutar: %v", err)
	}
	if !(cripto.Bcrypt{}).Verificar(strings.TrimSpace(salida.String()), "clave-sin-salto") {
		t.Error("no verifico")
	}
}

func TestClaveVaciaEsUnError(t *testing.T) {
	for nombre, entrada := range map[string]string{"vacia": "", "solo salto": "\n"} {
		t.Run(nombre, func(t *testing.T) {
			var salida, aviso bytes.Buffer
			if err := ejecutar(strings.NewReader(entrada), &salida, &aviso); err == nil {
				t.Fatal("se esperaba error")
			}
			if salida.Len() != 0 {
				t.Errorf("no debe escribir nada en stdout: %q", salida.String())
			}
		})
	}
}

// Los espacios del borde son parte de la clave: recortarlos cambiaria la
// credencial sin avisar.
func TestLosEspaciosDelBordeSonParteDeLaClave(t *testing.T) {
	const clave = "  con espacios  "
	var salida, aviso bytes.Buffer
	if err := ejecutar(strings.NewReader(clave+"\n"), &salida, &aviso); err != nil {
		t.Fatalf("ejecutar: %v", err)
	}
	hash := strings.TrimSpace(salida.String())
	if !(cripto.Bcrypt{}).Verificar(hash, clave) {
		t.Error("no verifico con los espacios")
	}
	if (cripto.Bcrypt{}).Verificar(hash, strings.TrimSpace(clave)) {
		t.Error("verifico con la clave recortada: se perdieron los espacios")
	}
}
