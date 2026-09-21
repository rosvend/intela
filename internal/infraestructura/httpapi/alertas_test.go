package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/recaudo"
)

// anomaliasFalsas devuelve lo que le pongan y apunta lo que recibe: estas
// pruebas comprueban el ADAPTADOR -- rutas, roles, codigos y forma de red --,
// no la deteccion ni la base.
type anomaliasFalsas struct {
	alertas []aplicacion.Alerta
	alerta  aplicacion.Alerta
	resumen aplicacion.ResumenEvaluacion

	errListar   error
	errEvaluar  error
	errResolver error

	filtroRecibido  aplicacion.FiltroAlertas
	periodoRecibido string
	idRecibido      string
	actorRecibido   string
	notaRecibida    string
}

func (a *anomaliasFalsas) Evaluar(
	_ context.Context, periodo, actorID string,
) (aplicacion.ResumenEvaluacion, error) {
	a.periodoRecibido, a.actorRecibido = periodo, actorID
	return a.resumen, a.errEvaluar
}

func (a *anomaliasFalsas) Listar(
	_ context.Context, f aplicacion.FiltroAlertas,
) ([]aplicacion.Alerta, error) {
	a.filtroRecibido = f
	return a.alertas, a.errListar
}

func (a *anomaliasFalsas) Resolver(
	_ context.Context, id, actorID, nota string,
) (aplicacion.Alerta, error) {
	a.idRecibido, a.actorRecibido, a.notaRecibida = id, actorID, nota
	return a.alerta, a.errResolver
}

var instanteAlertaHTTP = time.Date(2026, 5, 2, 8, 30, 0, 0, time.UTC)

func alertaDeEjemplo() aplicacion.Alerta {
	return aplicacion.Alerta{
		ID:         "3f1d0a4e-0000-4000-8000-000000000001",
		Periodo:    "2025-01",
		Tipo:       anomalias.TipoTitularSinPorcentaje,
		RefTipo:    anomalias.RefObra,
		RefID:      "obra-1",
		RefTitular: "IPI-00000002",
		Detalle:    "el coautor con IPI IPI-00000002 no tiene parte en la declaracion vigente",
		Critica:    false,
		Detectada:  instanteAlertaHTTP,
	}
}

func servidorConAnomalias(t *testing.T, rol aplicacion.Rol, svc Anomalias) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol}}
	return Nueva(Casos{Auth: auth, Anomalias: svc}, Opciones{}).Router()
}

// ---------------------------------------------------------------------------
// Roles

