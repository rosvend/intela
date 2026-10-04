package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

type origenDeUsosFalso struct {
	porUso map[string]OrigenDeUso
	pedido []string
}

func (o *origenDeUsosFalso) OrigenDeUsos(_ context.Context, ids []string) (map[string]OrigenDeUso, error) {
	o.pedido = append(o.pedido, ids...)
	return o.porUso, nil
}

func puntaje(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

// entornoValorizacion deja proc-1 en deducciones, listo para valorizar obra-1 (declarada) y obra-2 (sin declaracion).
func entornoValorizacion(t *testing.T) (entornoProcesos, *origenDeUsosFalso) {
	t.Helper()
	e := nuevoEntornoProcesos()
	e.guardar(t, reparto.EtapaDeducciones)

	decl, err := repertorio.NuevaDeclaracion("obra-1", []repertorio.Parte{
		{TitularID: "titular-1", IPI: "IPI-1", Porcentaje: d("100")},
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	u1 := usoDeCanal("z", reparto.TV, "")
	u1.Uso.ReporteID, u1.Uso.Escalon = "rep-1", "difuso"
	u2 := usoDeCanal("z", reparto.TV, "")
	u2.Uso.ID, u2.Uso.ObraID, u2.Uso.ReporteID, u2.Uso.Escalon = "u-2", "obra-2", "rep-1", "manual"

	resuelto := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	origen := &origenDeUsosFalso{porUso: map[string]OrigenDeUso{
		"u-1": {UsoID: "u-1", ReporteID: "rep-1", Fuente: "caracol", SHA256: "abc", ClaveObjeto: "crudos/abc", Escalon: "difuso", Puntaje: puntaje("0.91"), Evidencia: "trgm"},
		"u-2": {UsoID: "u-2", ReporteID: "rep-1", Fuente: "caracol", SHA256: "abc", ClaveObjeto: "crudos/abc", Escalon: "manual", ResueltoPor: "usr-admin", ResueltoEn: &resuelto},
	}}

	e.uc.Parametros = &parametrosNormativosFalso{snap: snapshotDePrueba()}
	e.uc.Bolsas = &repositorioRecaudoFalso{bolsa: BolsaPersistida{
		ID: "bolsa-1", UsuarioID: "z", Periodo: "2026-01", Circuito: recaudo.Nacional, Bruto: d("1000000"),
	}}
	e.uc.Declaraciones = &gestionDeclaracionesFalsa{porObra: map[string]VersionDeclaracion{
		"obra-1": {Version: 3, VigenteDesde: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC), Declaracion: decl},
	}}
	e.uc.Usos = &usosDeRepartoFalso{usos: []UsoDeReparto{u1, u2}}
	e.uc.Resultados = &repositorioResultadosFalso{}
	e.uc.Origen = origen
	return e, origen
}

func asientosDe(t *testing.T, b *bitacoraFalsa, hecho string) []Asiento {
	t.Helper()
	var hallados []Asiento
	for _, a := range b.asientos {
		if a.Hecho == hecho {
			hallados = append(hallados, a)
		}
	}
	return hallados
}

func TestValorizarAsientaLaCorridaConSuProcedencia(t *testing.T) {
	t.Parallel()
	e, _ := entornoValorizacion(t)

	if _, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "usr-admin"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	hallados := asientosDe(t, e.bitacora, HechoRepartoValorizado)
	if len(hallados) != 1 {
		t.Fatalf("se esperaba un asiento %q, hubo %d", HechoRepartoValorizado, len(hallados))
	}
	a := hallados[0]
	if a.RefTipo != RefProceso || a.RefID != "proc-1" || a.ActorID != "usr-admin" {
		t.Fatalf("asiento mal referenciado: %+v", a)
	}
	var p AsientoValorizacion
	if err := json.Unmarshal(a.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.SnapshotID != "snap-1" || p.Reglamento != "RD-IX" || p.Bolsa.ID != "bolsa-1" || p.Bolsa.UsuarioID != "z" || p.Bolsa.Bruto != "1000000.00" {
		t.Fatalf("procedencia incompleta: %+v", p)
	}
	if len(p.Deducciones) != 3 || p.Deducciones[0].Concepto != "gastos_administrativos" || p.Deducciones[0].Porcentaje != "20" || p.Deducciones[0].Monto != "200000.00" {
		t.Fatalf("deducciones = %+v, se esperaba concepto, porcentaje y monto", p.Deducciones)
	}
	if p.Neto == "" || p.Retenido == "" || p.NoDistribuido == "" {
		t.Fatalf("cifras de cierre ausentes: %+v", p)
	}
	if len(p.Reportes) != 1 || p.Reportes[0].SHA256 != "abc" || p.Reportes[0].ClaveObjeto != "crudos/abc" {
		t.Fatalf("reportes = %+v, se esperaba el archivo exacto que pondero", p.Reportes)
	}
}

func TestValorizarAsientaCadaObraConSplitYLinaje(t *testing.T) {
	t.Parallel()
	e, _ := entornoValorizacion(t)

	if _, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "usr-admin"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	obras := map[string]AsientoObraValorizada{}
	for _, a := range asientosDe(t, e.bitacora, HechoRepartoObraValorizada) {
		if a.RefTipo != RefObra {
			t.Fatalf("ref_tipo = %q, se esperaba obra", a.RefTipo)
		}
		var p AsientoObraValorizada
		if err := json.Unmarshal(a.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		obras[a.RefID] = p
	}
	if len(obras) != 2 {
		t.Fatalf("se esperaba un asiento por obra, hubo %d", len(obras))
	}

	declarada := obras["obra-1"]
	if declarada.ProcesoID != "proc-1" || declarada.Retenida || declarada.Declaracion == nil || declarada.Declaracion.Version != 3 {
		t.Fatalf("obra-1 = %+v, se esperaba su declaracion en la version usada", declarada)
	}
	if len(declarada.Titulares) != 1 || declarada.Titulares[0].TitularID != "titular-1" || declarada.Titulares[0].Porcentaje != "100" || declarada.Titulares[0].Importe == "" {
		t.Fatalf("titulares = %+v", declarada.Titulares)
	}
	if declarada.Titulares[0].DeclaracionVersion == nil || *declarada.Titulares[0].DeclaracionVersion != 3 {
		t.Fatalf("version de la linea = %v, se esperaba la v3 usada al valorizar", declarada.Titulares[0].DeclaracionVersion)
	}
	guardado := e.uc.Resultados.(*repositorioResultadosFalso).guardado
	if len(guardado.Titulares) != 1 || guardado.Titulares[0].DeclaracionVersion == nil || *guardado.Titulares[0].DeclaracionVersion != 3 {
		t.Fatalf("resultado persistido = %+v, la linea tiene que llevar la version que la repartio", guardado.Titulares)
	}
	if len(declarada.Usos) != 1 || declarada.Usos[0].Escalon != "difuso" || declarada.Usos[0].Puntaje != "0.91" || declarada.Usos[0].ReporteID != "rep-1" {
		t.Fatalf("usos = %+v, se esperaba escalon y puntaje", declarada.Usos)
	}

	retenida := obras["obra-2"]
	if !retenida.Retenida || retenida.Motivo == "" || retenida.Declaracion != nil || len(retenida.Titulares) != 0 {
		t.Fatalf("obra-2 = %+v, se esperaba retenida por falta de declaracion (RD 13.1.3)", retenida)
	}
	if len(retenida.Usos) != 1 || retenida.Usos[0].ResueltoPor != "usr-admin" || retenida.Usos[0].ResueltoEn == "" || retenida.Usos[0].Puntaje != "" {
		t.Fatalf("usos = %+v, se esperaba quien resolvio y cuando", retenida.Usos)
	}
}

func TestValorizarSinOrigenDeUnUsoNoAsientaAMedias(t *testing.T) {
	t.Parallel()
	e, origen := entornoValorizacion(t)
	delete(origen.porUso, "u-2")

	_, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "usr-admin")
	if !errors.Is(err, ErrLinajeIncompleto) {
		t.Fatalf("error = %v, se esperaba ErrLinajeIncompleto", err)
	}
	if e.unidad.confirmo {
		t.Fatal("una valorizacion sin linaje completo no debio confirmarse")
	}
}

func TestUnUsoPorAliasNoAsientaPuntaje(t *testing.T) {
	t.Parallel()
	id := identificacionDe(OrigenDeUso{UsoID: "u", Escalon: "alias", Puntaje: puntaje("0")})
	if id.Puntaje != "" {
		t.Fatalf("puntaje = %q, un alias es exacto y no lleva puntaje", id.Puntaje)
	}
}

func TestValorizarAsientaElDesglosePorFactor(t *testing.T) {
	t.Parallel()
	e, _ := entornoValorizacion(t)

	if _, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "usr-admin"); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	var obra AsientoObraValorizada
	for _, a := range asientosDe(t, e.bitacora, HechoRepartoObraValorizada) {
		if a.RefID != "obra-1" {
			continue
		}
		if err := json.Unmarshal(a.Payload, &obra); err != nil {
			t.Fatalf("payload: %v", err)
		}
	}
	if len(obra.Valorizacion) != 1 {
		t.Fatalf("valorizacion = %+v, se esperaba un uso", obra.Valorizacion)
	}
	v := obra.Valorizacion[0]
	if v.UsoID != "u-1" || v.Formula != "RD 9.1.1" || len(v.Terminos) != 1 {
		t.Fatalf("valorizacion = %+v", v)
	}
	nombres := []string{"ponderacion", "duracion_min", "rating", "emisiones"}
	origenes := []string{"parametro", "uso", "uso", "uso"}
	valores := []string{"1.3", "48", "9", "10"}
	fs := v.Terminos[0].Factores
	if len(fs) != len(nombres) {
		t.Fatalf("factores = %+v", fs)
	}
	prod := decimal.NewFromInt(1)
	for i, f := range fs {
		if f.Nombre != nombres[i] || f.Origen != origenes[i] || !decimal.RequireFromString(f.Valor).Equal(decimal.RequireFromString(valores[i])) {
			t.Errorf("factor %d = %+v, se esperaba %s=%s (%s)", i, f, nombres[i], valores[i], origenes[i])
		}
		prod = prod.Mul(decimal.RequireFromString(f.Valor))
	}
	if !prod.Equal(decimal.RequireFromString(v.Terminos[0].Producto)) || !prod.Equal(decimal.RequireFromString(v.Puntos)) {
		t.Errorf("producto de factores %s != producto %s / puntos %s", prod, v.Terminos[0].Producto, v.Puntos)
	}
	if !decimal.RequireFromString(v.Puntos).Round(8).Equal(decimal.RequireFromString(obra.Puntos)) || obra.Puntos != "5616" {
		t.Errorf("puntos del uso %s no reproducen los de la obra %s", v.Puntos, obra.Puntos)
	}
}

func TestValorizarConUsosDelMotorDescuadradosNoAsienta(t *testing.T) {
	t.Parallel()

	_, err := asientosDeValorizacion(entradaValorizacion{
		usos:   []UsoDeReparto{usoDeCanal("z", reparto.TV, "")},
		motor:  nil,
		origen: map[string]OrigenDeUso{"u-1": {UsoID: "u-1", ReporteID: "rep-1"}},
	})
	if !errors.Is(err, ErrLinajeIncompleto) {
		t.Fatalf("error = %v, se esperaba ErrLinajeIncompleto", err)
	}
}
