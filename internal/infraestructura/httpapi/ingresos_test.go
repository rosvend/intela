package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
)

type ingresosFalsos struct {
	filas  []aplicacion.Ingreso
	err    error
	actor  aplicacion.Usuario
	filtro aplicacion.FiltroIngresos
}

func (i *ingresosFalsos) MisIngresos(_ context.Context, actor aplicacion.Usuario, f aplicacion.FiltroIngresos) ([]aplicacion.Ingreso, error) {
	i.actor = actor
	i.filtro = f
	return i.filas, i.err
}

type explicarFalso struct {
	valor aplicacion.Explicacion
	err   error
	actor aplicacion.Usuario
	ref   string
}

func (e *explicarFalso) Explicar(_ context.Context, actor aplicacion.Usuario, ref string) (aplicacion.Explicacion, error) {
	e.actor = actor
	e.ref = ref
	return e.valor, e.err
}

func titularAna() aplicacion.Usuario {
	return aplicacion.Usuario{ID: "usr-ana", Email: "ana@redes.co", Nombre: "Ana", Rol: aplicacion.RolTitular, TitularID: "tit-ana"}
}

func filaAna() aplicacion.Ingreso {
	return aplicacion.Ingreso{
		Ref:     aplicacion.FormarRef("proc-2026-01", "obra-completa", "tit-ana"),
		ObraID:  "obra-completa",
		Obra:    "La Casa de las Dos Palmas",
		Fuente:  "caracol",
		Periodo: "2026-01",
		Neto:    decimal.RequireFromString("3600.00"),
	}
}

func linajeAna() aplicacion.Explicacion {
	return aplicacion.Explicacion{
		Ref:       filaAna().Ref,
		TitularID: "tit-ana",
		Neto:      decimal.RequireFromString("3600.00"),
		Bruto:     decimal.RequireFromString("4800.00"),
		Corrida:   aplicacion.CorridaLinaje{ProcesoID: "proc-2026-01", Periodo: "2026-01", Circuito: "nacional"},
		Reporte:   aplicacion.ReporteLinaje{ID: "rpt-caracol-2026-01", Fuente: "caracol", SHA256: "aa"},
		Obra:      aplicacion.ObraLinaje{ID: "obra-completa", Titulo: "La Casa de las Dos Palmas", Escalon: "alias", Puntaje: decimal.RequireFromString("1")},
		Regla:     aplicacion.ReglaLinaje{SnapshotID: "snap-2026-01", Reglamento: "RD-IX"},
		Split:     aplicacion.SplitLinaje{TitularID: "tit-ana", IPI: "IPI-00000001", Porcentaje: decimal.RequireFromString("60"), Version: 1},
		Deducciones: []aplicacion.Deduccion{
			{Concepto: "gastos administrativos", Porcentaje: decimal.RequireFromString("10.00"), Monto: decimal.RequireFromString("480.00")},
		},
	}
}

func TestMisIngresosDevuelveSoloLoDelTitular(t *testing.T) {
	ing := &ingresosFalsos{filas: []aplicacion.Ingreso{filaAna()}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, ing, nil), http.MethodGet, "/mis-ingresos", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ing.actor.TitularID != "tit-ana" {
		t.Fatalf("el caso de uso recibio TitularID %q", ing.actor.TitularID)
	}
	var cuerpo listaIngresosJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cuerpo.Ingresos) != 1 || cuerpo.Ingresos[0].ObraID != "obra-completa" {
		t.Fatalf("ingresos = %+v", cuerpo.Ingresos)
	}
	if cuerpo.Ingresos[0].Neto != "3600.00" {
		t.Fatalf("neto = %q", cuerpo.Ingresos[0].Neto)
	}
	if strings.Contains(rec.Body.String(), `"bruto"`) {
		t.Fatal("el bruto no puede ir en el listado (OE-6)")
	}
}

func TestMisIngresosPasaLosFiltros(t *testing.T) {
	ing := &ingresosFalsos{filas: []aplicacion.Ingreso{}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, ing, nil), http.MethodGet,
		"/mis-ingresos?obra=obra-completa&fuente=caracol&periodo=2026-01", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if ing.filtro.ObraID != "obra-completa" || ing.filtro.Fuente != "caracol" || ing.filtro.Periodo != "2026-01" {
		t.Fatalf("filtro = %+v", ing.filtro)
	}
}

