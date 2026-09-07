// Command hash-clave calcula el hash de una clave leida por la entrada estandar.
//
// Existe para la provision inicial (`docs/runbooks/primer-administrador.md`),
// que recibe la clave YA hasheada: asi la credencial en claro no viaja en el
// evento de invocacion, no queda en el registro de la plataforma y el proceso
// que provisiona no la ve nunca.
//
// # Por que por stdin y no por argumento
//
// Un argumento de linea de comandos es visible en la tabla de procesos de toda
// la maquina (`ps`) mientras el comando corre, y queda escrito en el historial
// del shell. La receta anterior de este procedimiento pasaba la clave asi.
//
// # Por que este binario y no un python de tres lineas
//
// Usa `cripto.Bcrypt`, que es EXACTAMENTE el verificador que despues comprueba
// el login: mismo algoritmo y mismo coste, por construccion y no por acuerdo.
// Un script equivalente en otro lenguaje puede elegir otro coste sin que nadie
// lo note, y anade una dependencia que la maquina del operador puede no tener.
//
// NO entra en la imagen de produccion: el Dockerfile nombra los binarios que
// copia, uno por uno, y este no esta en la lista.
//
//	$ go run ./cmd/hash-clave
//	clave: (se teclea, no se muestra en ps ni en el historial)
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rosvend/intela/internal/infraestructura/cripto"
)

func main() {
	if err := ejecutar(os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "hash-clave: %v\n", err)
		os.Exit(1)
	}
}

// ejecutar recibe los tres flujos para que la prueba no toque el proceso.
//
// El aviso va por stderr y el hash por stdout, separados a proposito: asi
// `go run ./cmd/hash-clave > h.txt` deja en el fichero el hash y nada mas.
func ejecutar(entrada io.Reader, salida, aviso io.Writer) error {
	// El aviso es cosmetico: si no se puede escribir, el comando sigue siendo
	// util -- el hash va por otro flujo. Se descarta explicitamente.
	_, _ = fmt.Fprint(aviso, "clave: ")

	lector := bufio.NewReader(entrada)
	linea, err := lector.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("leer la clave: %w", err)
	}

	// Solo el salto de linea final: una clave puede empezar o acabar con un
	// espacio a proposito, y recortarlo entero cambiaria la credencial sin
	// avisar. Lo que se hashea es lo que se teclea.
	clave := strings.TrimRight(linea, "\r\n")
	if clave == "" {
		return errors.New("la clave esta vacia")
	}

	hash, err := cripto.Bcrypt{}.Hash(clave)
	if err != nil {
		return fmt.Errorf("hashear: %w", err)
	}

	// Este SI se comprueba, y no por el linter: con la salida redirigida a un
	// fichero, una escritura que falle a medias deja al operador con un hash
	// truncado y la impresion de que todo fue bien. Preferible fallar aqui.
	if _, err := fmt.Fprintln(salida, hash); err != nil {
		return fmt.Errorf("escribir el hash: %w", err)
	}
	return nil
}
