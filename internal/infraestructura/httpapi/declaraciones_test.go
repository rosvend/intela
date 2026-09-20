package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// declaracionesFalso registra lo que le llega y devuelve lo que le pongan,
// igual que catalogoFalso: estas pruebas comprueban el ADAPTADOR, no la base.
type declaracionesFalso struct {
	version   aplicacion.VersionDeclaracion
	historial []aplicacion.VersionDeclaracion
	err       error

	obraIDRecibida  string
	partesRecibidas []repertorio.Parte
	actorRecibido   string
}

func (d *declaracionesFalso) GuardarSplits(_ context.Context, obraID string, partes []repertorio.Parte, actorID string) (aplicacion.VersionDeclaracion, error) {
	d.obraIDRecibida, d.partesRecibidas, d.actorRecibido = obraID, partes, actorID
	return d.version, d.err
}

func (d *declaracionesFalso) Historial(_ context.Context, obraID string) ([]aplicacion.VersionDeclaracion, error) {
	d.obraIDRecibida = obraID
	return d.historial, d.err
}

// servidorConDeclaraciones monta el router con una sesion de administrador ya
// resuelta, que es lo que exigen las tres rutas.
func servidorConDeclaraciones(t *testing.T, d Declaraciones) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	return Nueva(Casos{Auth: auth, Declaraciones: d}, Opciones{}).Router()
}

const cuerpoPartes = `[
  {"titular_id": "t1", "ipi": "IPI-1", "porcentaje": 60},
  {"titular_id": "t2", "ipi": "IPI-2", "porcentaje": 40}
]`

