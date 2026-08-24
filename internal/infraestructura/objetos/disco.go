// Package objetos adapta el puerto aplicacion.AlmacenObjetos.
//
// Disco escribe en el sistema de ficheros local. Es el adaptador de
// desarrollo: el ADR 0006 pide copia inmutable con retencion para los
// reportes crudos, que es la evidencia de la que cuelga todo lo demas, y eso
// pide MinIO o S3 con object-lock. Disco no da inmutabilidad.
package objetos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ErrClaveInvalida: la clave intenta salirse del directorio raiz.
var ErrClaveInvalida = errors.New("clave de objeto invalida")

// claveValida acota lo que puede aparecer en una clave: segmentos de letras,
// digitos, punto, guion y guion bajo, separados por barras.
//
// La lista blanca es deliberada. La alternativa -buscar ".." y rechazarlo- ha
// fallado historicamente ante codificaciones, enlaces y separadores de
// Windows; aqui solo pasa lo que se nombra.
var claveValida = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)

// Disco guarda cada objeto como un fichero bajo Dir.
type Disco struct {
	Dir string
}

// ruta traduce una clave a una ruta absoluta dentro de Dir, o falla.
//
// Tres comprobaciones, y hacen falta las tres: la clave la compone quien sube
// el fichero -el periodo sale de un formulario y el nombre de
// multipart.FileHeader.Filename, que la documentacion de Go advierte
// explicitamente que no es de fiar-, asi que un "../" en cualquiera de los
// dos escapaba del directorio y escribia donde alcanzase el proceso.
func (d Disco) ruta(clave string) (string, error) {
	if clave == "" || !claveValida.MatchString(clave) {
		return "", fmt.Errorf("%w: %q", ErrClaveInvalida, clave)
	}
	for _, seg := range strings.Split(clave, "/") {
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("%w: %q", ErrClaveInvalida, clave)
		}
	}

	raiz, err := filepath.Abs(d.Dir)
	if err != nil {
		return "", err
	}
	destino := filepath.Join(raiz, filepath.FromSlash(clave))

	// Cinturon y tirantes: aunque el patron ya lo impide, se comprueba que la
	// ruta resuelta siga colgando de la raiz.
	if destino != raiz && !strings.HasPrefix(destino, raiz+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q escapa de %q", ErrClaveInvalida, clave, d.Dir)
	}
	return destino, nil
}

// Poner escribe un objeto. No sobrescribe: el ADR 0006 pide que la copia
// cruda sea inmutable, asi que reescribir una clave existente es un error y
// no una actualizacion silenciosa.
func (d Disco) Poner(ctx context.Context, clave string, datos []byte) error {
	destino, err := d.ruta(clave)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(destino, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(datos); err != nil {
		return err
	}
	return f.Sync()
}

func (d Disco) Obtener(ctx context.Context, clave string) ([]byte, error) {
	destino, err := d.ruta(clave)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(destino)
	if errors.Is(err, os.ErrNotExist) {
		return nil, aplicacion.ErrNoEncontrado
	}
	return b, err
}

var _ aplicacion.AlmacenObjetos = Disco{}
