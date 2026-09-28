package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// resolucionFalsa es el doble del caso de uso: apunta que le llego y devuelve
// lo que le pongan. Lo que se prueba aqui son codigos, cabeceras y la forma del
// JSON; las reglas viven en internal/aplicacion y internal/dominio.
type resolucionFalsa struct {
	caso        aplicacion.CasoIdentificacion
	err         error
	pedido      aplicacion.SolicitudResolucion
	actorID     string
	actorNombre string
	llamadas    int
}

func (r *resolucionFalsa) Resolver(_ context.Context, s aplicacion.SolicitudResolucion,
	actorID, actorNombre string) (aplicacion.CasoIdentificacion, error) {
	r.llamadas++
	r.pedido, r.actorID, r.actorNombre = s, actorID, actorNombre
	if r.err != nil {
		return aplicacion.CasoIdentificacion{}, r.err
	}
	return r.caso, nil
}

func servidorConResolucion(t *testing.T, rol aplicacion.Rol, resolucion ResolucionIdentificacion) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{
		ID: "usr-admin", Nombre: "Ana Perez", Rol: rol,
	}}
	return Nueva(Casos{Auth: auth, Resolucion: resolucion}, Opciones{}).Router()
}

// casoResueltoDePrueba es lo que devuelve el caso de uso: el caso ya leido
// despues de escribir, con el esquema CasoIdentificacion de la lista.
func casoResueltoDePrueba(estado string, obra *aplicacion.ObraAsignada) aplicacion.CasoIdentificacion {
	resuelto := time.Date(2025, 2, 3, 9, 0, 0, 0, time.UTC)
	return aplicacion.CasoIdentificacion{
		UsoID: "u-1", Titulo: "La Casa", Fuente: "caracol", Modalidad: "tv",
		ReporteID: "rep-1", Periodo: "2025-01", IDsFuente: "id_ficha=871732",
		Evidencia: "manual: candidata o-1 (0.52941) de 2 propuestas",
		Estado:    estado, Candidatos: []aplicacion.CandidatoCaso{},
		ObraAsignada: obra, ResueltoEn: &resuelto,
		ResueltoPor:         &aplicacion.Resolutor{ID: "usr-admin", Nombre: "Ana Perez"},
		UltimaActualizacion: resuelto,
		Nota:                "coincide la ficha tecnica",
	}
}

const rutaResolucion = "/identificacion/casos/u-1/resolucion"

