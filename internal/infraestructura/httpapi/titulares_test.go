package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// padronFalso registra lo que le llega y devuelve lo que le pongan: estas
// pruebas comprueban el ADAPTADOR -codigos, filtros y forma del JSON- y no la
// base.
type padronFalso struct {
	titulares []afiliacion.Titular
	err       error

	filtro aplicacion.FiltroTitulares
}

func (p *padronFalso) BuscarTitulares(_ context.Context, f aplicacion.FiltroTitulares) ([]afiliacion.Titular, error) {
	p.filtro = f
	return p.titulares, p.err
}

func titularDePrueba(t *testing.T) afiliacion.Titular {
	t.Helper()

	tit, err := afiliacion.NuevoTitular(
		"tit-ana", "Ana Escritora", "IPI-00000001", true, afiliacion.ClaseSocio)
	if err != nil {
		t.Fatalf("construir el titular de prueba: %v", err)
	}
	return tit
}

// servidorConPadron monta el router con una sesion de administrador ya
// resuelta, que es lo que exige la ruta.
func servidorConPadron(t *testing.T, pad Padron) http.Handler {
	t.Helper()

	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador},
	}
	return Nueva(Casos{Auth: auth, Padron: pad}, Opciones{}).Router()
}

// ---------------------------------------------------------------------------
// Autorizacion

// La misma que el resto del catalogo: quien edita una declaracion ve el
// repertorio entero y el padron es la otra mitad de esa superficie.
func TestElPadronExigeElRolAdministrador(t *testing.T) {
	roles := []aplicacion.Rol{
		aplicacion.RolDistribucion,
		aplicacion.RolContabilidad,
		aplicacion.RolAuditor,
		aplicacion.RolTitular,
	}

	for _, rol := range roles {
		auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol}}
		h := Nueva(Casos{Auth: auth, Padron: &padronFalso{}}, Opciones{}).Router()
		t.Run(string(rol), func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, "/titulares", "", "tok")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

// Sin sesion el corte es conSesion (401), no requiereRol (403).
func TestElPadronSinSesionEs401(t *testing.T) {
	h := servidorConPadron(t, &padronFalso{})

	rec := pedir(t, h, http.MethodGet, "/titulares", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Filtros

func TestBuscarTitularesPasaNombreEIPI(t *testing.T) {
	pad := &padronFalso{}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?nombre=escritora&ipi=IPI-00000001", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	quiero := aplicacion.FiltroTitulares{
		Nombre: "escritora",
		IPI:    "IPI-00000001",
		Paginacion: aplicacion.Paginacion{
			Limite: aplicacion.LimiteObrasPorDefecto,
		},
	}
	if pad.filtro.Nombre != quiero.Nombre || pad.filtro.IPI != quiero.IPI {
		t.Fatalf("filtro = %+v, se esperaba %+v", pad.filtro, quiero)
	}
	// Sin `persona_natural` el filtro NO se aplica: convertirlo en `false`
	// devolveria solo personas juridicas justo cuando nadie lo pidio.
	if pad.filtro.PersonaNatural != nil {
		t.Fatalf("PersonaNatural = %v, se esperaba nil", *pad.filtro.PersonaNatural)
	}
	if pad.filtro.Paginacion != quiero.Paginacion {
		t.Fatalf("paginacion = %+v, se esperaba %+v", pad.filtro.Paginacion, quiero.Paginacion)
	}
}

// `false` es una pregunta legitima -quien esta en el padron y NO puede recibir
// reparto (R-01)-, asi que tiene que llegar al puerto como un false explicito
// y no confundirse con "sin filtro".
func TestBuscarTitularesConPersonaNaturalFalse(t *testing.T) {
	pad := &padronFalso{}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?persona_natural=false", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if pad.filtro.PersonaNatural == nil {
		t.Fatal("el filtro llego como nil: `false` y `sin filtro` no son lo mismo")
	}
	if *pad.filtro.PersonaNatural {
		t.Fatal("?persona_natural=false llego al puerto como true")
	}
}