func TestDeclaracionesExigenElRolAdministrador(t *testing.T) {
	peticiones := []struct{ metodo, ruta, cuerpo string }{
		{http.MethodPost, "/obras/obra-1/declaracion", cuerpoPartes},
		{http.MethodPut, "/obras/obra-1/declaracion", cuerpoPartes},
		{http.MethodGet, "/obras/obra-1/declaracion/historial", ""},
	}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolTitular}}
	h := Nueva(Casos{Auth: auth, Declaraciones: &declaracionesFalso{}}, Opciones{}).Router()

	for _, p := range peticiones {
		t.Run(p.metodo+" "+p.ruta, func(t *testing.T) {
			rec := pedir(t, h, p.metodo, p.ruta, p.cuerpo, "tok")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestDeclararObraGuardaYDevuelve201(t *testing.T) {
	falso := &declaracionesFalso{version: aplicacion.VersionDeclaracion{
		Version:      1,
		VigenteDesde: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Declaracion: repertorio.Declaracion{
			ObraID: "obra-1",
			Partes: []repertorio.Parte{
				{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
				{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(40)},
			},
		},
	}}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPost, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.obraIDRecibida != "obra-1" {
		t.Fatalf("obra recibida = %q", falso.obraIDRecibida)
	}
	if falso.actorRecibido != "usr-admin" {
		t.Fatalf("actor recibido = %q, se esperaba el de la sesion", falso.actorRecibido)
	}
	if len(falso.partesRecibidas) != 2 {
		t.Fatalf("partes recibidas = %+v", falso.partesRecibidas)
	}

	var cuerpo versionDeclaracionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("decodificar respuesta: %v", err)
	}
	if cuerpo.Version != 1 || cuerpo.Estado != "completa" {
		t.Fatalf("cuerpo = %+v", cuerpo)
	}
}

func TestEditarDeclaracionDevuelve200(t *testing.T) {
	falso := &declaracionesFalso{version: aplicacion.VersionDeclaracion{Version: 2}}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPut, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestGuardarDeclaracionInvalidaDevuelve400(t *testing.T) {
	falso := &declaracionesFalso{err: repertorio.ErrDeclaracionInvalida}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPost, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestGuardarDeclaracionDeObraInexistenteDevuelve404(t *testing.T) {
	falso := &declaracionesFalso{err: aplicacion.ErrNoEncontrado}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPost, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// 400 y no 404: el titular_id llega en el cuerpo, y el 404 de esta ruta esta
// reservado a la obra del path.
func TestGuardarDeclaracionConTitularInexistenteDevuelve400(t *testing.T) {
	falso := &declaracionesFalso{err: aplicacion.ErrTitularInexistente}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPost, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
	// Y con SU mensaje: el de R-01 es otro, y los dos son 400.
	if cuerpo := rec.Body.String(); !strings.Contains(cuerpo, "no existe") {
		t.Fatalf("el mensaje no dice que el titular no esta en el padron: %s", cuerpo)
	}
}

// El segundo 400 de esta ruta, y el que le faltaba al contrato: una parte cuyo
// titular SI esta en el padron y no es persona natural (`R-01`, `RD 4.5`). El
// mensaje tiene que nombrar la regla -es lo unico que explica el rechazo- y no
// puede confundirse con el del titular inexistente.
func TestGuardarDeclaracionConTitularQueNoEsPersonaNaturalDevuelve400(t *testing.T) {
	falso := &declaracionesFalso{err: aplicacion.ErrTitularNoEsPersonaNatural}
	h := servidorConDeclaraciones(t, falso)

	// PUT y no POST: el defecto que esto cierra se veia justo aqui, editando
	// una declaracion existente.
	rec := pedir(t, h, http.MethodPut, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, "R-01") {
		t.Fatalf("el mensaje no nombra la regla que rechaza: %s", cuerpo)
	}
	if strings.Contains(cuerpo, "no existe") {
		t.Fatalf("el rechazo de R-01 se anuncio como un titular inexistente: %s", cuerpo)
	}
}

// El tercer 400 de esta ruta, y el del 🔴: una parte declara un IPI que no es el
// que el padron tiene para ese titular. El mensaje es FIJO -no repite los dos
// numeros que el error del nucleo trae para el log- porque devolverlos
// convertiria este endpoint en un oraculo del padron para quien puede editar una
// declaracion.
func TestGuardarDeclaracionConIPIQueNoCuadraDevuelve400(t *testing.T) {
	falso := &declaracionesFalso{err: fmt.Errorf(
		"la parte del titular %q declara el IPI %q y el padron tiene %q: %w",
		"tit-ana", "IPI-00000002", "IPI-00000001", aplicacion.ErrIPIQueNoCuadra)}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPut, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, "padron") {
		t.Fatalf("el mensaje no dice contra que se compara el IPI: %s", cuerpo)
	}
	if strings.Contains(cuerpo, "IPI-00000002") || strings.Contains(cuerpo, "IPI-00000001") {
		t.Fatalf("la respuesta repite los IPI del padron: %s", cuerpo)
	}
	// Y no se confunde con los otros dos 400 de la ruta.
	if strings.Contains(cuerpo, "no existe") || strings.Contains(cuerpo, "persona natural") {
		t.Fatalf("el rechazo del IPI se anuncio como otro de los 400 de esta ruta: %s", cuerpo)
	}
}

// Un fallo del nucleo que no es ninguno de los rechazos de arriba es un 500, y
// ese es el punto de que R-01 tenga centinela propio: el dia que leer el padron
// falle, quien edita tiene que ver "no se pudo guardar" y no un 400 que le dice
// que sus datos estan mal.
func TestGuardarDeclaracionConFalloDelNucleoDevuelve500(t *testing.T) {
	falso := &declaracionesFalso{err: errors.New("comprobar quien puede recibir reparto: la base no responde")}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodPut, "/obras/obra-1/declaracion", cuerpoPartes, "tok")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("codigo = %d, se esperaba 500. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestGuardarDeclaracionCuerpoMalFormadoDevuelve400(t *testing.T) {
	h := servidorConDeclaraciones(t, &declaracionesFalso{})

	rec := pedir(t, h, http.MethodPost, "/obras/obra-1/declaracion", "no es json", "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestHistorialDevuelveListaVaciaYNoNull(t *testing.T) {
	h := servidorConDeclaraciones(t, &declaracionesFalso{})

	rec := pedir(t, h, http.MethodGet, "/obras/obra-1/declaracion/historial", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba un array vacio y no null", rec.Body.String())
	}
}

func TestHistorialDevuelveLasVersiones(t *testing.T) {
	falso := &declaracionesFalso{historial: []aplicacion.VersionDeclaracion{
		{Version: 1, Declaracion: repertorio.Declaracion{ObraID: "obra-1"}},
		{Version: 2, Declaracion: repertorio.Declaracion{ObraID: "obra-1"}},
	}}
	h := servidorConDeclaraciones(t, falso)

	rec := pedir(t, h, http.MethodGet, "/obras/obra-1/declaracion/historial", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}

	var cuerpo []versionDeclaracionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("decodificar respuesta: %v", err)
	}
	if len(cuerpo) != 2 || cuerpo[0].Version != 1 || cuerpo[1].Version != 2 {
		t.Fatalf("cuerpo = %+v", cuerpo)
	}
}

// ---------------------------------------------------------------------------
// Forma de red del porcentaje

// versionConPartes es la version que devuelven los dobles de estas pruebas:
// dos partes que suman 100, con los mismos porcentajes que manda
// [cuerpoPartes], para que la respuesta y la peticion se puedan comparar.
func versionConPartes() aplicacion.VersionDeclaracion {
	return aplicacion.VersionDeclaracion{
		Version:      1,
		VigenteDesde: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Declaracion: repertorio.Declaracion{
			ObraID: "obra-1",
			Partes: []repertorio.Parte{
				{TitularID: "t1", IPI: "IPI-1", Porcentaje: decimal.NewFromInt(60)},
				{TitularID: "t2", IPI: "IPI-2", Porcentaje: decimal.NewFromInt(40)},
			},
		},
	}
}

// El contrato declara `porcentaje` como `number` y la libreria de decimales
// serializa ENTRE COMILLAS por defecto, asi que hasta ahora `partes[].porcentaje`
// salia como "60" -y el editor de splits de la #30, que suma estos numeros,
// habria concatenado texto-. La prueba mira los BYTES y no el valor ya
// decodificado: "60" y 60 se decodifican al mismo numero, pero solo uno se
// puede sumar sin parsearlo antes.
//
// Las tres respuestas que llevan partes son el alta, la edicion y el
// historial. `GET /obras/{id}` tambien trae la suma de los porcentajes, y esa
// forma se comprueba en obras_test.go.
func TestElPorcentajeDeUnaParteViajaComoNumero(t *testing.T) {
	falso := &declaracionesFalso{
		version:   versionConPartes(),
		historial: []aplicacion.VersionDeclaracion{versionConPartes()},
	}
	h := servidorConDeclaraciones(t, falso)

	peticiones := []struct {
		nombre, metodo, ruta, cuerpo string
		quiero                       int
	}{
		{"el alta", http.MethodPost, "/obras/obra-1/declaracion", cuerpoPartes, http.StatusCreated},
		{"la edicion", http.MethodPut, "/obras/obra-1/declaracion", cuerpoPartes, http.StatusOK},
		{"el historial", http.MethodGet, "/obras/obra-1/declaracion/historial", "", http.StatusOK},
	}

	for _, p := range peticiones {
		t.Run(p.nombre, func(t *testing.T) {
			rec := pedir(t, h, p.metodo, p.ruta, p.cuerpo, "tok")
			if rec.Code != p.quiero {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, p.quiero, rec.Body)
			}

			cuerpo := rec.Body.String()
			for _, quiero := range []string{`"porcentaje":60`, `"porcentaje":40`} {
				if !strings.Contains(cuerpo, quiero) {
					t.Fatalf("la respuesta no trae %s: %s", quiero, cuerpo)
				}
			}

			// Y la forma vieja no puede quedar en ningun sitio: es justo la
			// que concatena en el cliente en vez de sumar.
			for _, prohibido := range []string{`"porcentaje":"60"`, `"porcentaje":"40"`} {
				if strings.Contains(cuerpo, prohibido) {
					t.Fatalf("la respuesta trae el porcentaje entre comillas (%s): %s", prohibido, cuerpo)
				}
			}
		})
	}
}

// El arreglo es de la forma que SALE: la que entra se sigue leyendo igual.
// El tipo se apoya en el UnmarshalJSON de la libreria, que acepta el numero
// del contrato y tambien la cadena entrecomillada que aceptaba
// `decimal.Decimal`; estrecharlo seria otro cambio, y rechazaria cuerpos que
// hoy se guardan.
func TestElPorcentajeSeSigueLeyendoIgualEnLaPeticion(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
		quiero string
	}{
		{"numero, que es lo que dice el contrato", `[{"titular_id": "t1", "ipi": "IPI-1", "porcentaje": 60}]`, "60"},
		{"cadena, tolerada como antes", `[{"titular_id": "t1", "ipi": "IPI-1", "porcentaje": "60"}]`, "60"},
		{"con decimales, sin redondear", `[{"titular_id": "t1", "ipi": "IPI-1", "porcentaje": 33.3333}]`, "33.3333"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			falso := &declaracionesFalso{version: aplicacion.VersionDeclaracion{Version: 1}}
			h := servidorConDeclaraciones(t, falso)

			rec := pedir(t, h, http.MethodPost, "/obras/obra-1/declaracion", c.cuerpo, "tok")
			if rec.Code != http.StatusCreated {
				t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
			}
			if len(falso.partesRecibidas) != 1 {
				t.Fatalf("partes recibidas = %+v", falso.partesRecibidas)
			}
			// Exacto y no aproximado: el porcentaje se guarda tal como llego.
			if !falso.partesRecibidas[0].Porcentaje.Equal(decimal.RequireFromString(c.quiero)) {
				t.Fatalf("porcentaje recibido = %s, se esperaba %s",
					falso.partesRecibidas[0].Porcentaje, c.quiero)
			}
		})
	}
}

// Y lo de arriba vale para el tipo suelto: un porcentaje con cuatro decimales
// -la precision que la columna admite- sale como numero, y ni entre comillas
// ni en notacion cientifica.
func TestElPorcentajeConDecimalesSerializaTalCual(t *testing.T) {
	bruto, err := json.Marshal(parteJSON{
		TitularID: "t1", IPI: "IPI-1",
		Porcentaje: decimalComoNumeroJSON(decimal.RequireFromString("33.3333")),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	quiero := `{"titular_id":"t1","ipi":"IPI-1","porcentaje":33.3333}`
	if string(bruto) != quiero {
		t.Fatalf("json = %s, se esperaba %s", bruto, quiero)
	}
}