func TestResolverCasoAsignaYDevuelveElCaso(t *testing.T) {
	falso := &resolucionFalsa{caso: casoResueltoDePrueba(aplicacion.EstadoCasoAsignado,
		&aplicacion.ObraAsignada{ID: "o-1", Titulo: "La Casa de Papel"})}
	h := servidorConResolucion(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"asignar","obra_id":"o-1","nota":"coincide la ficha tecnica"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}

	quiero := aplicacion.SolicitudResolucion{
		UsoID: "u-1", Decision: "asignar", ObraID: "o-1", Nota: "coincide la ficha tecnica",
	}
	if falso.pedido != quiero {
		t.Fatalf("pedido = %+v, quiere %+v", falso.pedido, quiero)
	}
	// El actor sale de la SESION, no del cuerpo.
	if falso.actorID != "usr-admin" || falso.actorNombre != "Ana Perez" {
		t.Fatalf("actor = %q/%q, se esperaba el de la sesion", falso.actorID, falso.actorNombre)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if cuerpo["estado"] != "asignado" || cuerpo["nota"] != "coincide la ficha tecnica" {
		t.Fatalf("cuerpo = %v", cuerpo)
	}
	obra, ok := cuerpo["obra_asignada"].(map[string]any)
	if !ok || obra["id"] != "o-1" || obra["titulo"] != "La Casa de Papel" {
		t.Fatalf("obra_asignada = %v", cuerpo["obra_asignada"])
	}
	resolutor, ok := cuerpo["resuelto_por"].(map[string]any)
	if !ok || resolutor["id"] != "usr-admin" || resolutor["nombre"] != "Ana Perez" {
		t.Fatalf("resuelto_por = %v", cuerpo["resuelto_por"])
	}
	if cuerpo["resuelto_en"] != "2025-02-03T09:00:00Z" {
		t.Fatalf("resuelto_en = %v", cuerpo["resuelto_en"])
	}
}

// El cuerpo NO puede decidir quien firma: un `actor` colado en el JSON se
// ignora y el asiento sigue llevando el actor de la sesion (ADR 0006).
func TestResolverCasoIgnoraElActorDelCuerpo(t *testing.T) {
	falso := &resolucionFalsa{caso: casoResueltoDePrueba(aplicacion.EstadoCasoAsignado,
		&aplicacion.ObraAsignada{ID: "o-1"})}
	h := servidorConResolucion(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"asignar","obra_id":"o-1","nota":"ok","actor":"usr-otro","actor_id":"usr-otro"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.actorID != "usr-admin" {
		t.Fatalf("actor = %q: el cuerpo no puede firmar la decision", falso.actorID)
	}
}

func TestResolverCasoDescarta(t *testing.T) {
	falso := &resolucionFalsa{caso: casoResueltoDePrueba(aplicacion.EstadoCasoDescartado, nil)}
	h := servidorConResolucion(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"descartar","nota":"no es del repertorio"}`, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if cuerpo["estado"] != "descartado" {
		t.Fatalf("estado = %v", cuerpo["estado"])
	}
	if v, hay := cuerpo["obra_asignada"]; !hay || v != nil {
		t.Fatalf("obra_asignada = %v, se esperaba null", cuerpo["obra_asignada"])
	}
}

// Un caso pendiente no tiene nota: la clave viaja como null, no como "".
func TestElCasoPendienteLlevaNotaNull(t *testing.T) {
	casos := &casosIdentificacionFalsos{pagina: paginaDePrueba()}
	h := servidorConCasos(t, aplicacion.RolAdministrador, casos)

	rec := pedir(t, h, http.MethodGet, "/identificacion/casos", "", "tok")
	var cuerpo struct {
		Casos []map[string]any `json:"casos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if v, hay := cuerpo.Casos[0]["nota"]; !hay || v != nil {
		t.Fatalf("nota del pendiente = %v, se esperaba null", v)
	}
}

// El contrato de la respuesta: los nombres de campo como LITERALES, no
// derivados del struct, por lo mismo que en alertas_contrato_test.go -una
// etiqueta renombrada se mueve con el lector y la prueba seguiria verde-.
func TestElContratoJSONDelCasoResueltoFijaSusNombres(t *testing.T) {
	falso := &resolucionFalsa{caso: casoResueltoDePrueba(aplicacion.EstadoCasoAsignado,
		&aplicacion.ObraAsignada{ID: "o-1", Titulo: "La Casa de Papel"})}
	h := servidorConResolucion(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"asignar","obra_id":"o-1","nota":"coincide la ficha tecnica"}`, "tok")

	var crudo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	// Los que el front de #39 ya consume de esta misma forma en GET /casos,
	// mas los dos que estrena #175.
	for _, campo := range []string{
		"id", "titulo", "titulo_original", "fuente", "modalidad", "reporte_id", "periodo",
		"ids_fuente", "evidencia", "estado", "candidatos", "obra_asignada", "resuelto_por",
		"resuelto_en", "ultima_actualizacion", "nota",
	} {
		if _, hay := crudo[campo]; !hay {
			t.Errorf("falta la clave %q en el JSON del caso resuelto: %v", campo, clavesDe(crudo))
		}
	}
	if crudo["id"] != "u-1" || crudo["estado"] != "asignado" {
		t.Errorf("id/estado = %v/%v", crudo["id"], crudo["estado"])
	}
}

func clavesDe(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestResolverCasoTiposDeError(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
		err    error
		quiero int
		frase  string
	}{
		{"json roto", `{"decision":`, nil, http.StatusBadRequest, "JSON"},
		// Un POST sin cuerpo decodifica como EOF, que NO es un JSON roto: el
		// caso de uso lo recibe con la nota vacia y sale 400 por eso.
		{"cuerpo vacio", "", identificacion.ErrNotaVacia, http.StatusBadRequest, "nota"},
		{
			"nota vacia", `{"decision":"descartar","nota":""}`, identificacion.ErrNotaVacia,
			http.StatusBadRequest, "nota",
		},
		{
			"nota de solo espacios", `{"decision":"descartar","nota":"   "}`, identificacion.ErrNotaVacia,
			http.StatusBadRequest, "nota",
		},
		{
			"nota de mas de 300", `{"decision":"descartar","nota":"x"}`,
			fmt.Errorf("%w: 301", identificacion.ErrNotaDemasiadoLarga),
			http.StatusBadRequest, "300",
		},
		{
			"decision invalida", `{"decision":"reasignar","nota":"ok"}`,
			identificacion.ErrDecisionInvalida, http.StatusBadRequest, "decision",
		},
		{
			"asignar sin obra", `{"decision":"asignar","nota":"ok"}`,
			identificacion.ErrDecisionInvalida, http.StatusBadRequest, "decision",
		},
		{
			"descartar con obra", `{"decision":"descartar","obra_id":"o-1","nota":"ok"}`,
			identificacion.ErrDecisionInvalida, http.StatusBadRequest, "decision",
		},
		{
			"obra inexistente", `{"decision":"asignar","obra_id":"o-x","nota":"ok"}`,
			fmt.Errorf("%w: %q", aplicacion.ErrObraInexistente, "o-x"),
			http.StatusBadRequest, "catalogo",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConResolucion(t, aplicacion.RolAdministrador, &resolucionFalsa{err: c.err})
			rec := pedir(t, h, http.MethodPost, rutaResolucion, c.cuerpo, "tok")
			if rec.Code != c.quiero {
				t.Fatalf("codigo = %d, quiere %d. Cuerpo: %s", rec.Code, c.quiero, rec.Body)
			}
			if c.frase != "" && !strings.Contains(rec.Body.String(), c.frase) {
				t.Errorf("el mensaje no nombra %q: %s", c.frase, rec.Body)
			}
		})
	}
}

// Las tres causas de 409, cada una con su mensaje: el front de #39 las pinta
// distinto (una dice "Otra persona resolvio este caso antes" y ofrece recargar).
func TestResolverCasoConflictos(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		frase  string
	}{
		{"ya lo resolvio otra persona", fmt.Errorf("%w: el caso esta en %q",
			identificacion.ErrCasoYaResuelto, "manual"), "otra persona"},
		{"ya no esta pendiente", fmt.Errorf("%w: el caso esta en %q",
			identificacion.ErrCasoNoPendiente, "difuso"), "difuso"},
		{"alias en conflicto", fmt.Errorf("%w: id_ficha=871732 ya apunta a %q",
			aplicacion.ErrAliasEnConflicto, "obra-40"), "obra-40"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			h := servidorConResolucion(t, aplicacion.RolAdministrador, &resolucionFalsa{err: c.err})
			rec := pedir(t, h, http.MethodPost, rutaResolucion,
				`{"decision":"asignar","obra_id":"o-1","nota":"ok"}`, "tok")
			if rec.Code != http.StatusConflict {
				t.Fatalf("codigo = %d, quiere 409. Cuerpo: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), c.frase) {
				t.Errorf("el mensaje no nombra %q: %s", c.frase, rec.Body)
			}
		})
	}
}

func TestResolverCasoNoEncontrado(t *testing.T) {
	h := servidorConResolucion(t, aplicacion.RolAdministrador,
		&resolucionFalsa{err: aplicacion.ErrNoEncontrado})
	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"descartar","nota":"ok"}`, "tok")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, quiere 404. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestResolverCasoErrorInternoEs500Generico(t *testing.T) {
	h := servidorConResolucion(t, aplicacion.RolAdministrador,
		&resolucionFalsa{err: fmt.Errorf("pgx: conexion rota")})
	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"descartar","nota":"ok"}`, "tok")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "pgx") {
		t.Fatalf("codigo = %d cuerpo = %s", rec.Code, rec.Body)
	}
}

func TestResolverCasoSinSesionEs401(t *testing.T) {
	h := Nueva(Casos{Auth: &autenticacionFalsa{}, Resolucion: &resolucionFalsa{}}, Opciones{}).Router()
	rec := pedir(t, h, http.MethodPost, rutaResolucion, `{"decision":"descartar","nota":"ok"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, quiere 401", rec.Code)
	}
}

func TestResolverCasoExigeAdministrador(t *testing.T) {
	for _, rol := range []aplicacion.Rol{
		aplicacion.RolTitular, aplicacion.RolAuditor, aplicacion.RolDistribucion, aplicacion.RolContabilidad,
	} {
		t.Run(string(rol), func(t *testing.T) {
			rec := pedir(t, servidorConResolucion(t, rol, &resolucionFalsa{}), http.MethodPost,
				rutaResolucion, `{"decision":"descartar","nota":"ok"}`, "tok")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("codigo = %d, quiere 403", rec.Code)
			}
		})
	}
}