func TestBuscarTitularesConPersonaNaturalTrue(t *testing.T) {
	pad := &padronFalso{}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?persona_natural=true", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if pad.filtro.PersonaNatural == nil || !*pad.filtro.PersonaNatural {
		t.Fatalf("PersonaNatural = %v, se esperaba true", pad.filtro.PersonaNatural)
	}
}

// Un valor que no es booleano se rechaza en vez de ignorarse: ignorado, quien
// pregunta por las personas juridicas recibiria el padron entero y leeria a
// las personas naturales como si fueran lo que pidio.
func TestBuscarTitularesRechazaUnaPersonaNaturalQueNoEsBooleana(t *testing.T) {
	h := servidorConPadron(t, &padronFalso{})

	for _, bruto := range []string{"si", "no", "1", "0", "t", "TRUE", "True", "False", "ninguna"} {
		t.Run(bruto, func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, "/titulares?persona_natural="+bruto, "", "tok")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// El filtro por identificadores (item 9b)

// La consulta ACOTADA: unos pocos ids y nada mas. El puerto y el store ya la
// soportaban -`FiltroTitulares.IDs`, con `TestBuscarTitularesPorIDsDevuelveSoloEsos`
// probando el SQL en `postgres`-, y lo que faltaba era que el handler leyera el
// parametro.
//
// LA afirmacion que importa es la del FILTRO, no la de la respuesta: el doble
// devuelve lo que le pongan, asi que "el cuerpo trae a tit-ana" pasaria tambien
// si el handler ignorase `ids` y filtrase despues en memoria, o si devolviese el
// padron entero. Que al padron le llegue la lista pedida es lo unico que dice
// que quien filtra es la consulta.
func TestBuscarTitularesConIDsPideEsosYNoElPadron(t *testing.T) {
	pad := &padronFalso{titulares: []afiliacion.Titular{titularDePrueba(t)}}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?ids=tit-ana", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if quiero := []string{"tit-ana"}; !reflect.DeepEqual(pad.filtro.IDs, quiero) {
		t.Fatalf("IDs = %#v, se esperaba %#v", pad.filtro.IDs, quiero)
	}

	var cuerpo []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("el cuerpo no es una lista JSON: %v (%q)", err, rec.Body)
	}
	// Y la fila que se pidio viaja con la forma del contrato.
	if len(cuerpo) != 1 || cuerpo[0]["id"] != "tit-ana" {
		t.Fatalf("cuerpo = %s", rec.Body)
	}
}

// Las dos formas del parametro son la MISMA pregunta, y ninguna es "la rara":
// repetir la clave es lo que produce un cliente generado a partir del contrato
// -`style: form` con `explode` en openapi.yaml- y la coma es lo que escribe una
// mano. Si una de las dos no llegara, el filtro se perderia en silencio y la
// pantalla pintaria un guion donde hay nombre.
func TestBuscarTitularesAceptaIDsRepetidosYSeparadosPorComas(t *testing.T) {
	casos := []struct {
		nombre string
		query  string
		quiero []string
	}{
		{
			"repetido, que es lo que manda el cliente del contrato",
			"ids=tit-ana&ids=tit-luis",
			[]string{"tit-ana", "tit-luis"},
		},
		{
			"separado por comas, que es lo que escribe una mano",
			"ids=tit-ana,tit-luis",
			[]string{"tit-ana", "tit-luis"},
		},
		{
			"las dos formas a la vez",
			"ids=tit-ana&ids=tit-luis,tit-carla",
			[]string{"tit-ana", "tit-luis", "tit-carla"},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			pad := &padronFalso{}
			h := servidorConPadron(t, pad)

			rec := pedir(t, h, http.MethodGet, "/titulares?"+c.query, "", "tok")
			if rec.Code != http.StatusOK {
				t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
			}
			if !reflect.DeepEqual(pad.filtro.IDs, c.quiero) {
				t.Fatalf("IDs = %#v, se esperaba %#v", pad.filtro.IDs, c.quiero)
			}
		})
	}
}

