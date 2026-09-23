package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
)

// auditoriaFalsa registra lo que le llega y devuelve lo que le pongan: estas
// pruebas comprueban el ADAPTADOR, no la base.
type auditoriaFalsa struct {
	asientos []aplicacion.Asiento
	err      error

	pagRecibida    aplicacion.Paginacion
	obraIDRecibida string
}

func (a *auditoriaFalsa) Asientos(_ context.Context, pag aplicacion.Paginacion) ([]aplicacion.Asiento, error) {
	a.pagRecibida = pag
	return a.asientos, a.err
}

func (a *auditoriaFalsa) HistorialDeObra(_ context.Context, obraID string) ([]aplicacion.Asiento, error) {
	a.obraIDRecibida = obraID
	return a.asientos, a.err
}

func asientoFalso(id, hecho, refTipo, refID, actor string) aplicacion.Asiento {
	return aplicacion.Asiento{
		ID:      id,
		Hecho:   hecho,
		RefTipo: refTipo,
		RefID:   refID,
		ActorID: actor,
		Payload: []byte(`{"version":2}`),
		Cuando:  time.Date(2026, 4, 2, 10, 30, 0, 0, time.UTC),
	}
}

// servidorConAuditoria monta el router con una sesion de auditor ya resuelta,
// que es lo que exigen las dos rutas.
func servidorConAuditoria(t *testing.T, a Auditoria) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-auditor", Rol: aplicacion.RolAuditor},
	}
	return Nueva(Casos{Auth: auth, Auditoria: a}, Opciones{}).Router()
}

func decodificarAsientos(t *testing.T, rec *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var cuerpo []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("el cuerpo no es una lista de asientos: %v. Cuerpo: %s", err, rec.Body)
	}
	return cuerpo
}

func TestAuditoriaExigeAuditorOAdministrador(t *testing.T) {
	peticiones := []struct{ metodo, ruta string }{
		{http.MethodGet, "/auditoria/asientos"},
		{http.MethodGet, "/auditoria/obra/obra-1"},
	}
	for _, rol := range []aplicacion.Rol{aplicacion.RolTitular, aplicacion.RolDistribucion, aplicacion.RolContabilidad} {
		auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: rol}}
		h := Nueva(Casos{Auth: auth, Auditoria: &auditoriaFalsa{}}, Opciones{}).Router()
		for _, p := range peticiones {
			t.Run(string(rol)+" "+p.metodo+" "+p.ruta, func(t *testing.T) {
				rec := pedir(t, h, p.metodo, p.ruta, "", "tok")
				if rec.Code != http.StatusForbidden {
					t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
				}
			})
		}
	}
}

func TestListarAsientosDevuelveLaPagina(t *testing.T) {
	falso := &auditoriaFalsa{asientos: []aplicacion.Asiento{
		asientoFalso("a-1", "declaracion.guardada", "obra", "obra-1", "usr-admin"),
	}}
	h := servidorConAuditoria(t, falso)

	rec := pedir(t, h, http.MethodGet, "/auditoria/asientos?limite=10", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.pagRecibida.Limite != 10 {
		t.Fatalf("limite = %d, se esperaba 10", falso.pagRecibida.Limite)
	}
	cuerpo := decodificarAsientos(t, rec)
	if len(cuerpo) != 1 {
		t.Fatalf("asientos = %v, se esperaba uno", cuerpo)
	}
	got := cuerpo[0]
	for campo, quiero := range map[string]string{
		"id": "a-1", "hecho": "declaracion.guardada",
		"ref_tipo": "obra", "ref_id": "obra-1", "actor": "usr-admin",
	} {
		if got[campo] != quiero {
			t.Fatalf("%s = %v, se esperaba %q. Cuerpo: %v", campo, got[campo], quiero, got)
		}
	}
	if _, ok := got["payload"].(map[string]any); !ok {
		t.Fatalf("payload = %v, se esperaba un objeto, no una cadena", got["payload"])
	}
	if _, ok := got["cuando"].(string); !ok {
		t.Fatalf("cuando = %v, se esperaba texto en RFC3339", got["cuando"])
	}
}

func TestListarAsientosRechazaLimiteIlegal(t *testing.T) {
	h := servidorConAuditoria(t, &auditoriaFalsa{})

	for _, ruta := range []string{"/auditoria/asientos?limite=0", "/auditoria/asientos?limite=501"} {
		rec := pedir(t, h, http.MethodGet, ruta, "", "tok")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: codigo = %d, se esperaba 400. Cuerpo: %s", ruta, rec.Code, rec.Body)
		}
	}
}

func TestListarAsientosSinCasoDeUsoEs503(t *testing.T) {
	auth := &autenticacionFalsa{
		usuario: aplicacion.Usuario{ID: "usr-auditor", Rol: aplicacion.RolAuditor},
	}
	h := Nueva(Casos{Auth: auth}, Opciones{}).Router()

	rec := pedir(t, h, http.MethodGet, "/auditoria/asientos", "", "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("codigo = %d, se esperaba 503. Cuerpo: %s", rec.Code, rec.Body)
	}
}

func TestHistorialDeObraDevuelveLaCadena(t *testing.T) {
	falso := &auditoriaFalsa{asientos: []aplicacion.Asiento{
		asientoFalso("a-1", "obra.registrada", "obra", "obra-1", "usr-admin"),
		asientoFalso("a-2", "declaracion.guardada", "obra", "obra-1", "usr-admin"),
	}}
	h := servidorConAuditoria(t, falso)

	rec := pedir(t, h, http.MethodGet, "/auditoria/obra/obra-1", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba 200. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.obraIDRecibida != "obra-1" {
		t.Fatalf("obra = %q, se esperaba %q", falso.obraIDRecibida, "obra-1")
	}
	cuerpo := decodificarAsientos(t, rec)
	if len(cuerpo) != 2 || cuerpo[0]["id"] != "a-1" || cuerpo[1]["id"] != "a-2" {
		t.Fatalf("asientos = %v, se esperaban los dos en orden de cadena", cuerpo)
	}
}

func TestHistorialDeObraPropagaElErrorComo500(t *testing.T) {
	falso := &auditoriaFalsa{err: errors.New("base caida")}
	h := servidorConAuditoria(t, falso)

	rec := pedir(t, h, http.MethodGet, "/auditoria/obra/obra-1", "", "tok")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("codigo = %d, se esperaba 500. Cuerpo: %s", rec.Code, rec.Body)
	}
}
