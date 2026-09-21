package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rosvend/intela/internal/aplicacion"
)

// Ingesta es lo que la capa HTTP necesita del nucleo para recibir entregas.
//
// Declarada en el consumidor, igual que [Catalogo] y [Autenticacion]:
// [aplicacion.Ingesta] la satisface sin nombrarla, y las pruebas pasan un doble
// sin levantar ni base ni boveda.
type Ingesta interface {
	IngerirReporte(ctx context.Context, fuente, formato, periodo string, datos []byte) (aplicacion.Recepcion, error)
	Cargas(ctx context.Context, periodo string, pag aplicacion.Paginacion) ([]aplicacion.CargaReporte, error)
	RechazosDeCarga(ctx context.Context, id string, pag aplicacion.Paginacion) ([]aplicacion.UsoPersistido, error)
	DeducirFormato(nombre string) string
}

// tamanoMaximoEntrega es el tope del ARCHIVO de una subida.
//
// La muestra del cliente son 21 KB y 16 KB. 32 MiB deja sitio de sobra para
// parrillas de un ano y sigue siendo un tope: sin el, `multipart.Reader`
// escribe en disco lo que le manden y una subida basta para llenar el volumen.
// El streaming de archivos grandes de verdad es el issue #46.
const tamanoMaximoEntrega = 32 << 20

// memoriaMaximaMultipart es lo que ParseMultipartForm guarda EN MEMORIA antes
// de derramar a disco.
//
// Es 1 MiB a proposito y NO tamanoMaximoEntrega: el argumento no acota lo que
// se lee -pasado ese numero `multipart.Reader` sigue leyendo y derrama a disco
// todo lo que le manden-, asi que pasarle el tope del archivo garantiza que
// una subida dentro del tope NUNCA se derrame, y el RemoveAll de abajo no
// llegaria a tener nada que limpiar. Con 1 MiB el pico de RSS por subida es
// ese mas el `io.ReadAll` de despues, y los temporales si existen y si se
// limpian.
const memoriaMaximaMultipart = 1 << 20

// tamanoMaximoCuerpo es el tope de la PETICION entera, que es donde el limite
// se puede aplicar de verdad.
//
// Son dos topes y no uno porque se comprueban en dos momentos que no se pueden
// juntar. `tamanoMaximoEntrega` mide el archivo, y para medirlo hay que haberlo
// recibido ya: cuando esa comprobacion se ejecuta, `ParseMultipartForm` lleva
// rato escribiendo en disco lo que le manden. Medido: una peticion de 40 MiB
// derrama ~40 MB antes de que nada la pueda frenar, y una de 4 GB los derrama
// tambien. El tope del archivo no es una defensa de entrada; es una regla de
// negocio sobre algo que ya llego.
//
// Este otro si es de entrada: va en un [http.MaxBytesReader] alrededor del
// Body, ANTES del parseo, y corta la lectura en cuanto se pasa. Lo que se llega
// a derramar deja de depender de lo que mande quien sube.
//
// El margen sobre el tope del archivo es la envoltura multipart -- los campos
// `fuente`, `periodo` y `formato`, las cabeceras de cada parte y las lineas de
// frontera --, que viaja en el mismo cuerpo. Sin el, un archivo de exactamente
// 32 MiB se rechazaria por el peso del sobre y el tope documentado seria mentira
// por unos cientos de bytes. 1 MiB es holgado a proposito: la envoltura real son
// unos cientos de bytes, y aqui lo que importa es que el maximo este ACOTADO, no
// que sea exacto.
const tamanoMaximoCuerpo = tamanoMaximoEntrega + 1<<20

// leerCuerpoArchivo lee el archivo ya recibido. Es una variable y no una
// llamada directa a [io.ReadAll] para que las pruebas puedan simular el fallo
// de lectura del temporal del servidor sin romper el disco: en produccion es
// io.ReadAll tal cual.
//
// El fallo de lectura a esta altura es E/S del servidor -los bytes ya estan
// en RAM o en un temporal- y se responde 500. Sin esta costura, la unica forma
// de cubrir esa rama seria romper el temporal entre el parseo y la lectura,
// que no tiene gancho determinista.
var leerCuerpoArchivo = io.ReadAll

// esFalloTemporal dice si el error de ParseMultipartForm viene del disco del
// servidor y no del cliente.
//
// Con memoriaMaximaMultipart en 1 MiB, todo archivo mayor se derrama a un
// temporal via os.CreateTemp: un TMPDIR roto, sin permiso o lleno (ENOENT,
// EACCES, ENOSPC) vuelve como *os.PathError o *os.SyscallError envuelto en el
// error del parseo. Eso es E/S del servidor, no peticion malformada, y se
// responde 500 con log a Error.
//
// Un cuerpo que no es multipart, sin boundary o truncado (ErrNotMultipart,
// ErrMissingBoundary, io.ErrUnexpectedEOF) NO es os.PathError y sigue dando
// 400. El tope se distingue antes, por *http.MaxBytesError, y da 413.
func esFalloTemporal(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno)
}

