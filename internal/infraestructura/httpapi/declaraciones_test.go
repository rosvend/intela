package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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
