package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// ingestaFalsa registra lo que le llega y devuelve lo que le pongan. Es lo que
// hace que estas pruebas comprueben el ADAPTADOR -codigos, cabeceras y forma
// del JSON- y no la boveda ni la base.
type ingestaFalsa struct {
	rec    aplicacion.Recepcion
	cargas []aplicacion.CargaReporte
	err    error

	fuente            string
	formato           string
	periodo           string
	datos             []byte
	periodoConsultado string
}

func (i *ingestaFalsa) IngerirReporte(_ context.Context, fuente, formato, periodo string, datos []byte) (aplicacion.Recepcion, error) {
	i.fuente, i.formato, i.periodo, i.datos = fuente, formato, periodo, datos
	return i.rec, i.err
}

func (i *ingestaFalsa) Cargas(_ context.Context, periodo string) ([]aplicacion.CargaReporte, error) {
	i.periodoConsultado = periodo
	return i.cargas, i.err
}

func servidorConIngesta(t *testing.T, ing Ingesta) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	return Nueva(Casos{Auth: auth, Ingesta: ing}, Opciones{}).Router()
}

// subir compone una peticion multipart como la del formulario de subida.
func subir(t *testing.T, h http.Handler, campos map[string]string, nombre string, contenido []byte) *httptest.ResponseRecorder {
	t.Helper()

	var cuerpo bytes.Buffer
	escritor := multipart.NewWriter(&cuerpo)
	for k, v := range campos {
		if err := escritor.WriteField(k, v); err != nil {
			t.Fatalf("escribir el campo %q: %v", k, err)
		}
	}
	if nombre != "" {
		parte, err := escritor.CreateFormFile("archivo", nombre)
		if err != nil {
			t.Fatalf("crear la parte del archivo: %v", err)
		}
		if _, err := parte.Write(contenido); err != nil {
			t.Fatalf("escribir el archivo: %v", err)
		}
	}
	if err := escritor.Close(); err != nil {
		t.Fatalf("cerrar el multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/reportes", &cuerpo)
	req.Header.Set("Content-Type", escritor.FormDataContentType())
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func recepcionDePrueba() aplicacion.Recepcion {
	mala := aplicacion.UsoPersistido{
		ID: "rep-1-4", Titulo: "Duracion rota", IDsFuente: "99999",
		RechazoMotivo: `fila 6, duracion_min (columna "Duracion_total"): "cuarenta y cinco" no es un numero`,
	}
	return aplicacion.Recepcion{
		Reporte: aplicacion.Reporte{
			ID: "rep-1", Fuente: "caracol", Periodo: "2026-01",
			SHA256: strings.Repeat("a", 64), ClaveObjeto: "reportes/" + strings.Repeat("a", 64),
			NBytes: 21032,
		},
		Aceptados:  58,
		Rechazados: []aplicacion.UsoPersistido{mala},
	}
}

// ---------------------------------------------------------------------------
// Autorizacion

func TestLosReportesExigenElRolAdministrador(t *testing.T) {
	// Una entrega pondera el reparto de un periodo entero. Las dos rutas van en
	// el mismo grupo de rol que el catalogo.
	for _, rol := range []aplicacion.Rol{
		aplicacion.RolDistribucion,
		aplicacion.RolContabilidad,
		aplicacion.RolAuditor,
		aplicacion.RolTitular,
	} {
		auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol}}
		h := Nueva(Casos{Auth: auth, Ingesta: &ingestaFalsa{}}, Opciones{}).Router()

		if rec := pedir(t, h, http.MethodGet, "/reportes", "", "tok"); rec.Code != http.StatusForbidden {
			t.Errorf("%s GET: codigo = %d, se esperaba 403", rol, rec.Code)
		}
		if rec := subir(t, h, map[string]string{"fuente": "caracol"}, "x.csv", []byte("a")); rec.Code != http.StatusForbidden {
			t.Errorf("%s POST: codigo = %d, se esperaba 403", rol, rec.Code)
		}
	}
}

