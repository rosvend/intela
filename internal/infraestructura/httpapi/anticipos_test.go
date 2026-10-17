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
	"github.com/rosvend/intela/internal/dominio/anticipos"
)

type anticiposFalso struct {
	registrado aplicacion.AnticipoRegistrado
	lista      []aplicacion.AnticipoRegistrado
	err        error

	actor       aplicacion.Usuario
	solicitud   aplicacion.SolicitudAnticipo
	idRecibido  string
	textoRecibo string
	llamadas    int
}

func (f *anticiposFalso) Solicitar(_ context.Context, actor aplicacion.Usuario, in aplicacion.SolicitudAnticipo) (aplicacion.AnticipoRegistrado, error) {
	f.llamadas++
	f.actor, f.solicitud = actor, in
	return f.registrado, f.err
}

func (f *anticiposFalso) Aprobar(_ context.Context, actor aplicacion.Usuario, id, soporte string) (aplicacion.AnticipoRegistrado, error) {
	f.llamadas++
	f.actor, f.idRecibido, f.textoRecibo = actor, id, soporte
	return f.registrado, f.err
}

func (f *anticiposFalso) Rechazar(_ context.Context, actor aplicacion.Usuario, id, motivo string) (aplicacion.AnticipoRegistrado, error) {
	f.llamadas++
	f.actor, f.idRecibido, f.textoRecibo = actor, id, motivo
	return f.registrado, f.err
}

func (f *anticiposFalso) Listar(_ context.Context, actor aplicacion.Usuario) ([]aplicacion.AnticipoRegistrado, error) {
	f.llamadas++
	f.actor = actor
	return f.lista, f.err
}

var solicitadoEl = time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)

func anticipoDeEjemplo() aplicacion.AnticipoRegistrado {
	return aplicacion.AnticipoRegistrado{
		Anticipo: anticipos.Anticipo{
			ID: "ant-1", TitularID: "tit-ana",
			Monto: decimal.RequireFromString("1000000"), Saldo: decimal.RequireFromString("400000.5"),
			Estado: anticipos.EstadoAprobado, Justificacion: "vivienda", Soporte: "acta 12",
		},
		Solicitado:  solicitadoEl,
		Resuelto:    solicitadoEl.Add(48 * time.Hour),
		ResueltoPor: "usr-admin",
	}
}

func servidorConAnticipos(t *testing.T, usuario aplicacion.Usuario, ant Anticipos) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: usuario}
	return Nueva(Casos{Auth: auth, Anticipos: ant}, Opciones{}).Router()
}

func socioDePrueba() aplicacion.Usuario {
	return aplicacion.Usuario{ID: "usr-ana", Rol: aplicacion.RolTitular, TitularID: "tit-ana"}
}

// TestAnticiposRolesPorRuta es la matriz de roles.md para /anticipos: pedir es
// del titular (RA 3.1.f), decidir de administrador (Consejo, RA 3.2.3), leer
// del staff y del titular. El middleware cierra la puerta ANTES del caso de uso:
// un 403 aqui no llega a llamarlo.
func TestAnticiposRolesPorRuta(t *testing.T) {
	staff := []aplicacion.Rol{
		aplicacion.RolAdministrador, aplicacion.RolDistribucion,
		aplicacion.RolContabilidad, aplicacion.RolAuditor,
	}
	todos := append([]aplicacion.Rol{aplicacion.RolTitular}, staff...)
	rutas := []struct {
		metodo, ruta, cuerpo string
		permitidos           []aplicacion.Rol
		exito                int
	}{
		{http.MethodGet, "/anticipos", "", todos, http.StatusOK},
		{http.MethodPost, "/anticipos", `{"monto":"1000.00","justificacion":"x"}`, []aplicacion.Rol{aplicacion.RolTitular}, http.StatusCreated},
		{http.MethodPost, "/anticipos/ant-1/aprobar", `{"soporte":"acta"}`, []aplicacion.Rol{aplicacion.RolAdministrador}, http.StatusOK},
		{http.MethodPost, "/anticipos/ant-1/rechazar", `{"motivo":"no"}`, []aplicacion.Rol{aplicacion.RolAdministrador}, http.StatusOK},
	}
	for _, r := range rutas {
		for _, rol := range todos {
			t.Run(fmt.Sprintf("%s %s como %s", r.metodo, r.ruta, rol), func(t *testing.T) {
				falso := &anticiposFalso{registrado: anticipoDeEjemplo()}
				usuario := aplicacion.Usuario{ID: "usr-" + string(rol), Rol: rol}
				if rol == aplicacion.RolTitular {
					usuario = socioDePrueba()
				}
				rec := pedir(t, servidorConAnticipos(t, usuario, falso), r.metodo, r.ruta, r.cuerpo, "tok")
				permitido := false
				for _, p := range r.permitidos {
					permitido = permitido || p == rol
				}
				switch {
				case permitido && rec.Code != r.exito:
					t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, r.exito, rec.Body)
				case !permitido && rec.Code != http.StatusForbidden:
					t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
				case !permitido && falso.llamadas != 0:
					t.Fatal("un rol sin permiso llego al caso de uso")
				}
			})
		}
	}
}