// Una coma de mas deja un elemento vacio, y un elemento vacio NO es un
// identificador que se le pida al padron: `id = ANY('{""}')` no encuentra nada,
// asi que pasarlo convierte una lista con dos ids utiles en una que no devuelve
// nada.
func TestBuscarTitularesIgnoraLosIDsVaciosQueDejaUnaComaDeMas(t *testing.T) {
	pad := &padronFalso{}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?ids=,tit-ana,,tit-luis,", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if quiero := []string{"tit-ana", "tit-luis"}; !reflect.DeepEqual(pad.filtro.IDs, quiero) {
		t.Fatalf("IDs = %#v, se esperaba %#v", pad.filtro.IDs, quiero)
	}
}

// Un identificador que no esta en el padron NO es un error: `ids` es un filtro,
// no una promesa de existencia. Quien pinta decide que hacer con el hueco, y por
// eso lo que se pide es la lista VACIA, con su 200.
func TestBuscarTitularesConIDInexistenteDevuelveListaVacia(t *testing.T) {
	pad := &padronFalso{}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?ids=NO-EXISTE-999", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if cuerpo := rec.Body.String(); cuerpo != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba \"[]\"", cuerpo)
	}
	// El id llego al padron -lo que falta es la fila, no la pregunta-.
	if quiero := []string{"NO-EXISTE-999"}; !reflect.DeepEqual(pad.filtro.IDs, quiero) {
		t.Fatalf("IDs = %#v, se esperaba %#v", pad.filtro.IDs, quiero)
	}

	// CONTROL: el mismo endpoint SIN `ids` sigue devolviendo el padron, asi que
	// el 200 con lista vacia de arriba es del filtro y no del handler.
	pad = &padronFalso{titulares: []afiliacion.Titular{titularDePrueba(t)}}
	h = servidorConPadron(t, pad)
	rec = pedir(t, h, http.MethodGet, "/titulares", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if cuerpo := rec.Body.String(); !strings.Contains(cuerpo, `"tit-ana"`) {
		t.Fatalf("sin `ids` el padron tiene que seguir saliendo: %q", cuerpo)
	}
}

// El CONTROL NEGATIVO, y es el que impide un arreglo que rechace todo: un `ids`
// vacio -`?ids=`, solo comas, solo espacios- es la misma pregunta que no mandar
// el parametro, o sea EL PADRON.
//
// No es una decision de este handler: el puerto lo documenta ("un IDs vacio y
// uno nil son la misma pregunta: sin filtro") y el SQL lo implementa literal
// (`cardinality($5::text[]) = 0 OR id = ANY($5)`, `postgres/titulares.go`). Un
// arreglo que tradujera "vacio" a "lista vacia de resultados" rompe ese contrato
// y deja al cliente que manda el parametro sin valor -un formulario que se envia
// sin rellenarlo- viendo un padron vacio que no pidio.
func TestBuscarTitularesConIDsVacioSeComportaComoSinFiltro(t *testing.T) {
	// El filtro que produce NO mandar el parametro: es el patron contra el que se
	// comparan los tres casos de abajo.
	sinParametro := &padronFalso{titulares: []afiliacion.Titular{titularDePrueba(t)}}
	rec := pedir(t, servidorConPadron(t, sinParametro), http.MethodGet, "/titulares", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	padronEntonces := rec.Body.String()

	for _, query := range []string{"ids=", "ids=,", "ids=,,", "ids=%20", "ids=,+"} {
		t.Run(query, func(t *testing.T) {
			pad := &padronFalso{titulares: []afiliacion.Titular{titularDePrueba(t)}}
			h := servidorConPadron(t, pad)

			rec := pedir(t, h, http.MethodGet, "/titulares?"+query, "", "tok")
			if rec.Code != http.StatusOK {
				t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
			}
			// La misma respuesta que sin el parametro...
			if cuerpo := rec.Body.String(); cuerpo != padronEntonces {
				t.Fatalf("cuerpo = %q, se esperaba el padron entero %q", cuerpo, padronEntonces)
			}
			// ...y el mismo filtro, dato a dato: es "sin filtro", no "filtro por
			// una lista de cero ids".
			if !reflect.DeepEqual(pad.filtro, sinParametro.filtro) {
				t.Fatalf("filtro = %#v, se esperaba %#v", pad.filtro, sinParametro.filtro)
			}
		})
	}
}

func TestBuscarTitularesPasaLimiteYDesplazamiento(t *testing.T) {
	pad := &padronFalso{}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares?limite=10&desplazamiento=20", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if pad.filtro.Limite != 10 || pad.filtro.Desplazamiento != 20 {
		t.Fatalf("paginacion = {%d,%d}, se esperaba {10,20}", pad.filtro.Limite, pad.filtro.Desplazamiento)
	}
}