// entregaJSON es el acuse que devuelve una subida.
//
// Los rechazos viajan ENTEROS, no como recuento: quien sube el archivo tiene
// que poder leer que linea le falta y por que sin abrir la base. Es criterio de
// aceptacion de OE-1.
type entregaJSON struct {
	ID          string        `json:"id"`
	Fuente      string        `json:"fuente"`
	Periodo     string        `json:"periodo"`
	SHA256      string        `json:"sha256"`
	ClaveObjeto string        `json:"clave_objeto"`
	NBytes      int           `json:"nbytes"`
	Aceptados   int           `json:"aceptados"`
	Rechazados  []rechazoJSON `json:"rechazados"`
}

// rechazoJSON es una fila que no se pudo normalizar.
//
// Lleva lo identificatorio y el motivo, y NINGUNA columna de medida: es la
// misma forma que la tabla `usos_rechazados` (ADR 0016), y por la misma razon
// -- una fila rechazada no pondera, y sin las medidas nadie las puede sumar
// "solo para ver".
type rechazoJSON struct {
	ID        string `json:"id"`
	Titulo    string `json:"titulo"`
	IDsFuente string `json:"ids_fuente"`
	Motivo    string `json:"motivo"`
}

// aRechazosJSON es la UNICA traduccion de un rechazo a la respuesta, la que
// comparten el acuse del POST y el log de GET /reportes/{id}/rechazos. Con dos
// copias, una medida anadida en un sitio y olvidada en el otro saldria por una
// de las dos puertas (ADR 0016).
//
// Nunca devuelve nil: make y no var, porque una lista sin rechazos tiene que
// salir como [] y no como null, o cualquier cliente que itere la lista revienta
// justo en el caso bueno.
func aRechazosJSON(usos []aplicacion.UsoPersistido) []rechazoJSON {
	rechazos := make([]rechazoJSON, 0, len(usos))
	for _, u := range usos {
		rechazos = append(rechazos, rechazoJSON{
			ID: u.ID, Titulo: u.Titulo, IDsFuente: u.IDsFuente, Motivo: u.RechazoMotivo,
		})
	}
	return rechazos
}

// cargaJSON es una entrega en el listado de cargas hechas.
type cargaJSON struct {
	ID          string    `json:"id"`
	Fuente      string    `json:"fuente"`
	Periodo     string    `json:"periodo"`
	SHA256      string    `json:"sha256"`
	ClaveObjeto string    `json:"clave_objeto"`
	NBytes      int       `json:"nbytes"`
	Recibido    time.Time `json:"recibido"`
	Aceptados   int       `json:"aceptados"`
	Rechazados  int       `json:"rechazados"`
}