// La matriz que fija docs/architecture/roles.md: `auditor` lee TODO y no opera
// el pipeline ni firma, asi que mira la bandeja y no puede cerrar una alerta.
// `distribucion` si, porque es quien persigue estas alertas con los autores.
// Que ver y resolver pidan cosas distintas es lo que obliga a DOS sub-grupos
// de chi en vez de un chequeo a mano dentro del handler.
func TestAlertasAplicaLosRolesPorRuta(t *testing.T) {
	casos := []struct {
		rol     aplicacion.Rol
		listar  int
		escribe int
	}{
		{aplicacion.RolAdministrador, http.StatusOK, http.StatusOK},
		{aplicacion.RolDistribucion, http.StatusOK, http.StatusOK},
		// Lee y no cierra: cerrar una alerta es una decision sobre a quien se
		// le paga, y el Revisor Fiscal no opera el pipeline.
		{aplicacion.RolAuditor, http.StatusOK, http.StatusForbidden},
		// Contabilidad LEE y no escribe, igual que en `/bolsas`. Es la
		// segunda firma de la compuerta (`docs/architecture/roles.md`: "La
		// otra firma de las mismas compuertas"; ADR 0008 sobre `RD 13.8.6`),
		// y lo que firma depende de cuantas criticas siguen abiertas: sin
		// lectura firmaria a ciegas. No escribe porque perseguir la anomalia
		// con los autores es trabajo de `distribucion`.
		//
		// El 403 de antes ademas era visible: `web/src/navegacion.ts` de #104
		// ya declara `/anomalias` para contabilidad, asi que veia la entrada
		// de menu y comia 403 al entrar.
		{aplicacion.RolContabilidad, http.StatusOK, http.StatusForbidden},
		// El titular solo ve las obras donde participa (OE-6).
		{aplicacion.RolTitular, http.StatusForbidden, http.StatusForbidden},
	}

	for _, c := range casos {
		t.Run(string(c.rol), func(t *testing.T) {
			falso := &anomaliasFalsas{alerta: alertaDeEjemplo()}
			h := servidorConAnomalias(t, c.rol, falso)

			if rec := pedir(t, h, http.MethodGet, "/alertas", "", "tok"); rec.Code != c.listar {
				t.Fatalf("GET /alertas = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.listar, rec.Body)
			}
			rec := pedir(t, h, http.MethodPost, "/alertas/evaluacion", `{"periodo":"2025-01"}`, "tok")
			if rec.Code != c.escribe {
				t.Fatalf("POST /alertas/evaluacion = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.escribe, rec.Body)
			}
			rec = pedir(t, h, http.MethodPost, "/alertas/al-1/resolver", `{"nota":"x"}`, "tok")
			if rec.Code != c.escribe {
				t.Fatalf("POST /alertas/{id}/resolver = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.escribe, rec.Body)
			}
		})
	}
}

// Sin sesion es 401 y no 403: un 403 a quien no se identifico le diria que la
// ruta existe y que el problema es el rol.
func TestAlertasSinSesionEs401(t *testing.T) {
	h := Nueva(Casos{Auth: &autenticacionFalsa{errResolver: aplicacion.ErrNoEncontrado},
		Anomalias: &anomaliasFalsas{}}, Opciones{}).Router()

	for _, ruta := range []string{"/alertas"} {
		if rec := pedir(t, h, http.MethodGet, ruta, "", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s sin token = %d, se esperaba 401", ruta, rec.Code)
		}
	}
}

// Un binario que no cablee el caso de uso responde 503 y no un 500 sin cuerpo:
// la ruta existe, lo que falta es la pieza en ESA instalacion.
func TestAlertasSinCasoDeUsoEs503(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	h := Nueva(Casos{Auth: auth}, Opciones{}).Router()

	if rec := pedir(t, h, http.MethodGet, "/alertas", "", "tok"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, se esperaba 503. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// ---------------------------------------------------------------------------
// GET /alertas

func TestListarAlertasSirveLaBandeja(t *testing.T) {
	falso := &anomaliasFalsas{alertas: []aplicacion.Alerta{alertaDeEjemplo()}}
	h := servidorConAnomalias(t, aplicacion.RolDistribucion, falso)

	rec := pedir(t, h, http.MethodGet, "/alertas?periodo=2025-01&tipo=titular_sin_porcentaje&resueltas=false", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.filtroRecibido.Periodo != "2025-01" || falso.filtroRecibido.Tipo != "titular_sin_porcentaje" {
		t.Fatalf("filtro recibido = %+v", falso.filtroRecibido)
	}
	if falso.filtroRecibido.Resueltas == nil || *falso.filtroRecibido.Resueltas {
		t.Fatalf("resueltas recibido = %v", falso.filtroRecibido.Resueltas)
	}

	var cuerpo []alertaJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if len(cuerpo) != 1 {
		t.Fatalf("llegaron %d alertas", len(cuerpo))
	}
	// La referencia compuesta es lo que el tablero de #104 pinta en la celda
	// "Referencia": un ref_id suelto no dice de que es.
	if cuerpo[0].Referencia != "obra:obra-1#IPI-00000002" {
		t.Fatalf("referencia = %q", cuerpo[0].Referencia)
	}
	// Y el par legible por maquina viaja aparte, para que #39 no tenga que
	// partir la cadena por el separador.
	if cuerpo[0].RefTipo != "obra" || cuerpo[0].RefID != "obra-1" {
		t.Fatalf("ref_tipo/ref_id = %q/%q", cuerpo[0].RefTipo, cuerpo[0].RefID)
	}
}

// Sin coincidencias sale [] y no null, o el tablero que itera la respuesta
// revienta.
func TestListarAlertasVaciaDevuelveArrayYNoNull(t *testing.T) {
	h := servidorConAnomalias(t, aplicacion.RolAuditor, &anomaliasFalsas{})

	rec := pedir(t, h, http.MethodGet, "/alertas", "", "tok")
	if cuerpo := rec.Body.String(); cuerpo != "[]\n" {
		t.Fatalf("cuerpo = %q, se esperaba []", cuerpo)
	}
}

func TestListarAlertasRechazaFiltrosInvalidos(t *testing.T) {
	casos := []struct {
		nombre, consulta string
		falso            *anomaliasFalsas
	}{
		{
			// El nucleo lo rechaza con el validador del dominio.
			nombre: "periodo imposible", consulta: "?periodo=2025-13",
			falso: &anomaliasFalsas{errListar: recaudo.ErrBolsaInvalida},
		},
		{
			nombre: "tipo inventado", consulta: "?tipo=onni",
			falso: &anomaliasFalsas{errListar: aplicacion.ErrFiltroInvalido},
		},
		{
			// Este lo rechaza el adaptador: es un problema de FORMA del
			// parametro, no de vocabulario del dominio.
			nombre: "resueltas que no es booleano", consulta: "?resueltas=quiza",
			falso: &anomaliasFalsas{},
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConAnomalias(t, aplicacion.RolAdministrador, c.falso)
			rec := pedir(t, h, http.MethodGet, "/alertas"+c.consulta, "", "tok")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// POST /alertas/evaluacion

func TestEvaluarDevuelveElResumen(t *testing.T) {
	falso := &anomaliasFalsas{resumen: aplicacion.ResumenEvaluacion{
		Periodo: "2025-01", Detectadas: 6, Nuevas: 6,
		PorTipo:          map[string]int{anomalias.TipoONI: 1},
		CriticasAbiertas: 3,
	}}
	h := servidorConAnomalias(t, aplicacion.RolDistribucion, falso)

	rec := pedir(t, h, http.MethodPost, "/alertas/evaluacion", `{"periodo":"2025-01"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.periodoRecibido != "2025-01" {
		t.Fatalf("periodo recibido = %q", falso.periodoRecibido)
	}
	// El actor sale de la SESION: dejarlo llegar por el cuerpo permitiria
	// firmar la pasada a nombre de otro.
	if falso.actorRecibido != "usr-1" {
		t.Fatalf("actor recibido = %q", falso.actorRecibido)
	}

	var cuerpo resumenEvaluacionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if cuerpo.Detectadas != 6 || cuerpo.Nuevas != 6 || cuerpo.CriticasAbiertas != 3 {
		t.Fatalf("resumen = %+v", cuerpo)
	}
}

func TestEvaluarRechazaUnPeriodoMalFormado(t *testing.T) {
	falso := &anomaliasFalsas{errEvaluar: recaudo.ErrBolsaInvalida}
	h := servidorConAnomalias(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, "/alertas/evaluacion", `{"periodo":"2025-13"}`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestEvaluarRechazaUnCuerpoQueNoEsJSON(t *testing.T) {
	h := servidorConAnomalias(t, aplicacion.RolAdministrador, &anomaliasFalsas{})

	rec := pedir(t, h, http.MethodPost, "/alertas/evaluacion", `no soy json`, "tok")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
	}
}

// ---------------------------------------------------------------------------
// POST /alertas/{id}/resolver

func TestResolverAlertaDevuelveLaAlertaCerrada(t *testing.T) {
	cerrada := alertaDeEjemplo()
	cerrada.Resuelta = true
	cerrada.ResueltaPor = "usr-1"
	cerrada.ResueltaEn = &instanteAlertaHTTP
	cerrada.Nota = "hablado con la autora"

	falso := &anomaliasFalsas{alerta: cerrada}
	h := servidorConAnomalias(t, aplicacion.RolDistribucion, falso)

	ruta := "/alertas/" + cerrada.ID + "/resolver"
	rec := pedir(t, h, http.MethodPost, ruta, `{"nota":"hablado con la autora"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.idRecibido != cerrada.ID || falso.notaRecibida != "hablado con la autora" {
		t.Fatalf("recibido id=%q nota=%q", falso.idRecibido, falso.notaRecibida)
	}
	if falso.actorRecibido != "usr-1" {
		t.Fatalf("actor recibido = %q, se esperaba el de la sesion", falso.actorRecibido)
	}

	var cuerpo alertaJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if !cuerpo.Resuelta || cuerpo.ResueltaPor != "usr-1" || cuerpo.Nota != "hablado con la autora" {
		t.Fatalf("cuerpo = %+v", cuerpo)
	}
}

// La nota es opcional, asi que un POST sin cuerpo se acepta: io.EOF al
// decodificar no es un cuerpo mal formado, es la ausencia de cuerpo.
func TestResolverAlertaAceptaUnPostSinCuerpo(t *testing.T) {
	falso := &anomaliasFalsas{alerta: alertaDeEjemplo()}
	h := servidorConAnomalias(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, "/alertas/al-1/resolver", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.notaRecibida != "" {
		t.Fatalf("nota recibida = %q", falso.notaRecibida)
	}
}

func TestResolverAlertaTraduceLosCentinelas(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		codigo int
	}{
		{"no existe", aplicacion.ErrNoEncontrado, http.StatusNotFound},
		// Llegar segundo no es un fallo del servidor ni un exito: quien pulso
		// el boton tiene que saber que la firma escrita no es la suya.
		{"ya resuelta", aplicacion.ErrAlertaYaResuelta, http.StatusConflict},
		{"cualquier otro fallo", fmt.Errorf("base caida"), http.StatusInternalServerError},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			falso := &anomaliasFalsas{errResolver: c.err}
			h := servidorConAnomalias(t, aplicacion.RolDistribucion, falso)

			rec := pedir(t, h, http.MethodPost, "/alertas/al-1/resolver", `{}`, "tok")
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.codigo, rec.Body)
			}
			// Los errores salen en JSON, no en text/plain.
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Fatalf("content-type = %q", ct)
			}
		})
	}
}