func TestLosReportesSinSesionSon401(t *testing.T) {
	h := servidorConIngesta(t, &ingestaFalsa{})
	if rec := pedir(t, h, http.MethodGet, "/reportes", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}

func TestLasRutasDeReportesSon503SiElBinarioNoCableaLaIngesta(t *testing.T) {
	// cmd/lambda arranca asi a proposito: su sistema de ficheros no sirve de
	// boveda. Sin esta guarda el handler llamaria a una interfaz nil y el
	// Recoverer devolveria un 500 sin cuerpo, que se lee como "el servidor esta
	// roto" en vez de "esta instalacion no tiene ingesta".
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	h := Nueva(Casos{Auth: auth}, Opciones{}).Router()

	if rec := pedir(t, h, http.MethodGet, "/reportes", "", "tok"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET: codigo = %d, se esperaba 503. Cuerpo: %s", rec.Code, rec.Body)
	}
	if rec := subir(t, h, map[string]string{"fuente": "caracol"}, "x.csv", []byte("a")); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("POST: codigo = %d, se esperaba 503. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// ---------------------------------------------------------------------------
// POST /reportes

func TestSubirReporteDevuelve201ConElAcuseYLosRechazos(t *testing.T) {
	ing := &ingestaFalsa{rec: recepcionDePrueba()}
	h := servidorConIngesta(t, ing)

	rec := subir(t, h,
		map[string]string{"fuente": "caracol", "periodo": "2026-01"},
		"CARACOL_REDES-SGC_(COLOMBIA)_20250202.xlsx", []byte("PK\x03\x04..."))
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}

	// El formato sale de la extension del nombre del fichero cuando no viene
	// explicito.
	if ing.formato != aplicacion.FormatoXLSX {
		t.Errorf("formato = %q, se esperaba %q", ing.formato, aplicacion.FormatoXLSX)
	}
	if ing.fuente != "caracol" || ing.periodo != "2026-01" {
		t.Errorf("fuente/periodo = %q/%q", ing.fuente, ing.periodo)
	}
	if string(ing.datos) != "PK\x03\x04..." {
		t.Errorf("los bytes llegaron cambiados: %q", ing.datos)
	}

	cuerpo := decodificar(t, rec)
	// El acuse tiene que traer la huella y la clave: sin ellas, la respuesta no
	// permite volver a la evidencia exacta (ADR 0006).
	if cuerpo["sha256"] != strings.Repeat("a", 64) {
		t.Errorf("sha256 = %v", cuerpo["sha256"])
	}
	if cuerpo["aceptados"] != float64(58) {
		t.Errorf("aceptados = %v", cuerpo["aceptados"])
	}

	rechazos, ok := cuerpo["rechazados"].([]any)
	if !ok || len(rechazos) != 1 {
		t.Fatalf("rechazados = %v", cuerpo["rechazados"])
	}
	// El motivo viaja entero: quien sube el archivo tiene que poder leer que
	// linea le falta sin abrir la base.
	motivo, _ := rechazos[0].(map[string]any)["motivo"].(string)
	if !strings.Contains(motivo, "Duracion_total") {
		t.Errorf("motivo = %q", motivo)
	}
}

func TestSubirReporteSinRechazosDevuelveListaVaciaYNoNull(t *testing.T) {
	entrega := recepcionDePrueba()
	entrega.Rechazados = nil
	h := servidorConIngesta(t, &ingestaFalsa{rec: entrega})

	rec := subir(t, h, map[string]string{"fuente": "caracol", "periodo": "2026-01"},
		"parrilla.xlsx", []byte("x"))
	// `null` revienta a cualquier cliente que itere la lista, y justo en el
	// caso bueno.
	if !strings.Contains(rec.Body.String(), `"rechazados":[]`) {
		t.Fatalf("cuerpo = %s", rec.Body)
	}
}

func TestSubirReporteRespetaElFormatoExplicito(t *testing.T) {
	// Un cliente que sepa lo que manda tiene que poder decirlo: el nombre del
	// fichero es una conveniencia, no la fuente de verdad.
	ing := &ingestaFalsa{rec: recepcionDePrueba()}
	h := servidorConIngesta(t, ing)

	subir(t, h, map[string]string{
		"fuente": "caracol", "periodo": "2026-01", "formato": aplicacion.FormatoCSV,
	}, "sin_extension_util.dat", []byte("titulo\n"))

	if ing.formato != aplicacion.FormatoCSV {
		t.Fatalf("formato = %q, se esperaba %q", ing.formato, aplicacion.FormatoCSV)
	}
}

func TestSubirReporteTraduceLosErroresDelNucleo(t *testing.T) {
	casos := []struct {
		nombre   string
		err      error
		codigo   int
		enCuerpo string
	}{
		{
			nombre: "columna que falta",
			err: fmt.Errorf("%w: a la entrega de \"caracol\" le faltan columnas requeridas: Duracion_total",
				aplicacion.ErrReporteInvalido),
			codigo: http.StatusBadRequest,
			// El mensaje del nucleo pasa TAL CUAL: nombra la columna, que es lo
			// que hay que volver a pedirle al cliente.
			enCuerpo: "Duracion_total",
		},
		{
			nombre:   "misma huella de la misma fuente",
			err:      fmt.Errorf("registrar la entrega: %w", aplicacion.ErrReporteDuplicado),
			codigo:   http.StatusConflict,
			enCuerpo: "ya entrego",
		},
		{
			nombre:   "boveda con contenido ajeno",
			err:      fmt.Errorf("%w: bajo la clave hay otros bytes", aplicacion.ErrEvidenciaCorrupta),
			codigo:   http.StatusConflict,
			enCuerpo: "avise a operacion",
		},
		{
			nombre:   "fallo de infraestructura",
			err:      fmt.Errorf("la base no responde"),
			codigo:   http.StatusInternalServerError,
			enCuerpo: "no se pudo registrar la entrega",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConIngesta(t, &ingestaFalsa{err: c.err})
			rec := subir(t, h, map[string]string{"fuente": "caracol", "periodo": "2026-01"},
				"parrilla.xlsx", []byte("x"))
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.codigo, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), c.enCuerpo) {
				t.Errorf("el cuerpo no dice %q: %s", c.enCuerpo, rec.Body)
			}
		})
	}
}