func TestMisIngresosIgnoraTitularIDEnLaQuery(t *testing.T) {
	ing := &ingresosFalsos{filas: []aplicacion.Ingreso{filaAna()}}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, ing, nil), http.MethodGet,
		"/mis-ingresos?titular_id=tit-beto", "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if ing.actor.TitularID != "tit-ana" {
		t.Fatalf("se uso un titular de la query: %q", ing.actor.TitularID)
	}
}

func TestMisIngresosOtroRolEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolAdministrador}}
	rec := pedir(t, servidorCon(t, auth, &ingresosFalsos{}, nil), http.MethodGet, "/mis-ingresos", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
}

func TestMisIngresosSinSesionEs401(t *testing.T) {
	rec := pedir(t, servidorCon(t, &autenticacionFalsa{}, &ingresosFalsos{}, nil), http.MethodGet, "/mis-ingresos", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}

func TestExplicarDevuelveElLinaje(t *testing.T) {
	x := linajeAna()
	exp := &explicarFalso{valor: x}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, nil, exp), http.MethodGet, "/explicar/"+x.Ref, "", "tok")

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d. Cuerpo: %s", rec.Code, rec.Body)
	}
	if exp.ref != x.Ref {
		t.Fatalf("ref recibida = %q", exp.ref)
	}
	var cuerpo explicacionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if cuerpo.Neto != "3600.00" || cuerpo.Bruto != "4800.00" {
		t.Fatalf("neto/bruto = %s/%s", cuerpo.Neto, cuerpo.Bruto)
	}
	if cuerpo.Corrida.ProcesoID != "proc-2026-01" {
		t.Fatalf("corrida = %+v", cuerpo.Corrida)
	}
	if cuerpo.Reporte.Fuente != "caracol" || cuerpo.Obra.Escalon != "alias" {
		t.Fatalf("origen = %+v %+v", cuerpo.Reporte, cuerpo.Obra)
	}
	if cuerpo.Regla.SnapshotID != "snap-2026-01" || cuerpo.Split.Version != 1 {
		t.Fatalf("regla/split = %+v %+v", cuerpo.Regla, cuerpo.Split)
	}
	if len(cuerpo.Deducciones) != 1 || cuerpo.Deducciones[0].Concepto != "gastos administrativos" {
		t.Fatalf("deducciones = %+v", cuerpo.Deducciones)
	}
}

func TestExplicarCifraAjenaEs403(t *testing.T) {
	refBeto := aplicacion.FormarRef("proc-2026-01", "obra-beto", "tit-beto")
	exp := &explicarFalso{err: aplicacion.ErrNoAutorizado}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, nil, exp), http.MethodGet, "/explicar/"+refBeto, "", "tok")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403. Cuerpo: %s", rec.Code, rec.Body)
	}
	if exp.ref != refBeto {
		t.Fatalf("ref = %q", exp.ref)
	}
	cuerpo := decodificar(t, rec)
	if cuerpo["error"] != aplicacion.ErrNoAutorizado.Error() {
		t.Fatalf("error = %v", cuerpo["error"])
	}
}

func TestExplicarNoEncontradaEs404(t *testing.T) {
	exp := &explicarFalso{err: aplicacion.ErrNoEncontrado}
	auth := &autenticacionFalsa{usuario: titularAna()}

	rec := pedir(t, servidorCon(t, auth, nil, exp), http.MethodGet, "/explicar/proc-x:obra-x:tit-ana", "", "tok")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("codigo = %d, se esperaba 404", rec.Code)
	}
}

func TestExplicarDistribucionEs403(t *testing.T) {
	auth := &autenticacionFalsa{usuario: aplicacion.Usuario{ID: "usr-1", Rol: aplicacion.RolDistribucion}}
	rec := pedir(t, servidorCon(t, auth, nil, &explicarFalso{}), http.MethodGet, "/explicar/a:b:c", "", "tok")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("codigo = %d, se esperaba 403", rec.Code)
	}
}

func TestExplicarSinSesionEs401(t *testing.T) {
	rec := pedir(t, servidorCon(t, &autenticacionFalsa{}, nil, &explicarFalso{}), http.MethodGet, "/explicar/a:b:c", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("codigo = %d, se esperaba 401", rec.Code)
	}
}