// TestSolicitarAnticipoHTTP: el monto viaja como cadena exacta, el titular
// sale de la sesion y la respuesta lleva el dinero con dos decimales.
func TestSolicitarAnticipoHTTP(t *testing.T) {
	falso := &anticiposFalso{registrado: anticipoDeEjemplo()}
	h := servidorConAnticipos(t, socioDePrueba(), falso)

	rec := pedir(t, h, http.MethodPost, "/anticipos",
		`{"monto":"1000000.10","justificacion":"tratamiento medico","titular_id":"tit-otro"}`, "tok")
	if rec.Code != http.StatusCreated {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if !falso.solicitud.Monto.Equal(decimal.RequireFromString("1000000.10")) || falso.solicitud.Justificacion != "tratamiento medico" {
		t.Fatalf("solicitud = %+v", falso.solicitud)
	}
	if falso.actor.TitularID != "tit-ana" {
		t.Fatalf("actor = %+v; el titular sale de la sesion, no del cuerpo", falso.actor)
	}
	// No hay GET /anticipos/{id}: un Location apuntaria a una ruta que no
	// existe.
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("Location = %q; no hay ruta del anticipo a la que apuntar", loc)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta: %v", err)
	}
	if cuerpo["monto"] != "1000000.00" || cuerpo["saldo"] != "400000.50" || cuerpo["estado"] != "aprobado" {
		t.Fatalf("respuesta = %v; el dinero viaja como cadena con dos decimales", cuerpo)
	}
	if cuerpo["solicitado"] != "2026-06-15T14:00:00Z" || cuerpo["resuelto"] != "2026-06-17T14:00:00Z" || cuerpo["resuelto_por"] != "usr-admin" {
		t.Fatalf("respuesta = %v", cuerpo)
	}

	if rec := pedir(t, h, http.MethodPost, "/anticipos", `{"monto":`, "tok"); rec.Code != http.StatusBadRequest {
		t.Fatalf("cuerpo roto: %d", rec.Code)
	}
}

// TestSolicitarAnticipoRechazaElMontoQueNoEsCadena: el contrato declara el
// monto como cadena, porque un JSON number ya paso por coma flotante
// (ADR 0010). Un number, aunque decimal.Decimal lo aceptaria, es un 400 que no
// llega al caso de uso; igual que una cadena que no es un importe.
func TestSolicitarAnticipoRechazaElMontoQueNoEsCadena(t *testing.T) {
	for _, cuerpo := range []string{
		`{"monto":1000000.10,"justificacion":"tratamiento medico"}`,
		`{"monto":1000000,"justificacion":"tratamiento medico"}`,
		`{"monto":"mil","justificacion":"tratamiento medico"}`,
		`{"justificacion":"tratamiento medico"}`,
	} {
		t.Run(cuerpo, func(t *testing.T) {
			falso := &anticiposFalso{registrado: anticipoDeEjemplo()}
			rec := pedir(t, servidorConAnticipos(t, socioDePrueba(), falso), http.MethodPost, "/anticipos", cuerpo, "tok")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("codigo = %d, se esperaba 400. Cuerpo: %s", rec.Code, rec.Body)
			}
			if falso.llamadas != 0 {
				t.Fatal("un monto que no es cadena decimal llego al caso de uso")
			}
		})
	}
}