func TestSubirReporteSinArchivoEs400(t *testing.T) {
	h := servidorConIngesta(t, &ingestaFalsa{rec: recepcionDePrueba()})

	rec := subir(t, h, map[string]string{"fuente": "caracol", "periodo": "2026-01"}, "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestSubirReporteConCuerpoQueNoEsMultipartEs400(t *testing.T) {
	h := servidorConIngesta(t, &ingestaFalsa{rec: recepcionDePrueba()})

	// Sin esto, ParseMultipartForm falla y el Recoverer lo convierte en un 500.
	rec := pedir(t, h, http.MethodPost, "/reportes", `{"fuente":"caracol"}`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// relleno produce n bytes sin materializarlos. Un cuerpo de prueba de 128 MiB
// dentro de un []byte serian 128 MiB de RAM por corrida, y lo que hay que
// comprobar es justamente que el servidor NO los lee.
type relleno struct{ quedan int64 }

func (r *relleno) Read(p []byte) (int, error) {
	if r.quedan <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.quedan {
		p = p[:r.quedan]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.quedan -= int64(len(p))
	return len(p), nil
}

// contador anota cuanto del cuerpo llego a leer el servidor. Es la unica forma
// de ver la diferencia entre cortar la entrada y tragarsela entera para
// rechazarla despues: el codigo de respuesta es el mismo en los dos casos.
type contador struct {
	r      io.Reader
	leidos int64
}

func (c *contador) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.leidos += int64(n)
	return n, err
}

// El tope de la subida tiene que aplicarse EN LA ENTRADA, no despues de haber
// recibido el archivo.
//
// Medido antes del arreglo: una peticion de 40 MiB hacia que ParseMultipartForm
// derramara ~40 MB a disco, y una de 4 GB los habria derramado igual. La
// comprobacion de `len(datos) > tamanoMaximoEntrega` que habia contestaba 413,
// pero solo cuando el archivo ya estaba escrito: es una regla de negocio sobre
// algo que ya llego, no una defensa.
//
// Por eso la asercion que carga con la prueba es la de bytes leidos y no la del
// codigo: sin MaxBytesReader el codigo sigue siendo 413.
func TestSubirReporteCortaElCuerpoQueSePasaSinLeerloEntero(t *testing.T) {
	const frontera = "FRONTERA"
	cabecera := "--" + frontera + "\r\n" +
		"Content-Disposition: form-data; name=\"fuente\"\r\n\r\ncaracol\r\n" +
		"--" + frontera + "\r\n" +
		"Content-Disposition: form-data; name=\"periodo\"\r\n\r\n2026-01\r\n" +
		"--" + frontera + "\r\n" +
		"Content-Disposition: form-data; name=\"archivo\"; filename=\"gigante.csv\"\r\n" +
		"Content-Type: application/octet-stream\r\n\r\n"
	pie := "\r\n--" + frontera + "--\r\n"

	// Cuatro veces el tope. El numero no importa mientras sea mucho mayor: lo
	// que se comprueba es que lo leido NO crece con el.
	cuerpo := &contador{r: io.MultiReader(
		strings.NewReader(cabecera),
		&relleno{quedan: 4 * tamanoMaximoCuerpo},
		strings.NewReader(pie),
	)}

	ing := &ingestaFalsa{rec: recepcionDePrueba()}
	h := servidorConIngesta(t, ing)

	req := httptest.NewRequest(http.MethodPost, "/reportes", cuerpo)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+frontera)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("codigo = %d, se esperaba 413. Cuerpo: %s", rec.Code, rec.Body)
	}
	// Y con 413 y no con el 400 de "esto no es multipart", que es lo que
	// contestaria un ParseMultipartForm cuyo error no se distingue: quien sube
	// leeria que mande un multipart habiendo mandado uno.
	if !strings.Contains(rec.Body.String(), "pasa de") {
		t.Errorf("el cuerpo no dice que se paso del tope: %s", rec.Body)
	}
	// Nada de esto llego al nucleo, asi que no hay huella que quemar ni objeto
	// que escribir en la boveda.
	if ing.datos != nil {
		t.Errorf("la entrega llego al nucleo: %d bytes", len(ing.datos))
	}
	// El margen sobre el tope es el bufer con el que el parseo lee: se corta EN
	// el limite, no en el byte exacto.
	if tope := int64(tamanoMaximoCuerpo) + 1<<20; cuerpo.leidos > tope {
		t.Errorf("se leyeron %d bytes del cuerpo con un tope de %d: el limite no corta la entrada",
			cuerpo.leidos, tamanoMaximoCuerpo)
	}
}

// ---------------------------------------------------------------------------
// GET /reportes

func TestListarCargasSirveElListadoDeCargasHechas(t *testing.T) {
	ing := &ingestaFalsa{cargas: []aplicacion.CargaReporte{{
		Reporte: aplicacion.Reporte{
			ID: "rep-1", Fuente: "caracol", Periodo: "2026-01",
			SHA256: strings.Repeat("a", 64), NBytes: 21032,
		},
		Recibido:   time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC),
		Aceptados:  58,
		Rechazados: 1,
	}}}
	h := servidorConIngesta(t, ing)

	rec := pedir(t, h, http.MethodGet, "/reportes?periodo=2026-01", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ing.periodoConsultado != "2026-01" {
		t.Errorf("periodo consultado = %q", ing.periodoConsultado)
	}

	var cuerpo []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("el cuerpo no es JSON: %v (%s)", err, rec.Body)
	}
	if len(cuerpo) != 1 {
		t.Fatalf("cargas = %d", len(cuerpo))
	}
	// Los dos recuentos son el punto del listado: responden "¿entro completo lo
	// que subi?".
	if cuerpo[0]["aceptados"] != float64(58) || cuerpo[0]["rechazados"] != float64(1) {
		t.Errorf("recuentos = %v / %v", cuerpo[0]["aceptados"], cuerpo[0]["rechazados"])
	}
	if cuerpo[0]["periodo"] != "2026-01" {
		t.Errorf("periodo = %v", cuerpo[0]["periodo"])
	}
}

func TestListarCargasSinNingunaDevuelveListaVaciaYNoNull(t *testing.T) {
	h := servidorConIngesta(t, &ingestaFalsa{})

	rec := pedir(t, h, http.MethodGet, "/reportes", "", "tok")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("codigo = %d, cuerpo = %s", rec.Code, rec.Body)
	}
}

func TestListarCargasConPeriodoMalEscritoEs400(t *testing.T) {
	// El nucleo lo rechaza; el adaptador tiene que decirlo con 400 y no con un
	// 500 mudo.
	h := servidorConIngesta(t, &ingestaFalsa{
		err: fmt.Errorf("%w: periodo \"2026-1\"", aplicacion.ErrReporteInvalido),
	})

	rec := pedir(t, h, http.MethodGet, "/reportes?periodo=2026-1", "", "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}
