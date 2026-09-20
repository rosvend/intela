package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// catalogoFalso registra lo que le llega y devuelve lo que le pongan. Es el
// doble que hace que estas pruebas comprueben el ADAPTADOR -codigos, cabeceras
// y forma del JSON- y no la base.
type catalogoFalso struct {
	obra  aplicacion.ObraDelCatalogo
	obras []aplicacion.ObraDelCatalogo
	err   error

	filtro     aplicacion.FiltroObras
	idRecibido string
	metadatos  repertorio.Metadatos
}

func (c *catalogoFalso) RegistrarObra(_ context.Context, id string, m repertorio.Metadatos) (aplicacion.ObraDelCatalogo, error) {
	c.idRecibido, c.metadatos = id, m
	return c.obra, c.err
}

func (c *catalogoFalso) ActualizarMetadatosObra(_ context.Context, id string, m repertorio.Metadatos) (aplicacion.ObraDelCatalogo, error) {
	c.idRecibido, c.metadatos = id, m
	return c.obra, c.err
}

func (c *catalogoFalso) ObraPorID(_ context.Context, id string) (aplicacion.ObraDelCatalogo, error) {
	c.idRecibido = id
	return c.obra, c.err
}

func (c *catalogoFalso) BuscarObras(_ context.Context, f aplicacion.FiltroObras) ([]aplicacion.ObraDelCatalogo, error) {
	c.filtro = f
	return c.obras, c.err
}

func obraDePrueba(t *testing.T) repertorio.Obra {
	t.Helper()

	o, err := repertorio.NuevaObra("obra-1", repertorio.Metadatos{
		Titulo: "La Casa de las Dos Palmas",
		Genero: "Drama",
		Anio:   1991,
		Tipo:   repertorio.TipoSerie,
		IDA:    "IDA-1",
		Coautores: []repertorio.Coautor{
			{Nombre: "Ana Escritora", IPI: "IPI-00000001", Rol: repertorio.RolGuionista},
			{Nombre: "Beto Libretista", IPI: "IPI-00000002", Rol: repertorio.RolLibretista},
		},
	})
	if err != nil {
		t.Fatalf("construir la obra de prueba: %v", err)
	}
	return o
}

// obraDelCatalogo es la obra de prueba con su declaracion vigente ya compuesta
// por el caso de uso: version 3, 60+40, completa.
func obraDelCatalogo(t *testing.T) aplicacion.ObraDelCatalogo {
	t.Helper()

	version := 3
	return aplicacion.ObraDelCatalogo{
		Obra:            obraDePrueba(t),
		EstadoDecl:      "completa",
		SumaPorcentajes: decimal.NewFromInt(100),
		VersionVigente:  &version,
	}
}

// obraSinDeclaracion es el otro extremo: la obra esta en el catalogo y no
// tiene ninguna declaracion. El estado sale de la Declaracion cero
// -"incompleta", R-04- y la version va a null, que es lo unico que permite
// distinguirla de una declarada a medias.
func obraSinDeclaracion(t *testing.T) aplicacion.ObraDelCatalogo {
	t.Helper()

	return aplicacion.ObraDelCatalogo{
		Obra:            obraDePrueba(t),
		EstadoDecl:      "incompleta",
		SumaPorcentajes: decimal.Zero,
	}
}

// servidorConCatalogo monta el router con una sesion de administrador ya
// resuelta, que es lo que exigen las cuatro rutas.
func servidorConCatalogo(t *testing.T, cat Catalogo) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	return Nueva(Casos{Auth: auth, Catalogo: cat}, Opciones{}).Router()
}

const cuerpoAlta = `{
  "id": "obra-1",
  "titulo": "La Casa de las Dos Palmas",
  "genero": "Drama",
  "anio": 1991,
  "tipo": "serie",
  "ida": "IDA-1",
  "coautores": [
    {"nombre": "Ana Escritora", "ipi": "IPI-00000001", "rol": "guionista"},
    {"nombre": "Beto Libretista", "ipi": "IPI-00000002", "rol": "libretista"}
  ]
}`