// TestSolicitudEnTramiteNoLlevaResolucion: sin decidir, `resuelto` no viaja
// como un instante cero que se lea como una fecha.
func TestSolicitudEnTramiteNoLlevaResolucion(t *testing.T) {
	en := anticipoDeEjemplo()
	en.Anticipo.Estado, en.Anticipo.Saldo, en.Anticipo.Soporte = anticipos.EstadoSolicitado, decimal.Zero, ""
	en.Resuelto, en.ResueltoPor = time.Time{}, ""
	falso := &anticiposFalso{lista: []aplicacion.AnticipoRegistrado{en}}
	rec := pedir(t, servidorConAnticipos(t, socioDePrueba(), falso), http.MethodGet, "/anticipos", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	var cuerpo struct {
		Anticipos []map[string]any `json:"anticipos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil || len(cuerpo.Anticipos) != 1 {
		t.Fatalf("respuesta = %s, %v", rec.Body, err)
	}
	for _, campo := range []string{"resuelto", "resuelto_por", "soporte"} {
		if _, hay := cuerpo.Anticipos[0][campo]; hay {
			t.Fatalf("una solicitud en tramite lleva %q: %v", campo, cuerpo.Anticipos[0])
		}
	}
	if cuerpo.Anticipos[0]["saldo"] != "0.00" {
		t.Fatalf("saldo = %v", cuerpo.Anticipos[0]["saldo"])
	}
}

func TestListarAnticiposVacioEsUnaListaYNoNull(t *testing.T) {
	falso := &anticiposFalso{}
	rec := pedir(t, servidorConAnticipos(t, aplicacion.Usuario{ID: "u", Rol: aplicacion.RolAuditor}, falso),
		http.MethodGet, "/anticipos", "", "tok")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"anticipos":[]`) {
		t.Fatalf("codigo = %d, cuerpo = %s", rec.Code, rec.Body)
	}
}

func TestAprobarYRechazarPasanElTextoDelConsejo(t *testing.T) {
	consejo := aplicacion.Usuario{ID: "usr-admin", Rol: aplicacion.RolAdministrador}
	falso := &anticiposFalso{registrado: anticipoDeEjemplo()}
	h := servidorConAnticipos(t, consejo, falso)

	if rec := pedir(t, h, http.MethodPost, "/anticipos/ant-9/aprobar", `{"soporte":"acta 12, resolucion 34"}`, "tok"); rec.Code != http.StatusOK {
		t.Fatalf("aprobar = %d", rec.Code)
	}
	if falso.idRecibido != "ant-9" || falso.textoRecibo != "acta 12, resolucion 34" || falso.actor.ID != "usr-admin" {
		t.Fatalf("aprobar recibio %q/%q/%+v", falso.idRecibido, falso.textoRecibo, falso.actor)
	}
	if rec := pedir(t, h, http.MethodPost, "/anticipos/ant-8/rechazar", `{"motivo":"sin soportes de RA 2.3"}`, "tok"); rec.Code != http.StatusOK {
		t.Fatalf("rechazar = %d", rec.Code)
	}
	if falso.idRecibido != "ant-8" || falso.textoRecibo != "sin soportes de RA 2.3" {
		t.Fatalf("rechazar recibio %q/%q", falso.idRecibido, falso.textoRecibo)
	}
}

// TestErroresDeAnticiposSeTraducen: cada centinela con su codigo. El motivo de
// una regla viaja en el mensaje: es lo que el socio necesita leer.
func TestErroresDeAnticiposSeTraducen(t *testing.T) {
	casos := []struct {
		err    error
		codigo int
	}{
		{aplicacion.ErrNoAutorizado, http.StatusForbidden},
		{fmt.Errorf("anticipo: %w", aplicacion.ErrNoEncontrado), http.StatusNotFound},
		{fmt.Errorf("%w: falta la justificacion", anticipos.ErrSolicitudInvalida), http.StatusBadRequest},
		{fmt.Errorf("%w: falta el soporte", anticipos.ErrResolucionIncompleta), http.StatusBadRequest},
		{fmt.Errorf("%w: ant-0", anticipos.ErrAnticipoVigente), http.StatusConflict},
		{fmt.Errorf("%w: ant-0", anticipos.ErrSaldoPendiente), http.StatusConflict},
		{fmt.Errorf("%w: ya resuelto", anticipos.ErrEstadoInvalido), http.StatusConflict},
		{fmt.Errorf("%w: administrado", anticipos.ErrNoEsSocio), http.StatusUnprocessableEntity},
		{fmt.Errorf("%w: el tope es 1000.00", anticipos.ErrSuperaTope), http.StatusUnprocessableEntity},
		{fmt.Errorf("%w: oficio 3", anticipos.ErrEmbargoReportado), http.StatusUnprocessableEntity},
		{fmt.Errorf("%w: smmlv", aplicacion.ErrParametroAusente), http.StatusInternalServerError},
		{fmt.Errorf("la base no responde"), http.StatusInternalServerError},
	}
	for _, c := range casos {
		t.Run(c.err.Error(), func(t *testing.T) {
			falso := &anticiposFalso{err: c.err}
			rec := pedir(t, servidorConAnticipos(t, socioDePrueba(), falso),
				http.MethodPost, "/anticipos", `{"monto":"10.00","justificacion":"x"}`, "tok")
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d. Cuerpo: %s", rec.Code, c.codigo, rec.Body)
			}
			if c.codigo == http.StatusUnprocessableEntity && !strings.Contains(rec.Body.String(), c.err.Error()) {
				t.Fatalf("el cuerpo %s no lleva el motivo %q", rec.Body, c.err)
			}
		})
	}
}

// TestAnticiposSinCablearResponde503 por lo mismo que la ingesta: una
// instalacion sin el modulo dice que le falta, no revienta en un 500.
func TestAnticiposSinCablearResponde503(t *testing.T) {
	h := Nueva(Casos{Auth: &autenticacionFalsa{usuario: socioDePrueba()}}, Opciones{}).Router()
	if rec := pedir(t, h, http.MethodGet, "/anticipos", "", "tok"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, se esperaba 503", rec.Code)
	}
}
