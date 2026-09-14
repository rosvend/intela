package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// recaudoFalso registra lo que le llega y devuelve lo que le pongan: estas
// pruebas comprueban el ADAPTADOR, no la base ni el caso de uso.
type recaudoFalso struct {
	bolsa    aplicacion.BolsaPersistida
	bolsas   []aplicacion.BolsaPersistida
	usuario  recaudo.Usuario
	usuarios []recaudo.Usuario
	err      error

	bolsaRecibida   aplicacion.BolsaPersistida
	usuarioRecibido recaudo.Usuario
	periodoRecibido string
	idRecibido      string
	actorRecibido   string
}

func (r *recaudoFalso) Registrar(_ context.Context, b aplicacion.BolsaPersistida, actorID string) (aplicacion.BolsaPersistida, error) {
	r.bolsaRecibida, r.actorRecibido = b, actorID
	return r.bolsa, r.err
}

func (r *recaudoFalso) Listar(_ context.Context, periodo string) ([]aplicacion.BolsaPersistida, error) {
	r.periodoRecibido = periodo
	return r.bolsas, r.err
}

func (r *recaudoFalso) PorID(_ context.Context, id string) (aplicacion.BolsaPersistida, error) {
	r.idRecibido = id
	return r.bolsa, r.err
}

func (r *recaudoFalso) RegistrarUsuario(_ context.Context, u recaudo.Usuario, actorID string) (recaudo.Usuario, error) {
	r.usuarioRecibido, r.actorRecibido = u, actorID
	return r.usuario, r.err
}

func (r *recaudoFalso) ListarUsuarios(context.Context) ([]recaudo.Usuario, error) {
	return r.usuarios, r.err
}

// servidorConRecaudo monta el router con una sesion de contabilidad ya
// resuelta: es quien factura, y por tanto quien puede registrar lo cobrado.
func servidorConRecaudo(t *testing.T, rec Recaudo) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-conta", Rol: aplicacion.RolContabilidad},
	}
	return Nueva(nil, Casos{Auth: auth, Recaudo: rec}, Opciones{}).Router()
}

const cuerpoRecaudo = `{
  "id": "bolsa-caracol-2025-01-nacional",
  "usuario_id": "caracol",
  "periodo": "2025-01",
  "circuito": "nacional",
  "bruto": 1000000.00,
  "convenio": "conv-caracol-2025",
  "factura": "FE-1024"
}`

const cuerpoUsuarioRecaudo = `{
  "id": "caracol",
  "nombre": "Caracol Television S.A.",
  "nit": "860025674-2",
  "categoria": "tv_abierta"
}`

func bolsaDeEjemplo() aplicacion.BolsaPersistida {
	return aplicacion.BolsaPersistida{
		ID:        "bolsa-caracol-2025-01-nacional",
		UsuarioID: "caracol",
		Periodo:   "2025-01",
		Circuito:  recaudo.Nacional,
		Bruto:     decimal.RequireFromString("1000000.00"),
		Convenio:  "conv-caracol-2025",
		Factura:   "FE-1024",
	}
}