func TestBuscarTitularesRechazaPaginacionInvalida(t *testing.T) {
	h := servidorConPadron(t, &padronFalso{})

	casos := []string{
		"limite=0",
		"limite=-1",
		"limite=abc",
		"limite=501",
		"desplazamiento=-1",
		"desplazamiento=x",
	}
	for _, c := range casos {
		t.Run(c, func(t *testing.T) {
			rec := pedir(t, h, http.MethodGet, "/titulares?"+c, "", "tok")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Forma del contrato

func TestBuscarTitularesDevuelveLaFormaDelContrato(t *testing.T) {
	pad := &padronFalso{titulares: []afiliacion.Titular{titularDePrueba(t)}}
	h := servidorConPadron(t, pad)

	rec := pedir(t, h, http.MethodGet, "/titulares", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}

	var cuerpo []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("el cuerpo no es una lista JSON: %v (%q)", err, rec.Body)
	}
	if len(cuerpo) != 1 {
		t.Fatalf("cuerpo = %s", rec.Body)
	}
	// Los nombres de los campos SON el contrato: cuadran con el schema Titular
	// de api/openapi.yaml.
	for _, campo := range []string{"id", "nombre", "ipi", "persona_natural", "clase"} {
		if _, hay := cuerpo[0][campo]; !hay {
			t.Fatalf("falta el campo %q en la respuesta: %s", campo, rec.Body)
		}
	}
	if cuerpo[0]["id"] != "tit-ana" || cuerpo[0]["nombre"] != "Ana Escritora" ||
		cuerpo[0]["clase"] != "socio" {
		t.Fatalf("cuerpo = %v", cuerpo[0])
	}
	if cuerpo[0]["persona_natural"] != true {
		t.Fatalf("persona_natural = %v, se esperaba true", cuerpo[0]["persona_natural"])
	}
	// El padron se sirve para armar un reparto: la direccion de contacto de
	// cada titular no sale de la base por aqui.
	if _, hay := cuerpo[0]["email"]; hay {
		t.Fatalf("el padron no puede devolver el email: %s", rec.Body)
	}
}

// Sin coincidencias tiene que salir [] y no null, o cualquier cliente que
// itere la respuesta revienta.
func TestBuscarTitularesSinCoincidenciasDevuelveListaVacia(t *testing.T) {
	h := servidorConPadron(t, &padronFalso{})

	rec := pedir(t, h, http.MethodGet, "/titulares", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200", rec.Code)
	}
	if cuerpo := rec.Body.String(); cuerpo != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba \"[]\"", cuerpo)
	}
}

// El struct de red es el que tiene que cuadrar con el schema, campo a campo.
func TestElTitularSerializaLaFormaDelContrato(t *testing.T) {
	bruto, err := json.Marshal(aTitularJSON(titularDePrueba(t)))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(bruto, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	quiero := []string{"clase", "id", "ipi", "nombre", "persona_natural"}
	if len(m) != len(quiero) {
		t.Fatalf("el JSON tiene %d campos, se esperaban %d: %s", len(m), len(quiero), bruto)
	}
	for _, campo := range quiero {
		if _, hay := m[campo]; !hay {
			t.Fatalf("falta el campo %q: %s", campo, bruto)
		}
	}
}