// subirReporte recibe una entrega por multipart y la ingiere entera.
//
// # Por que multipart y no un cuerpo binario con cabeceras
//
// Un .xlsx no cabe en un JSON sin base64, y la entrega necesita tres datos que
// no estan en el archivo: la fuente, el periodo de recaudo al que pondera y --
// implicito en el nombre -- el formato. multipart es la unica forma de mandar
// los cuatro en una peticion sin inventarse cabeceras propias.
//
// # El nombre del fichero decide UNA cosa y no puede decidir mas
//
// `multipart.FileHeader.Filename` no es de fiar, y la propia documentacion de
// Go lo advierte: lo elige el cliente y puede traer separadores de ruta. Aqui
// se usa SOLO para deducir el formato, que es una decision que se puede
// equivocar sin consecuencias -- un formato mal deducido da un error de lectura
// y nada mas --. La clave del objeto de la boveda se deriva de la HUELLA del
// contenido, no del nombre, asi que no hay forma de que un nombre hostil llegue
// al sistema de ficheros.
func (a *API) subirReporte(w http.ResponseWriter, r *http.Request) {
	// ANTES del parseo, que es lo unico que hace que el tope sea un tope. El
	// argumento de ParseMultipartForm limita lo que se guarda EN MEMORIA y no lo
	// que se lee: pasado ese numero, `multipart.Reader` sigue leyendo y va
	// derramando a disco todo lo que le manden. Con MaxBytesReader la lectura se
	// corta en el limite y el resto del cuerpo no llega a existir en ningun
	// lado.
	r.Body = http.MaxBytesReader(w, r.Body, tamanoMaximoCuerpo)

	if err := r.ParseMultipartForm(memoriaMaximaMultipart); err != nil {
		// Pasarse del tope no es una peticion mal formada, asi que no puede
		// contestarse con el mismo 400 que un cuerpo que no es multipart: quien
		// sube leeria "manda un multipart" habiendo mandado uno. MaxBytesReader
		// devuelve *http.MaxBytesError, y viaja envuelto en el error del parseo.
		var excede *http.MaxBytesError
		if errors.As(err, &excede) {
			escribirError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("la entrega pasa de %d MiB", tamanoMaximoEntrega>>20))
			return
		}
		// El derrame a disco tambien es E/S del servidor, no peticion
		// malformada: con 1 MiB en memoria, todo archivo mayor crea un temporal
		// via os.CreateTemp, y un TMPDIR roto, sin permiso o lleno (ENOENT,
		// EACCES, ENOSPC) falla aqui. Es el punto 5 de #114 aplicado a esta
		// rama, igual que ya se hace para el ReadAll de abajo: 5xx con log a
		// Error, y un mensaje que no culpa al cliente. Un cuerpo que no es
		// multipart, sin boundary o truncado no es *os.PathError y sigue
		// dando 400.
		if esFalloTemporal(err) {
			a.log.ErrorContext(r.Context(), "fallo al recibir la entrega multipart", slog.Any("error", err))
			escribirError(w, http.StatusInternalServerError, "no se pudo recibir la entrega por un fallo del servidor")
			return
		}
		escribirError(w, http.StatusBadRequest,
			"la entrega tiene que llegar como multipart/form-data con los campos fuente, periodo y archivo")
		return
	}
	// Los ficheros temporales que ParseMultipartForm deja en disco cuando la
	// subida no cabe en memoria. Sin esto se acumulan hasta que alguien
	// reinicia el proceso.
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	// Los campos del FORMULARIO, no de la query: `r.FormValue` consulta
	// primero la URL, asi que `POST /reportes?fuente=netflix` con un multipart
	// `fuente=cine` encaminaba el archivo al adaptador equivocado. `fuente`
	// decide el id del reporte, la clave de deduplicacion UNIQUE (sha256,
	// fuente) y el indice de alias_obra, asi que una entrega mal etiquetada es
	// cara de deshacer. El contrato declara los tres como propiedades
	// multipart (api/openapi.yaml).
	fuente := r.PostFormValue("fuente")
	periodo := r.PostFormValue("periodo")

	archivo, cabecera, err := r.FormFile("archivo")
	if err != nil {
		escribirError(w, http.StatusBadRequest, "falta el campo `archivo` con el reporte")
		return
	}
	defer func() { _ = archivo.Close() }()

	// El formato explicito gana al deducido. El nombre del fichero es una
	// conveniencia; un cliente que sepa lo que manda tiene que poder decirlo.
	formato := r.PostFormValue("formato")
	if formato == "" {
		formato = a.ingesta.DeducirFormato(cabecera.Filename)
	}

	// El tope DEL ARCHIVO, que es otra pregunta que la del cuerpo: el
	// MaxBytesReader de arriba acota la peticion entera -- envoltura incluida --
	// y esto acota la parte `archivo` sola. Dentro de un cuerpo permitido cabe un
	// archivo que se pasa, y es el que hay que nombrar en la respuesta.
	//
	// El +1 es para poder distinguir "justo el tope" de "se paso": LimitReader no
	// avisa, se queda callado en el limite.
	datos, err := leerCuerpoArchivo(io.LimitReader(archivo, tamanoMaximoEntrega+1))
	if err != nil {
		// 500 y no 400: a esta altura los bytes ya estan en RAM o en un
		// temporal del servidor, asi que es E/S del servidor, no peticion
		// malformada. Y no puede ser un MaxBytesError, porque el cuerpo ya
		// lo consumio ParseMultipartForm.
		a.log.ErrorContext(r.Context(), "fallo al leer la subida", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo leer el archivo subido")
		return
	}
	if len(datos) > tamanoMaximoEntrega {
		escribirError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("el archivo pasa de %d MiB", tamanoMaximoEntrega>>20))
		return
	}

	rec, err := a.ingesta.IngerirReporte(r.Context(), fuente, formato, periodo, datos)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrReporteInvalido):
		// 400 con el mensaje del nucleo TAL CUAL. Nombra la columna que falta o
		// el formato que no se reconoce, que es exactamente lo que hay que
		// volver a pedirle al cliente; sustituirlo por un texto generico
		// perderia la unica informacion util de la respuesta.
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrReporteDuplicado):
		// 409 y no 400: la entrega estaba bien formada, es que esos mismos
		// bytes de esa misma fuente ya llegaron. Lo decide el
		// UNIQUE (sha256, fuente), no el nombre del archivo.
		escribirError(w, http.StatusConflict, "esa fuente ya entrego exactamente ese archivo")
		return
	case errors.Is(err, aplicacion.ErrEvidenciaCorrupta):
		// 500 y no 409: bajo la clave de la boveda hay bytes que no son los
		// que dice la huella. No es culpa de quien sube y no se arregla
		// reintentando, pero tampoco es un conflicto que el cliente resuelva:
		// es un incidente de integridad del servidor, y ninguna alerta basada
		// en 5xx se dispararia con un 4xx. El mensaje se queda, que era bueno.
		a.log.ErrorContext(r.Context(), "evidencia corrupta en la boveda",
			slog.Any("error", err), slog.String("fuente", fuente))
		escribirError(w, http.StatusInternalServerError,
			"la boveda ya tiene contenido distinto bajo esa huella; avise a operacion")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al ingerir un reporte",
			slog.Any("error", err), slog.String("fuente", fuente), slog.String("periodo", periodo))
		escribirError(w, http.StatusInternalServerError, "no se pudo registrar la entrega")
		return
	}

	escribirJSON(w, http.StatusCreated, entregaJSON{
		ID:          rec.Reporte.ID,
		Fuente:      rec.Reporte.Fuente,
		Periodo:     rec.Reporte.Periodo,
		SHA256:      rec.Reporte.SHA256,
		ClaveObjeto: rec.Reporte.ClaveObjeto,
		NBytes:      rec.Reporte.NBytes,
		Aceptados:   rec.Aceptados,
		Rechazados:  aRechazosJSON(rec.Rechazados),
	})
}

