package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// catalogoFalso registra lo que le llega y devuelve lo que le pongan. Es el
// doble que hace que estas pruebas comprueben el ADAPTADOR -codigos, cabeceras
// y forma del JSON- y no la base.
type catalogoFalso struct {
	obra  repertorio.Obra
	obras []repertorio.Obra
	err   error

	filtro     aplicacion.FiltroObras
	idRecibido string
	metadatos  repertorio.Metadatos
}

func (c *catalogoFalso) RegistrarObra(_ context.Context, id string, m repertorio.Metadatos) (repertorio.Obra, error) {
	c.idRecibido, c.metadatos = id, m
	return c.obra, c.err
}

func (c *catalogoFalso) ActualizarMetadatosObra(_ context.Context, id string, m repertorio.Metadatos) (repertorio.Obra, error) {
	c.idRecibido, c.metadatos = id, m
	return c.obra, c.err
}

func (c *catalogoFalso) ObraPorID(_ context.Context, id string) (repertorio.Obra, error) {
	c.idRecibido = id
	return c.obra, c.err
}

func (c *catalogoFalso) BuscarObras(_ context.Context, f aplicacion.FiltroObras) ([]repertorio.Obra, error) {
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

// servidorConCatalogo monta el router con una sesion de administrador ya
// resuelta, que es lo que exigen las cuatro rutas.
func servidorConCatalogo(t *testing.T, cat Catalogo) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	return Nueva(nil, auth, cat, Opciones{}).Router()
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
		h := Nueva(nil, auth, &catalogoFalso{}, Opciones{}).Router()
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
	}
	if cat.filtro != quiero {
		t.Fatalf("filtro = %+v, se esperaba %+v", cat.filtro, quiero)
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
	cat := &catalogoFalso{obra: obraDePrueba(t)}
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
	for _, campo := range []string{"id", "titulo", "genero", "anio", "tipo", "ida", "eidr", "imdb", "coautores"} {
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
	cat := &catalogoFalso{obra: obraDePrueba(t)}
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
	cat := &catalogoFalso{obra: obraDePrueba(t)}
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
	bruto, err := json.Marshal(aObraJSON(obraDePrueba(t)))
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