func TestRecaudoExigeRolQuePuedaTocarDinero(t *testing.T) {
	// `titular` es el rol mas acotado: solo ve las obras donde participa
	// (OE-6). Que pueda leer bolsas o registrar recaudo seria ver -y mover-
	// el ingreso de toda la sociedad.
	peticiones := []struct{ metodo, ruta, cuerpo string }{
		{http.MethodPost, "/recaudo", cuerpoRecaudo},
		{http.MethodPost, "/recaudo/usuarios", cuerpoUsuarioRecaudo},
		{http.MethodGet, "/recaudo/usuarios", ""},
		{http.MethodGet, "/bolsas", ""},
		{http.MethodGet, "/bolsas/bolsa-1", ""},
	}
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolTitular}}
	h := Nueva(nil, Casos{Auth: auth, Recaudo: &recaudoFalso{}}, Opciones{}).Router()

	for _, p := range peticiones {
		t.Run(p.metodo+" "+p.ruta, func(t *testing.T) {
			rec := pedir(t, h, p.metodo, p.ruta, p.cuerpo, "tok")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestDistribucionLeeBolsasPeroNoRegistraRecaudo(t *testing.T) {
	// `distribucion` necesita la bolsa para correr el reparto, y no es quien
	// factura. Las dos mitades de la separacion de funciones del RD 13.5.
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-dist", Rol: aplicacion.RolDistribucion}}
	h := Nueva(nil, Casos{Auth: auth, Recaudo: &recaudoFalso{bolsa: bolsaDeEjemplo()}}, Opciones{}).Router()

	if rec := pedir(t, h, http.MethodGet, "/bolsas", "", "tok"); rec.Code != http.StatusOK {
		t.Fatalf("GET /bolsas = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if rec := pedir(t, h, http.MethodPost, "/recaudo", cuerpoRecaudo, "tok"); rec.Code != http.StatusForbidden {
		t.Fatalf("POST /recaudo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestRegistrarRecaudoDevuelve201YLocation(t *testing.T) {
	falso := &recaudoFalso{bolsa: bolsaDeEjemplo()}
	h := servidorConRecaudo(t, falso)

	rec := pedir(t, h, http.MethodPost, "/recaudo", cuerpoRecaudo, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc != "/bolsas/bolsa-caracol-2025-01-nacional" {
		t.Fatalf("Location = %q", loc)
	}
	if falso.actorRecibido != "usr-conta" {
		t.Fatalf("actor recibido = %q, se esperaba el de la sesion", falso.actorRecibido)
	}
	if falso.bolsaRecibida.UsuarioID != "caracol" || falso.bolsaRecibida.Circuito != recaudo.Nacional {
		t.Fatalf("bolsa recibida = %+v", falso.bolsaRecibida)
	}
	// El bruto tiene que llegar al nucleo como decimal exacto. Si el DTO lo
	// leyera como float64, 1000000.00 sobreviviria y otras cifras no: es el
	// unico sitio del sistema donde el ADR 0005 exige aritmetica decimal.
	if !falso.bolsaRecibida.Bruto.Equal(decimal.RequireFromString("1000000.00")) {
		t.Fatalf("bruto recibido = %s", falso.bolsaRecibida.Bruto)
	}

	var cuerpo bolsaJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es la bolsa: %v", err)
	}
	if cuerpo.ID != "bolsa-caracol-2025-01-nacional" || cuerpo.Convenio != "conv-caracol-2025" {
		t.Fatalf("cuerpo = %+v", cuerpo)
	}
}

func TestRegistrarRecaudoNoPierdeCentavos(t *testing.T) {
	// Un bruto con centavos que en binario no es exacto. Con float64 por medio
	// llega 1000000.09999999 y el dominio lo rechaza por precision -o peor, lo
	// acepta y la cifra deja de cuadrar con la factura.
	falso := &recaudoFalso{bolsa: bolsaDeEjemplo()}
	h := servidorConRecaudo(t, falso)

	cuerpo := `{"id":"b1","usuario_id":"caracol","periodo":"2025-01",
	            "circuito":"nacional","bruto":1000000.10}`
	if rec := pedir(t, h, http.MethodPost, "/recaudo", cuerpo, "tok"); rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if !falso.bolsaRecibida.Bruto.Equal(decimal.RequireFromString("1000000.10")) {
		t.Fatalf("bruto recibido = %s, se esperaba 1000000.10 exacto", falso.bolsaRecibida.Bruto)
	}
}

func TestRegistrarRecaudoTraduceLosErroresDelNucleo(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		codigo int
	}{
		{"bolsa mal formada", recaudo.ErrBolsaInvalida, http.StatusBadRequest},
		// 409 y no 500: el alta estaba bien formada y esa bolsa ya existe.
		{"bolsa duplicada", aplicacion.ErrBolsaDuplicada, http.StatusConflict},
		// 400 y no 404: lo que no existe es un dato DENTRO del cuerpo, no el
		// recurso de la URL.
		{"usuario inexistente", aplicacion.ErrUsuarioRecaudoInexistente, http.StatusBadRequest},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConRecaudo(t, &recaudoFalso{err: c.err})
			rec := pedir(t, h, http.MethodPost, "/recaudo", cuerpoRecaudo, "tok")
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.codigo, rec.Body)
			}
		})
	}
}

func TestRegistrarRecaudoConCuerpoQueNoEsJSON(t *testing.T) {
	h := servidorConRecaudo(t, &recaudoFalso{})
	rec := pedir(t, h, http.MethodPost, "/recaudo", "no soy json", "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestListarBolsasPasaElPeriodo(t *testing.T) {
	falso := &recaudoFalso{bolsas: []aplicacion.BolsaPersistida{bolsaDeEjemplo()}}
	h := servidorConRecaudo(t, falso)

	rec := pedir(t, h, http.MethodGet, "/bolsas?periodo=2025-01", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.periodoRecibido != "2025-01" {
		t.Fatalf("periodo recibido = %q", falso.periodoRecibido)
	}
}

func TestListarBolsasConPeriodoMalFormadoDevuelve400(t *testing.T) {
	h := servidorConRecaudo(t, &recaudoFalso{err: recaudo.ErrBolsaInvalida})
	rec := pedir(t, h, http.MethodGet, "/bolsas?periodo=enero", "", "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestListarBolsasVaciasDevuelveListaYNoNull(t *testing.T) {
	h := servidorConRecaudo(t, &recaudoFalso{})
	rec := pedir(t, h, http.MethodGet, "/bolsas", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba []", rec.Body.String())
	}
}

func TestBolsaPorIDInexistenteDevuelve404(t *testing.T) {
	h := servidorConRecaudo(t, &recaudoFalso{err: aplicacion.ErrNoEncontrado})
	rec := pedir(t, h, http.MethodGet, "/bolsas/no-esta", "", "tok")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestRegistrarUsuarioDeRecaudoDevuelve201(t *testing.T) {
	u, err := recaudo.NuevoUsuario("caracol", recaudo.Datos{
		Nombre:    "Caracol Television S.A.",
		NIT:       "860025674-2",
		Categoria: recaudo.TVAbierta,
	})
	if err != nil {
		t.Fatalf("NuevoUsuario: %v", err)
	}
	falso := &recaudoFalso{usuario: u}
	h := servidorConRecaudo(t, falso)

	rec := pedir(t, h, http.MethodPost, "/recaudo/usuarios", cuerpoUsuarioRecaudo, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d, se esperaba 201. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.usuarioRecibido.ID() != "caracol" {
		t.Fatalf("usuario recibido = %q", falso.usuarioRecibido.ID())
	}
	if falso.usuarioRecibido.Datos().Categoria != recaudo.TVAbierta {
		t.Fatalf("categoria recibida = %q", falso.usuarioRecibido.Datos().Categoria)
	}
	// Sin Location, a diferencia del alta de una bolsa: no hay
	// GET /recaudo/usuarios/{id} que apuntar, y una cabecera hacia una ruta
	// inexistente manda al cliente a un 404 sin forma de saber si el alta
	// funciono.
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("Location = %q, y la ruta que apunta no esta registrada", loc)
	}
}

func TestRegistrarUsuarioDeRecaudoTraduceLosErrores(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		codigo int
	}{
		{"categoria invalida", recaudo.ErrUsuarioInvalido, http.StatusBadRequest},
		{"ya existe", aplicacion.ErrUsuarioDeRecaudoDuplicado, http.StatusConflict},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConRecaudo(t, &recaudoFalso{err: c.err})
			rec := pedir(t, h, http.MethodPost, "/recaudo/usuarios", cuerpoUsuarioRecaudo, "tok")
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.codigo, rec.Body)
			}
		})
	}
}

func TestListarUsuariosDeRecaudoVaciosDevuelveLista(t *testing.T) {
	h := servidorConRecaudo(t, &recaudoFalso{})
	rec := pedir(t, h, http.MethodGet, "/recaudo/usuarios", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba []", rec.Body.String())
	}
}