// listarCargas sirve una pagina del listado de cargas hechas, opcionalmente
// de un periodo. Va paginado con la misma forma que `GET /obras` (limite y
// desplazamiento, mismo defecto y mismo techo): el listado crece sin cota con
// cada entrega, y cada fila trae dos subconsultas de recuento.
func (a *API) listarCargas(w http.ResponseWriter, r *http.Request) {
	pag, ok := leerPaginacion(w, r.URL.Query())
	if !ok {
		return
	}
	cargas, err := a.ingesta.Cargas(r.Context(), r.URL.Query().Get("periodo"), pag)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrReporteInvalido):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al listar cargas", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar las cargas")
		return
	}

	cuerpo := make([]cargaJSON, 0, len(cargas))
	for _, c := range cargas {
		cuerpo = append(cuerpo, cargaJSON{
			ID: c.ID, Fuente: c.Fuente, Periodo: c.Periodo, SHA256: c.SHA256,
			ClaveObjeto: c.ClaveObjeto, NBytes: c.NBytes, Recibido: c.Recibido,
			Aceptados: c.Aceptados, Rechazados: c.Rechazados,
		})
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}

// listarRechazosDeCarga sirve una pagina del log de rechazos de una carga: lo
// que despliega una fila del listado, que solo trae el recuento.
//
// Va paginado con la misma forma que `GET /obras` (limite y desplazamiento,
// mismo defecto y mismo techo): el log de una entrega grande -un archivo con la
// cabecera equivocada rechaza todas sus filas- no cabe en una respuesta ni en
// una tabla. La cifra no se trunca: el total es `Carga.rechazados`, que el
// listado ya tiene en la fila que se despliega, y es con lo que se dice "N de M".
//
// 404 y no una lista vacia cuando la carga no existe: "no llego" y "llego
// entera" son justo las dos respuestas que esta lectura tiene que separar. Una
// pagina vacia de una carga que si existe es 200 con la lista vacia.
func (a *API) listarRechazosDeCarga(w http.ResponseWriter, r *http.Request) {
	pag, ok := leerPaginacion(w, r.URL.Query())
	if !ok {
		return
	}
	usos, err := a.ingesta.RechazosDeCarga(r.Context(), chi.URLParam(r, "id"), pag)
	switch {
	case err == nil:
	case errors.Is(err, aplicacion.ErrReporteInvalido):
		escribirError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, aplicacion.ErrNoEncontrado):
		escribirError(w, http.StatusNotFound, "esa carga no existe")
		return
	default:
		a.log.ErrorContext(r.Context(), "fallo al consultar los rechazos de una carga", slog.Any("error", err))
		escribirError(w, http.StatusInternalServerError, "no se pudo consultar los rechazos de la carga")
		return
	}
	escribirJSON(w, http.StatusOK, aRechazosJSON(usos))
}
