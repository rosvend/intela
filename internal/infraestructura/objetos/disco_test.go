package objetos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
)

func TestDiscoCumpleElContrato(t *testing.T) {
	probarContrato(t, func(t *testing.T) aplicacion.AlmacenObjetos { return Disco{Dir: t.TempDir()} })
}

// Una clave invalida no deja nada escrito, ni fuera de la raiz ni dentro.
func TestPonerRechazaEscapeDelDirectorio(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}
	for _, clave := range clavesInvalidas {
		_ = d.Poner(context.Background(), clave, []byte("x"))
	}
	var vistos []string
	_ = filepath.Walk(raiz, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			vistos = append(vistos, p)
		}
		return nil
	})
	if len(vistos) != 0 {
		t.Fatalf("no se debio escribir nada, se escribio: %v", vistos)
	}
}

func TestPonerDejaElFicheroBajoLaClave(t *testing.T) {
	raiz := t.TempDir()
	if err := (Disco{Dir: raiz}).Poner(context.Background(), "reportes/2026-01/abc123/parrilla.csv", []byte("x")); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	if _, err := os.Stat(filepath.Join(raiz, "reportes", "2026-01", "abc123", "parrilla.csv")); err != nil {
		t.Fatalf("el fichero no quedo donde toca: %v", err)
	}
}

// H4a: "no sobrescribe" no es "todo o nada".
//
// Una escritura que muere a medias no puede dejar el destino OCUPADO. Si lo
// deja, el resto truncado se convierte en evidencia: la clave es la huella, asi
// que el siguiente intento con los mismos bytes recibe ese resto como
// ErrObjetoYaExiste -"esos bytes ya estan congelados"- y la ingesta escribe un
// acuse que certifica un SHA-256 que el objeto real no tiene.
//
// # Lo que esta prueba NO cubre, y no lo cubre ninguna otra
//
// Cubre la ATOMICIDAD del enlace, no la DURABILIDAD de los bytes. Los dos fsync
// de Poner -el del fichero temporal y el de la entrada de directorio- se pueden
// borrar los dos y la suite entera sigue verde; esta incluida. Comprobado
// quitandolos y corriendo `go test -race -count=1 ./...` completo.
//
// El motivo esta en el doc de Poner, con el riesgo dimensionado: un fsync
// compra que los bytes sobrevivan a un corte de corriente, y eso no se observa
// desde el proceso que lo pide. Aqui se repite el aviso porque este es el
// fichero que se lee cuando alguien pregunta "¿y esto quien lo prueba?".
//
// El fallo se provoca con RLIMIT_FSIZE, que es un EFBIG autentico del nucleo y
// no un doble: lo que se prueba es el adaptador de verdad contra el sistema de
// ficheros de verdad. Go deja SIGXFSZ sin efecto y el write devuelve el error,
// que es justo lo que hace falta aqui.
//
// Sin el arreglo esta prueba falla: el OpenFile con O_CREATE|O_EXCL creaba el
// destino ANTES de escribir, y el Write moria despues dejandolo a medias.
func TestPonerNoDejaObjetoTruncadoSiFallaLaEscritura(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}
	clave := "reportes/" + strings.Repeat("a", 64)

	const tope = 64 * 1024
	limitarTamanoDeFichero(t, tope)

	// Cuatro veces el tope: el write no puede completarse de ninguna manera.
	datos := make([]byte, 4*tope)
	for i := range datos {
		datos[i] = byte('a' + i%26)
	}

	err := d.Poner(context.Background(), clave, datos)
	if err == nil {
		t.Fatal("se esperaba error: la escritura no cabe en el limite")
	}
	// Y no puede salir como "ya existe": eso autorizaria a la ingesta a seguir.
	if errors.Is(err, aplicacion.ErrObjetoYaExiste) {
		t.Fatalf("un fallo de escritura no es ErrObjetoYaExiste: %v", err)
	}

	// Lo que de verdad importa: el destino quedo LIBRE.
	destino := filepath.Join(raiz, "reportes", strings.Repeat("a", 64))
	if info, err := os.Stat(destino); err == nil {
		t.Fatalf("el destino quedo ocupado por un objeto truncado: %d bytes", info.Size())
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat del destino: %v", err)
	}

	// Y sin restos: ni el objeto ni el temporal de la escritura fallida.
	var vistos []string
	_ = filepath.Walk(raiz, func(p string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			vistos = append(vistos, p)
		}
		return nil
	})
	if len(vistos) != 0 {
		t.Fatalf("una escritura fallida no puede dejar ficheros: %v", vistos)
	}

	// La boveda sigue utilizable: bajo esa clave todavia se puede congelar la
	// evidencia buena. Si el resto truncado se hubiera quedado, esto seria
	// ErrObjetoYaExiste y la evidencia real no entraria nunca.
	if err := d.Poner(context.Background(), clave, []byte("los bytes buenos")); err != nil {
		t.Fatalf("la clave tenia que seguir libre: %v", err)
	}
}

// limitarTamanoDeFichero pone RLIMIT_FSIZE para la prueba y lo restaura al
// terminar. Es estado del PROCESO, asi que ninguna prueba de este fichero
// puede llamar a t.Parallel().
func limitarTamanoDeFichero(t *testing.T, tope uint64) {
	t.Helper()

	var previo syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &previo); err != nil {
		t.Skipf("no se puede leer RLIMIT_FSIZE: %v", err)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE,
		&syscall.Rlimit{Cur: tope, Max: previo.Max}); err != nil {
		t.Skipf("no se puede bajar RLIMIT_FSIZE: %v", err)
	}
	t.Cleanup(func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &previo); err != nil {
			t.Errorf("restaurar RLIMIT_FSIZE: %v", err)
		}
	})
}