// ---------------------------------------------------------------------------
// Autorizacion: las cuatro rutas piden `administrador`

func TestElCatalogoExigeElRolAdministrador(t *testing.T) {
	peticiones := []struct{ metodo, ruta, cuerpo string }{
		{http.MethodGet, "/obras", ""},
		{http.MethodGet, "/obras/obra-1", ""},
		{http.MethodPost, "/obras", cuerpoAlta},
		{http.MethodPatch, "/obras/obra-1", cuerpoAlta},
	}
	roles := []aplicacion.Rol{
		aplicacion.RolDistribucion,
		aplicacion.RolContabilidad,
		aplicacion.RolAuditor,
		aplicacion.RolTitular,
	}

	for _, rol := range roles {
		auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol}}
		h := Nueva(Casos{Auth: auth, Catalogo: &catalogoFalso{}}, Opciones{}).Router()
		for _, p := range peticiones {
			t.Run(string(rol)+" "+p.metodo+" "+p.ruta, func(t *testing.T) {
				rec := pedir(t, h, p.metodo, p.ruta, p.cuerpo, "tok")
				if rec.Code != http.StatusForbidden {
					t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
				}
			})
		}
	}
}

// Sin sesion el corte es conSesion (401), no requiereRol (403).
func TestElCatalogoSinSesionEs401(t *testing.T) {
	h := servidorConCatalogo(t, &catalogoFalso{})

	rec := pedir(t, h, http.MethodGet, "/obras", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Busqueda

func TestBuscarObrasPasaLosCuatroFiltros(t *testing.T) {
	cat := &catalogoFalso{}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodGet,
		"/obras?titulo=palmas&genero=Drama&ipi=IPI-00000001&anio=1991", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	quiero := aplicacion.FiltroObras{
		Titulo: "palmas", Genero: "Drama", IPI: "IPI-00000001", Anio: 1991,
		Paginacion: aplicacion.Paginacion{Limite: aplicacion.LimiteObrasPorDefecto},
	}
	if cat.filtro != quiero {
		t.Fatalf("filtro = %+v, se esperaba %+v", cat.filtro, quiero)
	}
}

func TestBuscarObrasPasaLimiteYDesplazamiento(t *testing.T) {
	cat := &catalogoFalso{}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodGet, "/obras?limite=10&desplazamiento=20", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if cat.filtro.Limite != 10 || cat.filtro.Desplazamiento != 20 {
		t.Fatalf("paginacion = {%d,%d}, se esperaba {10,20}",
			cat.filtro.Limite, cat.filtro.Desplazamiento)
	}
}

func TestBuscarObrasRechazaPaginacionInvalida(t *testing.T) {
	h := servidorConCatalogo(t, &catalogoFalso{})

	casos := []struct{ query, trozo string }{
		{"limite=0", "limite"},
		{"limite=-1", "limite"},
		{"limite=abc", "limite"},
		{"limite=501", "limite"},
		{"desplazamiento=-1", "desplazamiento"},
		{"desplazamiento=x", "desplazamiento"},
	}
	for _, c := range casos {
		t.Run(c.query, func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, "/obras?"+c.query, "", "tok")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

// Sin coincidencias tiene que salir [] y no null, o cualquier cliente que
// itere la respuesta revienta.
func TestBuscarObrasSinCoincidenciasDevuelveListaVacia(t *testing.T) {
	h := servidorConCatalogo(t, &catalogoFalso{})

	rec := pedir(t, h, http.MethodGet, "/obras", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200", rec.Code)
	}
	if cuerpo := rec.Body.String(); cuerpo != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba \"[]\"", cuerpo)
	}
}

// Un anio que no es un numero se rechaza en vez de ignorarse: ignorado
// devolveria el catalogo entero, y quien pregunta lo leeria como "no hay
// ninguna de ese anio".
func TestBuscarObrasRechazaUnAnioQueNoEsNumero(t *testing.T) {
	h := servidorConCatalogo(t, &catalogoFalso{})

	for _, anio := range []string{"dosmil", "0", "-1991", "1991.5"} {
		t.Run(anio, func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, "/obras?anio="+anio, "", "tok")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400", rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Lectura por id

func TestObraPorIDDevuelveLaFormaDelContrato(t *testing.T) {
	cat := &catalogoFalso{obra: obraDelCatalogo(t)}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodGet, "/obras/obra-1", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if cat.idRecibido != "obra-1" {
		t.Fatalf("id recibido = %q", cat.idRecibido)
	}

	cuerpo := decodificar(t, rec)
	// Los nombres de los campos SON el contrato: cuadran con el schema Obra de
	// api/openapi.yaml, y el id sale al mismo nivel que los metadatos.
	for _, campo := range []string{
		"id", "titulo", "genero", "anio", "tipo", "ida", "eidr", "imdb", "coautores",
		"estado_declaracion", "suma_porcentajes", "version_vigente",
	} {
		if _, hay := cuerpo[campo]; !hay {
			t.Fatalf("falta el campo %q en la respuesta: %s", campo, rec.Body)
		}
	}
	if cuerpo["id"] != "obra-1" || cuerpo["genero"] != "Drama" {
		t.Fatalf("cuerpo = %v", cuerpo)
	}
	if anio, _ := cuerpo["anio"].(float64); anio != 1991 {
		t.Fatalf("anio = %v", cuerpo["anio"])
	}
	// El porcentaje y su suma viajan como NUMERO, sin comillas (igual que
	// `porcentaje` en la declaracion): el cliente decide los decimales que
	// muestra.
	if suma, esNumero := cuerpo["suma_porcentajes"].(float64); !esNumero || suma != 100 {
		t.Fatalf("suma_porcentajes = %#v, se esperaba el numero 100", cuerpo["suma_porcentajes"])
	}
	if version, esNumero := cuerpo["version_vigente"].(float64); !esNumero || version != 3 {
		t.Fatalf("version_vigente = %#v, se esperaba el numero 3", cuerpo["version_vigente"])
	}
	if cuerpo["estado_declaracion"] != "completa" {
		t.Fatalf("estado_declaracion = %v", cuerpo["estado_declaracion"])
	}

	coautores, _ := cuerpo["coautores"].([]any)
	if len(coautores) != 2 {
		t.Fatalf("se esperaban 2 coautores: %s", rec.Body)
	}
	primero, _ := coautores[0].(map[string]any)
	if primero["ipi"] != "IPI-00000001" || primero["rol"] != "guionista" {
		t.Fatalf("coautor = %v", primero)
	}
	// El catalogo no reparte: la forma de red no tiene donde poner un
	// porcentaje (`R-02`, `R-03`).
	if _, hay := primero["porcentaje"]; hay {
		t.Fatal("un coautor del catalogo no puede llevar porcentaje")
	}
}

func TestObraPorIDNoEncontradaEs404(t *testing.T) {
	cat := &catalogoFalso{err: aplicacion.ErrNoEncontrado}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodGet, "/obras/obra-1", "", "tok")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Alta

func TestRegistrarObraDevuelve201YLocation(t *testing.T) {
	cat := &catalogoFalso{obra: obraDelCatalogo(t)}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodPost, "/obras", cuerpoAlta, "tok")

	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/obras/obra-1" {
		t.Fatalf("Location = %q", loc)
	}
	if cat.idRecibido != "obra-1" {
		t.Fatalf("id recibido = %q", cat.idRecibido)
	}

	// El cuerpo llego entero hasta el nucleo, coautores incluidos, y con el
	// rol autoral tipado.
	m := cat.metadatos
	if m.Titulo != "La Casa de las Dos Palmas" || m.Genero != "Drama" || m.Anio != 1991 {
		t.Fatalf("metadatos = %+v", m)
	}
	if m.Tipo != repertorio.TipoSerie {
		t.Fatalf("Tipo = %q", m.Tipo)
	}
	if len(m.Coautores) != 2 || m.Coautores[1].Rol != repertorio.RolLibretista {
		t.Fatalf("coautores = %+v", m.Coautores)
	}
}

// El caso de uso valida con el dominio; el adaptador solo traduce el
// centinela. 400 y no 500: los datos llegaron, no forman una obra.
func TestRegistrarObraInvalidaEs400(t *testing.T) {
	cat := &catalogoFalso{err: repertorio.ErrObraInvalida}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodPost, "/obras", cuerpoAlta, "tok")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// El criterio del issue por la puerta HTTP: el segundo alta con el mismo
// identificador se rechaza, y se distingue de un fallo de la base.
func TestRegistrarObraDuplicadaEs409(t *testing.T) {
	cat := &catalogoFalso{err: aplicacion.ErrObraDuplicada}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodPost, "/obras", cuerpoAlta, "tok")

	if rec.Code != http.StatusConflict {
		t.Fatalf("codigo = %d, se esperaba 409. Cuerpo: %s", rec.Code, rec.Body)
	}
	if cuerpo := decodificar(t, rec); cuerpo["error"] != aplicacion.ErrObraDuplicada.Error() {
		t.Fatalf("error = %v", cuerpo["error"])
	}
}

func TestRegistrarObraConCuerpoQueNoEsJSONEs400(t *testing.T) {
	h := servidorConCatalogo(t, &catalogoFalso{})

	rec := pedir(t, h, http.MethodPost, "/obras", "esto no es json", "tok")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Correccion de metadatos

// La propiedad que da nombre al issue: el identificador NO se puede cambiar.
// El cuerpo del PATCH se decodifica en un struct que no tiene campo id, asi
// que un "id" en el JSON se ignora y el que llega al nucleo es el de la ruta.
func TestActualizarObraIgnoraCualquierIDDelCuerpo(t *testing.T) {
	cat := &catalogoFalso{obra: obraDelCatalogo(t)}
	h := servidorConCatalogo(t, cat)

	cuerpo := `{
	  "id": "obra-secuestrada",
	  "titulo": "Otro titulo",
	  "genero": "Comedia",
	  "anio": 2001,
	  "tipo": "telenovela",
	  "coautores": [{"nombre": "Ana", "ipi": "IPI-1", "rol": "adaptador"}]
	}`
	rec := pedir(t, h, http.MethodPatch, "/obras/obra-1", cuerpo, "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if cat.idRecibido != "obra-1" {
		t.Fatalf("id recibido = %q: el id del cuerpo no puede ganarle al de la ruta", cat.idRecibido)
	}
	if cat.metadatos.Titulo != "Otro titulo" || cat.metadatos.Anio != 2001 {
		t.Fatalf("metadatos = %+v", cat.metadatos)
	}
}

func TestActualizarObraQueNoExisteEs404(t *testing.T) {
	cat := &catalogoFalso{err: aplicacion.ErrNoEncontrado}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodPatch, "/obras/obra-1", cuerpoAlta, "tok")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404", rec.Code)
	}
}