func TestResolverCasoSinCasoDeUsoEs503(t *testing.T) {
	rec := pedir(t, servidorConResolucion(t, aplicacion.RolAdministrador, nil), http.MethodPost,
		rutaResolucion, `{"decision":"descartar","nota":"ok"}`, "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, quiere 503", rec.Code)
	}
}

// La respuesta no lleva dinero ni medidas de ponderacion: resolver un caso no
// mueve dinero (ADR 0007) y la identificacion no necesita rating ni taquilla.
func TestLaRespuestaDeResolverNoLlevaDineroNiMedidas(t *testing.T) {
	falso := &resolucionFalsa{caso: casoResueltoDePrueba(aplicacion.EstadoCasoAsignado,
		&aplicacion.ObraAsignada{ID: "o-1", Titulo: "La Casa de Papel"})}
	h := servidorConResolucion(t, aplicacion.RolAdministrador, falso)

	rec := pedir(t, h, http.MethodPost, rutaResolucion,
		`{"decision":"asignar","obra_id":"o-1","nota":"ok"}`, "tok")
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	prohibidas := []string{"importe", "monto", "bruto", "neto", "rating", "taquilla", "vistas",
		"minutos_vistos", "emisiones", "duracion_min", "espectadores", "exhibiciones", "pb",
		"puntos", "ponderacion", "porcentaje"}
	var recorrer func(v any)
	recorrer = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, hijo := range x {
				for _, p := range prohibidas {
					if k == p {
						t.Errorf("la respuesta lleva %q: la identificacion no toca dinero (ADR 0007)", k)
					}
				}
				recorrer(hijo)
			}
		case []any:
			for _, hijo := range x {
				recorrer(hijo)
			}
		}
	}
	recorrer(cuerpo)
}
