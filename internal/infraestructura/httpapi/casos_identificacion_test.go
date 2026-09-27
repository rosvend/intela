package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

type casosIdentificacionFalsos struct {
	pagina aplicacion.PaginaCasos
	err    error
	filtro aplicacion.FiltroCasos
}

func (c *casosIdentificacionFalsos) Listar(_ context.Context, f aplicacion.FiltroCasos) (aplicacion.PaginaCasos, error) {
	c.filtro = f
	return c.pagina, c.err
}

func servidorConCasos(t *testing.T, rol aplicacion.Rol, casos CasosIdentificacion) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol}}
	return Nueva(Casos{Auth: auth, Identificacion: casos}, Opciones{}).Router()
}

func paginaDePrueba() aplicacion.PaginaCasos {
	creado := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	resuelto := time.Date(2025, 2, 3, 9, 0, 0, 0, time.UTC)
	return aplicacion.PaginaCasos{Pendientes: 1, Casos: []aplicacion.CasoIdentificacion{
		{
			UsoID: "u-1", Titulo: "La Casa", TituloOriginal: "The House", Fuente: "caracol",
			Modalidad: "tv", ReporteID: "rep-1", Periodo: "2025-01", IDsFuente: "id_ficha=1",
			Evidencia: "banda ambigua", Estado: aplicacion.EstadoCasoPendiente,
			UltimaActualizacion: creado,
			Candidatos: []aplicacion.CandidatoCaso{{
				ObraID: "o-1", Titulo: "La Casa de Papel", Anio: 2017, Genero: "Drama",
				Puntaje: decimal.RequireFromString("0.61"), TituloConsultado: "la casa",
			}},
		},
		{
			UsoID: "u-2", Estado: aplicacion.EstadoCasoAsignado, Candidatos: []aplicacion.CandidatoCaso{},
			ObraAsignada: &aplicacion.ObraAsignada{ID: "o-1", Titulo: "La Casa de Papel"},
			ResueltoPor:  &aplicacion.Resolutor{ID: "usr-9", Nombre: "Revisora"},
			ResueltoEn:   &resuelto, UltimaActualizacion: resuelto,
		},
	}}
}

func TestListarCasosIdentificacionDevuelveCasosYPendientes(t *testing.T) {
	casos := &casosIdentificacionFalsos{pagina: paginaDePrueba()}
	rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, casos), http.MethodGet,
		"/identificacion/casos?estado=pendiente&fuente=caracol&periodo=2025-01&limite=10&desplazamiento=5", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	quiere := aplicacion.FiltroCasos{Estado: "pendiente", Fuente: "caracol", Periodo: "2025-01",
		Paginacion: aplicacion.Paginacion{Limite: 10, Desplazamiento: 5}}
	if casos.filtro != quiere {
		t.Fatalf("filtro = %+v, quiere %+v", casos.filtro, quiere)
	}
	var cuerpo paginaCasosJSON
	if err := json.NewDecoder(rec.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	if cuerpo.Pendientes != 1 || len(cuerpo.Casos) != 2 {
		t.Fatalf("cuerpo = %+v", cuerpo)
	}
	p := cuerpo.Casos[0]
	if p.ID != "u-1" || p.Estado != "pendiente" || p.ReporteID != "rep-1" || p.TituloOriginal != "The House" ||
		p.ResueltoPor != nil || p.ResueltoEn != nil || p.ObraAsignada != nil {
		t.Fatalf("pendiente = %+v", p)
	}
	if len(p.Candidatos) != 1 || p.Candidatos[0].ObraID != "o-1" || p.Candidatos[0].Anio != 2017 {
		t.Fatalf("candidatos = %+v", p.Candidatos)
	}
	a := cuerpo.Casos[1]
	if a.ResueltoPor == nil || a.ResueltoPor.Nombre != "Revisora" || a.ObraAsignada == nil || a.ResueltoEn == nil {
		t.Fatalf("asignado = %+v", a)
	}
}

