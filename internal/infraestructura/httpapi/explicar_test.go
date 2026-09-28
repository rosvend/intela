package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

type explicadorFalso struct {
	x         aplicacion.Explicacion
	err       error
	actor     aplicacion.Usuario
	refPedida string
	llamadas  int
}

func (e *explicadorFalso) Explicar(_ context.Context, actor aplicacion.Usuario, ref string) (aplicacion.Explicacion, error) {
	e.llamadas++
	e.actor, e.refPedida = actor, ref
	return e.x, e.err
}

func explicacionDeEjemplo() aplicacion.Explicacion {
	v := 3
	return aplicacion.Explicacion{
		Ref: "proc-1:obra-1:titular-1", TitularID: "titular-1",
		Neto: decimal.RequireFromString("650"), Bruto: decimal.RequireFromString("1000"),
		Corrida:        aplicacion.CorridaLinaje{ProcesoID: "proc-1", Periodo: "2026-01", Circuito: "nacional"},
		Bolsa:          aplicacion.BolsaLinaje{ID: "bolsa-1", UsuarioID: "caracol", Bruto: decimal.RequireFromString("1000000")},
		Reporte:        aplicacion.ReporteAsentado{ID: "rep-1", Fuente: "caracol", SHA256: "abc", ClaveObjeto: "crudos/abc"},
		Reportes:       []aplicacion.ReporteAsentado{{ID: "rep-1", Fuente: "caracol", SHA256: "abc", ClaveObjeto: "crudos/abc"}},
		Obra:           aplicacion.ObraLinaje{ID: "obra-1", Titulo: "Obra Uno", Escalon: "difuso", Puntaje: "0.91"},
		Identificacion: []aplicacion.IdentificacionDeUso{{UsoID: "u-1", ReporteID: "rep-1", Escalon: "difuso", Puntaje: "0.91"}},
		Regla:          aplicacion.ReglaLinaje{SnapshotID: "snap-1", Reglamento: "RD-IX"},
		Split:          &aplicacion.SplitLinaje{TitularID: "titular-1", IPI: "IPI-1", Porcentaje: decimal.RequireFromString("100"), Version: &v},
		Deducciones: []aplicacion.DeduccionLinaje{
			{Concepto: "gastos_administrativos", Porcentaje: decimal.RequireFromString("20"), Monto: decimal.RequireFromString("200")},
		},
		Firmas:    []aplicacion.FirmaLinaje{{Rol: "distribucion", ActorID: "usr-dist", SobreRevision: 1, Etapa: "verificacion", Cuando: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}},
		Faltantes: []string{},
	}
}

func servidorConExplicador(t *testing.T, usuario aplicacion.Usuario, e *explicadorFalso) http.Handler {
	t.Helper()
	auth := &autenticacionFalsa{usuario: usuario}
	return Nueva(Casos{Auth: auth, Explicar: e}, Opciones{}).Router()
}

func TestExplicarDevuelveElLinajeConDineroComoCadena(t *testing.T) {
	falso := &explicadorFalso{x: explicacionDeEjemplo()}
	titular := aplicacion.Usuario{ID: "usr-t1", Rol: aplicacion.RolTitular, TitularID: "titular-1"}

	rec := pedir(t, servidorConExplicador(t, titular, falso), http.MethodGet, "/explicar/proc-1:obra-1:titular-1", "", "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if falso.refPedida != "proc-1:obra-1:titular-1" || falso.actor.TitularID != "titular-1" {
		t.Fatalf("ref/actor = %q / %+v", falso.refPedida, falso.actor)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("cuerpo no es JSON: %v", err)
	}
	if cuerpo["neto"] != "650.00" || cuerpo["bruto"] != "1000.00" {
		t.Fatalf("neto/bruto = %v / %v, el dinero viaja como cadena con dos decimales", cuerpo["neto"], cuerpo["bruto"])
	}
	for _, clave := range []string{"corrida", "bolsa", "reporte", "reportes", "obra", "identificacion", "regla", "split", "deducciones", "firmas", "faltantes"} {
		if _, ok := cuerpo[clave]; !ok {
			t.Errorf("falta %q en la respuesta", clave)
		}
	}
	split := cuerpo["split"].(map[string]any)
	if split["version"] != float64(3) || split["porcentaje"] != "100" {
		t.Fatalf("split = %v", split)
	}
	ded := cuerpo["deducciones"].([]any)[0].(map[string]any)
	if ded["porcentaje"] != "20" || ded["monto"] != "200.00" {
		t.Fatalf("deduccion = %v", ded)
	}
}

func TestExplicarTraduceLosErrores(t *testing.T) {
	admin := aplicacion.Usuario{ID: "usr-a", Rol: aplicacion.RolAdministrador}
	casos := map[error]int{
		aplicacion.ErrNoEncontrado: http.StatusNotFound,
		aplicacion.ErrNoAutorizado: http.StatusForbidden,
	}
	for err, codigo := range casos {
		rec := pedir(t, servidorConExplicador(t, admin, &explicadorFalso{err: err}), http.MethodGet, "/explicar/proc-1:obra-1", "", "tok")
		if rec.Code != codigo {
			t.Errorf("%v: codigo = %d, se esperaba %d", err, rec.Code, codigo)
		}
	}
}

func TestExplicarExigeSesionYRol(t *testing.T) {
	falso := &explicadorFalso{x: explicacionDeEjemplo()}
	rec := pedir(t, servidorConExplicador(t, aplicacion.Usuario{ID: "usr-a", Rol: aplicacion.RolAuditor}, falso), http.MethodGet, "/explicar/proc-1:obra-1", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token = %d, se esperaba 401", rec.Code)
	}
	for _, rol := range []aplicacion.Rol{aplicacion.RolDistribucion, aplicacion.RolContabilidad} {
		rec := pedir(t, servidorConExplicador(t, aplicacion.Usuario{ID: "usr-x", Rol: rol}, falso), http.MethodGet, "/explicar/proc-1:obra-1", "", "tok")
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s = %d, se esperaba 403", rol, rec.Code)
		}
	}
	if falso.llamadas != 0 {
		t.Fatal("el caso de uso no debio llamarse sin sesion o con otro rol")
	}
}
