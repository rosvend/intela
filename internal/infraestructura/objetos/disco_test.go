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

// Las claves que un atacante controla. El periodo sale de un formulario y el
// nombre de multipart.FileHeader.Filename: los dos entran sin sanear en la
// clave que se le pasa al almacen.
func TestPonerRechazaEscapeDelDirectorio(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}

	claves := []string{
		"reportes/../../../etc/passwd",
		"../fuera.txt",
		"reportes/2026/../../../../tmp/x",
		"/etc/passwd",
		`reportes\..\..\fuera.txt`,
		"reportes/./../../fuera",
		"",
		"reportes//doble",
		"reportes/2026-01/sub dir/x.csv",
	}

	for _, clave := range claves {
		t.Run(clave, func(t *testing.T) {
			err := d.Poner(context.Background(), clave, []byte("x"))
			if !errors.Is(err, ErrClaveInvalida) {
				t.Fatalf("clave %q: se esperaba ErrClaveInvalida, se obtuvo %v", clave, err)
			}
		})
	}

	// Nada escrito fuera de la raiz, y nada dentro tampoco.
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

func TestPonerYObtener(t *testing.T) {
	raiz := t.TempDir()
	d := Disco{Dir: raiz}
	clave := "reportes/2026-01/abc123/parrilla.csv"
	datos := []byte("titulo,emisiones\nX,3\n")

	if err := d.Poner(context.Background(), clave, datos); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	leido, err := d.Obtener(context.Background(), clave)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if string(leido) != string(datos) {
		t.Fatalf("leido %q, esperado %q", leido, datos)
	}
	if _, err := os.Stat(filepath.Join(raiz, "reportes", "2026-01", "abc123", "parrilla.csv")); err != nil {
		t.Fatalf("el fichero no quedo donde toca: %v", err)
	}
}

// ADR 0006: la copia cruda es inmutable. Reescribir una clave ya usada es un
// error, no una actualizacion.
//
// Y el error es ErrObjetoYaExiste, no el os.ErrExist de debajo: quien llama
// tiene que poder distinguir "ya estaba" de "no se pudo escribir" sin conocer
// los errores del sistema de ficheros. La ingesta deriva la clave de la huella
// del contenido y depende de esa distincion para completar una subida que se
// quedo a medias.
func TestPonerNoSobrescribe(t *testing.T) {
	d := Disco{Dir: t.TempDir()}
	clave := "reportes/2026-01/sha/x.csv"

	if err := d.Poner(context.Background(), clave, []byte("original")); err != nil {
		t.Fatalf("primer Poner: %v", err)
	}
	err := d.Poner(context.Background(), clave, []byte("suplantado"))
	if err == nil {
		t.Fatal("se esperaba error al reescribir una clave existente")
	}
	if !errors.Is(err, aplicacion.ErrObjetoYaExiste) {
		t.Fatalf("se esperaba ErrObjetoYaExiste, se obtuvo %v", err)
	}

	leido, err := d.Obtener(context.Background(), clave)
	if err != nil {
		t.Fatalf("Obtener: %v", err)
	}
	if string(leido) != "original" {
		t.Fatalf("el contenido cambio: %q", leido)
	}
}

func TestBorrarQuitaElObjetoYEsIdempotente(t *testing.T) {
	d := Disco{Dir: t.TempDir()}
	clave := "afiliaciones/afil-1/rut"
	if err := d.Poner(context.Background(), clave, []byte("%PDF")); err != nil {
		t.Fatalf("Poner: %v", err)
	}
	if err := d.Borrar(context.Background(), clave); err != nil {
		t.Fatalf("Borrar: %v", err)
	}
	if _, err := d.Obtener(context.Background(), clave); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("tras borrar se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
	if err := d.Borrar(context.Background(), clave); err != nil {
		t.Fatalf("borrar de nuevo: %v", err)
	}
}

func TestObtenerInexistente(t *testing.T) {
	d := Disco{Dir: t.TempDir()}
	_, err := d.Obtener(context.Background(), "no/existe.csv")
	if err == nil || !strings.Contains(err.Error(), "no encontrado") {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
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