func TestCasosIdentificacionPuntajeViajaComoNumeroYCandidatosVacioEsLista(t *testing.T) {
	rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, &casosIdentificacionFalsos{pagina: paginaDePrueba()}),
		http.MethodGet, "/identificacion/casos", "", "tok")
	cuerpo := rec.Body.String()
	if !strings.Contains(cuerpo, `"puntaje":0.61`) {
		t.Fatalf("puntaje no es numero JSON: %s", cuerpo)
	}
	if !strings.Contains(cuerpo, `"candidatos":[]`) {
		t.Fatalf("un caso sin candidatos tiene que ser []: %s", cuerpo)
	}
}

func TestCasosIdentificacionNoLlevaDineroNiMedidas(t *testing.T) {
	rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, &casosIdentificacionFalsos{pagina: paginaDePrueba()}),
		http.MethodGet, "/identificacion/casos", "", "tok")
	var cuerpo map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&cuerpo); err != nil {
		t.Fatalf("decodificar: %v", err)
	}
	prohibidas := []string{"importe", "monto", "bruto", "neto", "rating", "taquilla", "vistas",
		"minutos_vistos", "emisiones", "duracion_min", "espectadores", "exhibiciones", "pb"}
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

func TestCasosIdentificacionVacioEsListaVacia(t *testing.T) {
	casos := &casosIdentificacionFalsos{pagina: aplicacion.PaginaCasos{Casos: []aplicacion.CasoIdentificacion{}}}
	rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, casos), http.MethodGet, "/identificacion/casos", "", "tok")
	if cuerpo := strings.TrimSpace(rec.Body.String()); cuerpo != `{"casos":[],"pendientes":0}` {
		t.Fatalf("cuerpo = %s", cuerpo)
	}
}

func TestCasosIdentificacionFiltroMalformadoEs400(t *testing.T) {
	for _, ruta := range []string{"/identificacion/casos?limite=0", "/identificacion/casos?desplazamiento=-1"} {
		rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, &casosIdentificacionFalsos{}), http.MethodGet, ruta, "", "tok")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: codigo = %d", ruta, rec.Code)
		}
	}
	casos := &casosIdentificacionFalsos{err: fmt.Errorf("%w: periodo", aplicacion.ErrFiltroCasosInvalido)}
	rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, casos), http.MethodGet, "/identificacion/casos?periodo=2025-13", "", "tok")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "periodo") {
		t.Fatalf("codigo = %d cuerpo = %s", rec.Code, rec.Body)
	}
}

func TestCasosIdentificacionErrorInternoEs500Generico(t *testing.T) {
	casos := &casosIdentificacionFalsos{err: fmt.Errorf("pgx: conexion rota")}
	rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, casos), http.MethodGet, "/identificacion/casos", "", "tok")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "pgx") {
		t.Fatalf("codigo = %d cuerpo = %s", rec.Code, rec.Body)
	}
}

func TestCasosIdentificacionExigeAdministrador(t *testing.T) {
	for _, rol := range []aplicacion.Rol{aplicacion.RolTitular, aplicacion.RolAuditor, aplicacion.RolDistribucion, aplicacion.RolContabilidad} {
		rec := pedir(t, servidorConCasos(t, rol, &casosIdentificacionFalsos{}), http.MethodGet, "/identificacion/casos", "", "tok")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: codigo = %d, quiere 403", rol, rec.Code)
		}
	}
}

func TestCasosIdentificacionSinSesionEs401(t *testing.T) {
	h := Nueva(Casos{Auth: &autenticacionFalsa{}, Identificacion: &casosIdentificacionFalsos{}}, Opciones{}).Router()
	if rec := pedir(t, h, http.MethodGet, "/identificacion/casos", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, quiere 401", rec.Code)
	}
}

func TestCasosIdentificacionSinCasoDeUsoEs503(t *testing.T) {
	if rec := pedir(t, servidorConCasos(t, aplicacion.RolAdministrador, nil), http.MethodGet, "/identificacion/casos", "", "tok"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, quiere 503", rec.Code)
	}
}