func TestActualizarObraInvalidaEs400(t *testing.T) {
	cat := &catalogoFalso{err: repertorio.ErrObraInvalida}
	h := servidorConCatalogo(t, cat)

	rec := pedir(t, h, http.MethodPatch, "/obras/obra-1", cuerpoAlta, "tok")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Forma de red

// El struct de red mete el id y los metadatos al mismo nivel. Si el embebido
// dejara de aplanarse, el JSON saldria anidado y el contrato dejaria de
// cuadrar sin que nada mas se rompa.
func TestLaObraSerializaPlana(t *testing.T) {
	bruto, err := json.Marshal(aObraJSON(obraDelCatalogo(t)))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(bruto, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, anidado := m["metadatosJSON"]; anidado {
		t.Fatalf("el JSON salio anidado: %s", bruto)
	}
	if m["id"] != "obra-1" || m["titulo"] != "La Casa de las Dos Palmas" {
		t.Fatalf("json = %s", bruto)
	}
}

// El contrato declara `suma_porcentajes` como `number`, y la libreria de
// decimales serializa ENTRE COMILLAS por defecto. La prueba mira los BYTES que
// viajan -que es lo unico que ve un cliente-, porque una cadena "100" y un
// numero 100 se leen igual de bien al decodificar el JSON pero no son lo
// mismo: contra la cadena no se puede restar sin parsearla antes.
func TestLaSumaDePorcentajesViajaComoNumero(t *testing.T) {
	casos := []struct {
		nombre string
		obra   aplicacion.ObraDelCatalogo
		quiero string
	}{
		{"100 sin comillas", obraDelCatalogo(t), `"suma_porcentajes":100`},
		{"cero sin comillas", obraSinDeclaracion(t), `"suma_porcentajes":0`},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bruto, err := json.Marshal(aObraJSON(c.obra))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(bruto), c.quiero) {
				t.Fatalf("el JSON no trae %s: %s", c.quiero, bruto)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// El estado de la declaracion en las cuatro respuestas (D-008)

// obraDeRespuesta devuelve la obra del cuerpo, sea la respuesta un objeto
// -GET /obras/{id}, POST, PATCH- o una lista de una sola obra -GET /obras-.
// Las cuatro llevan el mismo schema, asi que la comprobacion tiene que ser la
// misma para las cuatro.
func obraDeRespuesta(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var bruto any
	if err := json.Unmarshal(rec.Body.Bytes(), &bruto); err != nil {
		t.Fatalf("el cuerpo no es JSON: %v (%q)", err, rec.Body.String())
	}
	switch v := bruto.(type) {
	case map[string]any:
		return v
	case []any:
		if len(v) != 1 {
			t.Fatalf("se esperaba una obra en la lista, llegaron %d: %s", len(v), rec.Body)
		}
		obra, ok := v[0].(map[string]any)
		if !ok {
			t.Fatalf("la lista no trae objetos: %s", rec.Body)
		}
		return obra
	default:
		t.Fatalf("cuerpo inesperado: %s", rec.Body)
		return nil
	}
}

// El schema `Obra` sirve a CUATRO respuestas -GET /obras, GET /obras/{id},
// POST /obras y PATCH /obras/{id}- y los tres campos nuevos son obligatorios
// en las cuatro. Si una sola se dejara sin ellos, el contrato prometeria algo
// que esa respuesta no trae, que es el fallo caro de este paso.
//
// Las dos de escritura son las peligrosas: no llevaban esos campos antes de
// este cambio y nadie los echaria de menos leyendo su prueba vieja.
func TestLasCuatroRespuestasLlevanElEstadoDeLaDeclaracion(t *testing.T) {
	peticiones := []struct {
		nombre string
		metodo string
		ruta   string
		cuerpo string
		codigo int
	}{
		{"GET /obras", http.MethodGet, "/obras", "", http.StatusOK},
		{"GET /obras/{id}", http.MethodGet, "/obras/obra-1", "", http.StatusOK},
		{"POST /obras", http.MethodPost, "/obras", cuerpoAlta, http.StatusCreated},
		{"PATCH /obras/{id}", http.MethodPatch, "/obras/obra-1", cuerpoAlta, http.StatusOK},
	}

	for _, p := range peticiones {
		t.Run(p.nombre, func(t *testing.T) {
			cat := &catalogoFalso{
				obra:  obraDelCatalogo(t),
				obras: []aplicacion.ObraDelCatalogo{obraDelCatalogo(t)},
			}
			h := servidorConCatalogo(t, cat)

			rec := pedir(t, h, p.metodo, p.ruta, p.cuerpo, "tok")
			if rec.Code != p.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, p.codigo, rec.Body)
			}

			cuerpo := obraDeRespuesta(t, rec)
			for _, campo := range []string{"estado_declaracion", "suma_porcentajes", "version_vigente"} {
				if _, hay := cuerpo[campo]; !hay {
					t.Fatalf("%s no trae %q: %s", p.nombre, campo, rec.Body)
				}
			}
			if cuerpo["estado_declaracion"] != "completa" {
				t.Fatalf("estado_declaracion = %v, se esperaba completa. Cuerpo: %s",
					cuerpo["estado_declaracion"], rec.Body)
			}
			if suma, esNumero := cuerpo["suma_porcentajes"].(float64); !esNumero || suma != 100 {
				t.Fatalf("suma_porcentajes = %#v, se esperaba el numero 100", cuerpo["suma_porcentajes"])
			}
			if version, esNumero := cuerpo["version_vigente"].(float64); !esNumero || version != 3 {
				t.Fatalf("version_vigente = %#v, se esperaba el numero 3", cuerpo["version_vigente"])
			}
		})
	}
}

// La otra mitad del mismo contrato: una obra SIN declaracion tiene que traer
// `version_vigente` PRESENTE y en null. Ausente y null no son lo mismo para un
// cliente -una clave ausente no distingue "no lo mire" de "no hay"-, y
// `version_vigente` es justo el campo que dice que no hay ninguna.
func TestUnaObraSinDeclaracionLlevaVersionVigenteNula(t *testing.T) {
	cat := &catalogoFalso{
		obra:  obraSinDeclaracion(t),
		obras: []aplicacion.ObraDelCatalogo{obraSinDeclaracion(t)},
	}
	h := servidorConCatalogo(t, cat)

	for _, ruta := range []string{"/obras/obra-1", "/obras"} {
		t.Run(ruta, func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, ruta, "", "tok")
			if rec.Code != http.StatusOK {
				t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
			}
			cuerpo := obraDeRespuesta(t, rec)

			version, hay := cuerpo["version_vigente"]
			if !hay {
				t.Fatalf("falta version_vigente: una obra sin declaracion tiene que decir null, no callarse. Cuerpo: %s", rec.Body)
			}
			if version != nil {
				t.Fatalf("version_vigente = %v, se esperaba null", version)
			}
			if cuerpo["estado_declaracion"] != "incompleta" {
				t.Fatalf("estado_declaracion = %v, se esperaba incompleta (R-04)",
					cuerpo["estado_declaracion"])
			}
			if suma, esNumero := cuerpo["suma_porcentajes"].(float64); !esNumero || suma != 0 {
				t.Fatalf("suma_porcentajes = %#v, se esperaba 0", cuerpo["suma_porcentajes"])
			}
		})
	}
}

