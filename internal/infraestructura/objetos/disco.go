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
//
// Una clave ya ocupada sale como aplicacion.ErrObjetoYaExiste, no como el
// os.ErrExist del sistema de ficheros. Es la simetrica de la traduccion que ya
// hace Obtener, y existe por la misma razon: quien llama tiene que poder
// distinguir "ya estaba" de "no se pudo escribir" sin aprenderse los errores
// del sistema de ficheros, que en un almacen S3 serian otros.
//
// La distincion no es cosmetica. La ingesta deriva la clave de la huella del
// contenido, asi que "ya estaba" significa "esos mismos bytes ya estan
// congelados" -sea por una resubida, por otra fuente que entrego lo mismo, o
// por un intento anterior que murio antes de dejar el acuse-, y ninguna de las
// tres es un fallo de escritura.
//
// # Todo o nada, y no solo "no sobrescribe"
//
// Los bytes NO se escriben sobre el destino. Van a un temporal del MISMO
// directorio, se sincronizan, y solo entonces se enlaza el resultado sobre la
// clave definitiva.
//
// El O_EXCL de antes daba "no sobrescribe" pero no daba "todo o nada": el
// fichero se creaba al abrirlo y los bytes se escribian despues, asi que un
// Write o un Sync que fallara a medias dejaba el destino OCUPADO por un objeto
// truncado. Y como la clave es la huella, el siguiente intento con los mismos
// bytes recibia ese resto como ErrObjetoYaExiste -o sea, como "esos bytes ya
// estan congelados"- y la ingesta lo certificaba. Medido con RLIMIT_FSIZE en
// TestPonerNoDejaObjetoTruncadoSiFallaLaEscritura.
//
// os.Link y NO os.Rename: Rename SOBRESCRIBE en silencio, que es justo lo que
// el ADR 0006 prohibe. Link falla con EEXIST y conserva la semantica que daba
// el O_EXCL.
//
// # Hueco de cobertura conocido: los dos fsync no los defiende ninguna prueba
//
// Se deja escrito porque el verde de la suite NO es senal aqui, y quien venga
// despues merece saberlo antes de "limpiar" dos lineas que parecen de mas.
//
// Medido, no supuesto: quitando `tmp.Sync()`, o quitando el `sincronizarDir`
// del final, `go test -race -count=1 ./...` sigue en verde ENTERO, incluidas
// las pruebas contra Postgres real. Las dos llamadas son hoy codigo que nadie
// comprueba.
//
// No es un descuido de las pruebas, es lo que cuesta el fenomeno: lo que un
// fsync compra es que los bytes sobrevivan a un CORTE DE CORRIENTE, y eso no se
// observa desde dentro del proceso que lo pide. `fsync` devuelve nil en los dos
// casos; la diferencia solo aparece despues del corte, en el arranque
// siguiente. Provocarlo de verdad pide algo que `go test` sin privilegios no
// alcanza -`dm-flakey` o una capa de bloques que descarte la cache, un
// LD_PRELOAD que haga fallar la syscall, o una VM a la que se le quite la
// alimentacion-, y un doble que solo compruebe "se llamo a Sync" no probaria
// durabilidad: probaria que esta escrita la linea que se acaba de escribir.
//
// Lo que se pierde si alguien las quita, para que el riesgo quede dimensionado:
// el objeto se enlaza y `GuardarReporte` escribe acto seguido el acuse en
// `reportes` diciendo de que bytes sale cada cifra. Tras un corte, el acuse
// -que si paso por el WAL de Postgres- puede sobrevivir a la evidencia que
// certifica. Es el mismo estado que el orden de GuardarReporte existe para
// impedir, alcanzado por el otro lado, y con el agravante de que no se recupera
// solo: la resubida de los mismos bytes choca contra el UNIQUE (sha256, fuente)
// y sale como ErrReporteDuplicado. La reproducibilidad a diez anos de `RD 16`
// se apoya en estas dos llamadas.
//
// Donde deja de importar: Disco es el adaptador de DESARROLLO. En cuanto entre
// el de MinIO o S3 que el ADR 0006 pide de verdad -con object-lock-, la
// durabilidad la responde el almacen y su contrato, no este fichero. Hasta
// entonces el hueco existe y esta aqui escrito.
func (d Disco) Poner(ctx context.Context, clave string, datos []byte) error {
	destino, err := d.ruta(clave)
	if err != nil {
		return err
	}
	dir := filepath.Dir(destino)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	// En el mismo directorio que el destino a proposito: os.Link no cruza
	// sistemas de ficheros, y un temporal en /tmp podria estar en otro.
	tmp, err := os.CreateTemp(dir, ".parcial-*")
	if err != nil {
		return err
	}
	// Si algo falla antes del Link, esto se lleva el temporal y el destino no
	// llega a existir. Si el Link va bien, esto solo quita el NOMBRE temporal:
	// el contenido se queda bajo la clave definitiva, que es el otro enlace.
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	// CreateTemp abre con 0600; el objeto definitivo lleva los mismos permisos
	// que le daba el OpenFile de antes.
	if err := tmp.Chmod(0o640); err != nil {
		return err
	}
	if _, err := tmp.Write(datos); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	// El Close explicito y no solo el del defer: en algunos sistemas de
	// ficheros el error de escritura diferida aparece aqui y en ningun otro
	// sitio, y tragarselo seria enlazar un objeto incompleto.
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Link(tmp.Name(), destino); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %q", aplicacion.ErrObjetoYaExiste, clave)
		}
		return err
	}

	// El Sync de arriba es el del FICHERO; este es el de su ENTRADA de
	// directorio, y hacen falta los dos. Un fsync del fichero no arrastra el
	// directorio que lo nombra: sin esto, un corte tras el COMMIT puede dejar
	// el acuse de `reportes` apuntando a un objeto que el sistema de ficheros
	// nunca llego a publicar. Es el mismo estado que el orden de GuardarReporte
	// existe para impedir, alcanzado por el otro lado.
	return sincronizarDir(dir)
}

// sincronizarDir fuerza a disco la entrada de directorio de lo que se acaba de
// enlazar.
func sincronizarDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
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

// Borrar quita el fichero. Una clave que no existe no es error: la
// compensacion del alta tiene que poder llamarlo dos veces.
func (d Disco) Borrar(_ context.Context, clave string) error {
	destino, err := d.ruta(clave)
	if err != nil {
		return err
	}
	if err := os.Remove(destino); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

var _ aplicacion.AlmacenObjetos = Disco{}