// El estado NO entra por el cuerpo. Un alta decodifica en un struct que no
// tiene esos tres campos, asi que un `estado_declaracion: completa` en el JSON
// se descarta y la respuesta dice lo que el sistema sabe -lo que devuelve el
// caso de uso-, no lo que mando el cliente. Sin esto, el contrato estaria
// prometiendo una facultad que nadie tiene: declarar el estado a mano.
func TestElEstadoDelCuerpoNoSeAceptaEnElAlta(t *testing.T) {
	cat := &catalogoFalso{obra: obraSinDeclaracion(t)}
	h := servidorConCatalogo(t, cat)

	cuerpoSecuestrado := `{
	  "id": "obra-1",
	  "titulo": "La Casa de las Dos Palmas",
	  "genero": "Drama",
	  "anio": 1991,
	  "tipo": "serie",
	  "coautores": [{"nombre": "Ana Escritora", "ipi": "IPI-00000001", "rol": "guionista"}],
	  "estado_declaracion": "completa",
	  "suma_porcentajes": 100,
	  "version_vigente": 9
	}`
	rec := pedir(t, h, http.MethodPost, "/obras", cuerpoSecuestrado, "tok")

	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	respuesta := decodificar(t, rec)
	if respuesta["estado_declaracion"] != "incompleta" {
		t.Fatalf("el estado del cuerpo gano: %v", respuesta["estado_declaracion"])
	}
	if version, hay := respuesta["version_vigente"]; !hay || version != nil {
		t.Fatalf("version_vigente = %#v: el cuerpo no puede inventar una version", respuesta["version_vigente"])
	}
}
