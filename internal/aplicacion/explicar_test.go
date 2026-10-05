package aplicacion

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

// corridaAsentada es corridaSinAltas mas el alta asentada de cada obra: la cadena completa.
func corridaAsentada(t *testing.T) *bitacoraFalsa {
	t.Helper()
	b := corridaSinAltas(t)
	for _, o := range []struct{ id, titulo string }{{"obra-1", "La Primera"}, {"obra-2", "La Segunda"}} {
		b.asientos = append(b.asientos, Asiento{
			ID: "as-alta-" + o.id, Hecho: HechoObraRegistrada, RefTipo: RefObra, RefID: o.id,
			Payload: []byte(`{"despues":{"titulo":"` + o.titulo + `","genero":"drama","anio":2020}}`),
		})
	}
	return b
}

// corridaSinAltas corre proc-1 de verdad hasta firmar Verificacion; las obras no tienen alta asentada.
func corridaSinAltas(t *testing.T) *bitacoraFalsa {
	t.Helper()
	e, _ := entornoValorizacion(t)
	for range 4 {
		if _, err := e.uc.AvanzarEtapa(t.Context(), "proc-1", "usr-admin"); err != nil {
			t.Fatalf("avanzar: %v", err)
		}
	}
	if _, err := e.uc.Firmar(t.Context(), "proc-1", reparto.RolDistribucion, "usr-dist"); err != nil {
		t.Fatalf("firmar distribucion: %v", err)
	}
	if _, err := e.uc.Firmar(t.Context(), "proc-1", reparto.RolContabilidad, "usr-conta"); err != nil {
		t.Fatalf("firmar contabilidad: %v", err)
	}
	e.bitacora.asientos = append(e.bitacora.asientos, Asiento{
		ID: "as-recaudo", Hecho: HechoRecaudoRegistrado, RefTipo: RefBolsa, RefID: "bolsa-1",
		Payload: []byte(`{"usuario_id":"z","periodo":"2026-01","circuito":"nacional","bruto":"1000000.00","convenio":"conv-9","tarifa":"T-01","factura":"F-77"}`),
	})
	return e.bitacora
}

var (
	auditor    = Usuario{ID: "usr-aud", Rol: RolAuditor}
	titularUno = Usuario{ID: "usr-t1", Rol: RolTitular, TitularID: "titular-1"}
	otroTitu   = Usuario{ID: "usr-t9", Rol: RolTitular, TitularID: "titular-9"}
)

func TestExplicarUnaLineaDeTitularRespondeLasSietePreguntas(t *testing.T) {
	t.Parallel()
	uc := ExplicarCifra{Bitacora: corridaAsentada(t)}

	x, err := uc.Explicar(t.Context(), auditor, "proc-1:obra-1:titular-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if x.Ref != "proc-1:obra-1:titular-1" || x.TitularID != "titular-1" || x.Retenida {
		t.Fatalf("cabecera = %+v", x)
	}
	if x.Bolsa.ID != "bolsa-1" || x.Bolsa.UsuarioID != "z" || x.Bolsa.Recaudo == nil || x.Bolsa.Recaudo.Factura != "F-77" {
		t.Fatalf("1. bolsa = %+v", x.Bolsa)
	}
	if len(x.Reportes) != 1 || x.Reportes[0].SHA256 != "abc" || x.Reportes[0].ClaveObjeto != "crudos/abc" || x.Reporte.ID != "rep-1" {
		t.Fatalf("2. reportes = %+v / %+v", x.Reportes, x.Reporte)
	}
	if x.Obra.ID != "obra-1" || x.Obra.Escalon != "difuso" || x.Obra.Puntaje != "0.91" || len(x.Identificacion) != 1 {
		t.Fatalf("3. obra = %+v, identificacion = %+v", x.Obra, x.Identificacion)
	}
	if x.Regla.SnapshotID != "snap-1" || x.Regla.Reglamento != "RD-IX" {
		t.Fatalf("4. regla = %+v", x.Regla)
	}
	if x.Split == nil || x.Split.IPI != "IPI-1" || !x.Split.Porcentaje.Equal(d("100")) || x.Split.Version == nil || *x.Split.Version != 3 {
		t.Fatalf("5. split = %+v", x.Split)
	}
	if len(x.Deducciones) != 3 || x.Deducciones[0].Concepto != "gastos_administrativos" || !x.Deducciones[0].Porcentaje.Equal(d("20")) {
		t.Fatalf("6. deducciones = %+v", x.Deducciones)
	}
	suma := x.Neto
	for _, ded := range x.Deducciones {
		suma = suma.Add(ded.Monto)
	}
	if x.Neto.IsZero() || !suma.Equal(x.Bruto) {
		t.Fatalf("6. bruto %s != neto %s + deducciones", x.Bruto, x.Neto)
	}
	if len(x.Firmas) != 2 || x.Firmas[0].Rol != "distribucion" || x.Firmas[1].ActorID != "usr-conta" {
		t.Fatalf("7. firmas = %+v", x.Firmas)
	}
	if x.Corrida.ProcesoID != "proc-1" || x.Corrida.Periodo != "2026-01" || x.Corrida.Circuito != "nacional" {
		t.Fatalf("corrida = %+v", x.Corrida)
	}
	if x.Obra.Titulo != "La Primera" {
		t.Fatalf("obra.titulo = %q, sale del alta asentada", x.Obra.Titulo)
	}
	if len(x.Faltantes) != 0 {
		t.Fatalf("faltantes = %v, la cadena esta completa", x.Faltantes)
	}
}

func TestExplicarNombraElAltaDeObraQueFalta(t *testing.T) {
	t.Parallel()
	uc := ExplicarCifra{Bitacora: corridaSinAltas(t)}

	for _, ref := range []string{"proc-1:obra-1:titular-1", "proc-1:obra-2"} {
		x, err := uc.Explicar(t.Context(), auditor, ref)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		if x.Obra.Titulo != "" {
			t.Fatalf("%s: obra.titulo = %q sin alta asentada", ref, x.Obra.Titulo)
		}
		if len(x.Faltantes) != 1 || x.Faltantes[0] != HechoObraRegistrada {
			t.Fatalf("%s: faltantes = %v, tiene que nombrar %q", ref, x.Faltantes, HechoObraRegistrada)
		}
	}
}

func TestExplicarUnaObraRetenidaDiceElMotivo(t *testing.T) {
	t.Parallel()
	uc := ExplicarCifra{Bitacora: corridaAsentada(t)}

	x, err := uc.Explicar(t.Context(), auditor, "proc-1:obra-2")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !x.Retenida || x.Motivo == "" || x.Split != nil || x.TitularID != "" {
		t.Fatalf("retenida = %+v, se esperaba el motivo de RD 13.1.3", x)
	}
	if x.Identificacion[0].ResueltoPor != "usr-admin" {
		t.Fatalf("identificacion = %+v", x.Identificacion)
	}
}

func TestExplicarAlcanceDelTitular(t *testing.T) {
	t.Parallel()
	uc := ExplicarCifra{Bitacora: corridaAsentada(t)}

	if _, err := uc.Explicar(t.Context(), titularUno, "proc-1:obra-1:titular-1"); err != nil {
		t.Fatalf("el titular no pudo ver su propia cifra: %v", err)
	}
	if _, err := uc.Explicar(t.Context(), otroTitu, "proc-1:obra-1:titular-1"); !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("error = %v, la cifra de otro titular es 403", err)
	}
	if _, err := uc.Explicar(t.Context(), titularUno, "proc-1:obra-2"); !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("error = %v, un titular no ve la obra donde no participa", err)
	}
}

func TestExplicarSinLinajeEsNoEncontrado(t *testing.T) {
	t.Parallel()
	uc := ExplicarCifra{Bitacora: corridaAsentada(t)}

	for _, ref := range []string{
		"proc-x:obra-1:titular-1", // corrida sin asientos
		"proc-1:obra-x",           // obra que no valorizo en esa corrida
		"proc-1:obra-1:titular-x", // titular ajeno a la obra
		"sin-separador",
		"a:b:c:d",
		"proc-1::titular-1",
	} {
		if _, err := uc.Explicar(t.Context(), auditor, ref); !errors.Is(err, ErrNoEncontrado) {
			t.Errorf("%q: error = %v, se esperaba ErrNoEncontrado", ref, err)
		}
	}
}

func TestExplicarNombraElEslabonQueFalta(t *testing.T) {
	t.Parallel()
	b := corridaAsentada(t)
	var sinRecaudo []Asiento
	for _, a := range b.asientos {
		if a.Hecho != HechoRecaudoRegistrado {
			sinRecaudo = append(sinRecaudo, a)
		}
	}
	b.asientos = sinRecaudo

	x, err := ExplicarCifra{Bitacora: b}.Explicar(t.Context(), auditor, "proc-1:obra-1:titular-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if x.Bolsa.Recaudo != nil || len(x.Faltantes) != 1 || x.Faltantes[0] != HechoRecaudoRegistrado {
		t.Fatalf("faltantes = %v, se esperaba nombrar el recaudo ausente", x.Faltantes)
	}
}

func TestRefDeLineaYDeObra(t *testing.T) {
	t.Parallel()
	if got := FormarRef("proc-1", "obra-1", "titular-1"); got != "proc-1:obra-1:titular-1" {
		t.Fatalf("FormarRef = %q", got)
	}
	if got := FormarRef("proc-1", "obra-1", ""); got != "proc-1:obra-1" {
		t.Fatalf("FormarRef sin titular = %q", got)
	}
}

// Una correccion da el titulo pero no es el alta: faltantes sigue nombrando obra.registrada (B3).
func TestExplicarConCorreccionSinAltaNombraElAlta(t *testing.T) {
	t.Parallel()
	b := corridaSinAltas(t)
	b.asientos = append(b.asientos, Asiento{
		ID: "as-correccion-obra-1", Hecho: HechoObraCorregida, RefTipo: RefObra, RefID: "obra-1",
		Payload: []byte(`{"despues":{"titulo":"La Primera Corregida","genero":"drama","anio":2020}}`),
	})

	x, err := ExplicarCifra{Bitacora: b}.Explicar(t.Context(), auditor, "proc-1:obra-1:titular-1")
	if err != nil {
		t.Fatalf("explicar: %v", err)
	}
	if x.Obra.Titulo != "La Primera Corregida" {
		t.Fatalf("obra.titulo = %q, sale de la ultima correccion", x.Obra.Titulo)
	}
	if len(x.Faltantes) != 1 || x.Faltantes[0] != HechoObraRegistrada {
		t.Fatalf("faltantes = %v, sin alta asentada tiene que nombrar %q", x.Faltantes, HechoObraRegistrada)
	}
}

func TestBrutoYDeduccionesUsaElMismoVectorQueElExport(t *testing.T) {
	t.Parallel()
	// Tres titulares de 100 y admin 1.00: el mayor-resto le da 0.34 al primero.
	// Prorratear cada linea sola le daria 0.33 a las tres.
	c := AsientoValorizacion{
		ProcesoID: "proc-1",
		Neto:      "300.00",
		Deducciones: []DeduccionAsentada{
			{Concepto: liquidacion.ConceptoAdministracion, Porcentaje: "0", Monto: "1.00"},
			{Concepto: liquidacion.ConceptoSocial, Porcentaje: "0", Monto: "0.00"},
			{Concepto: liquidacion.ConceptoReserva, Porcentaje: "0", Monto: "0.00"},
		},
		Netos: []NetoTitularAsentado{
			{ObraID: "obra-a", TitularID: "t1", Importe: "100.00"},
			{ObraID: "obra-b", TitularID: "t2", Importe: "100.00"},
			{ObraID: "obra-c", TitularID: "t3", Importe: "100.00"},
		},
	}
	_, ded, err := brutoYDeducciones(decimal.RequireFromString("100.00"), c, "obra-a", "t1")
	if err != nil {
		t.Fatalf("prorratear: %v", err)
	}
	if len(ded) != 3 || !ded[0].Monto.Equal(decimal.RequireFromString("0.34")) {
		t.Fatalf("admin del primero = %+v, se esperaba 0.34 (el mismo centavo que el export)", ded)
	}
}

func TestSplitPrefiereLaVersionSelladaEnLaLinea(t *testing.T) {
	t.Parallel()
	deLaLinea := 4
	s, importe, err := splitDe(TitularAsentado{
		TitularID: "titular-1", IPI: "IPI-1", Porcentaje: "60", Importe: "10.00",
		DeclaracionVersion: &deLaLinea,
	}, &DeclaracionAsentada{Version: 3})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if s.Version == nil || *s.Version != 4 || !importe.Equal(d("10.00")) {
		t.Fatalf("split = %+v, se esperaba la v4 de la linea y no la v3 del asiento de la obra", s)
	}
}

func TestSplitDeUnAsientoViejoLeeLaVersionDeLaObra(t *testing.T) {
	t.Parallel()
	s, _, err := splitDe(TitularAsentado{
		TitularID: "titular-1", IPI: "IPI-1", Porcentaje: "60", Importe: "10.00",
	}, &DeclaracionAsentada{Version: 3})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if s.Version == nil || *s.Version != 3 {
		t.Fatalf("split = %+v, un asiento sin version por linea conserva la de la obra", s)
	}
}

func TestSplitSinVersionQuedaEnNil(t *testing.T) {
	t.Parallel()
	s, _, err := splitDe(TitularAsentado{
		TitularID: "titular-1", IPI: "IPI-1", Porcentaje: "60", Importe: "10.00",
	}, nil)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if s.Version != nil {
		t.Fatalf("version = %d, no se inventa una version que la corrida no guardo", *s.Version)
	}
}

func TestBrutoYDeduccionesCifraCeroNoInventaPorcentajes(t *testing.T) {
	t.Parallel()
	c := AsientoValorizacion{
		Neto: "300.00",
		Deducciones: []DeduccionAsentada{
			{Concepto: liquidacion.ConceptoAdministracion, Porcentaje: "20", Monto: "1.00"},
		},
	}
	bruto, ded, err := brutoYDeducciones(decimal.Zero, c, "obra-a", "t1")
	if err != nil {
		t.Fatalf("prorratear: %v", err)
	}
	if !bruto.IsZero() || len(ded) != 0 {
		t.Fatalf("bruto=%s deducciones=%v, una cifra en cero no lleva porcentajes", bruto, ded)
	}
}

func TestExplicarTraeElDesgloseYLosPuntosDeLaObra(t *testing.T) {
	t.Parallel()
	b := corridaAsentada(t)

	x, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), auditor, "proc-1:obra-1:titular-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(x.Valorizacion) != 1 || x.Valorizacion[0].Puntos != "5616" {
		t.Fatalf("valorizacion = %+v", x.Valorizacion)
	}
	if x.Obra.Puntos != "5616" || x.Obra.Puntaje != "0.91" {
		t.Fatalf("obra = %+v, los puntos de reparto no son el puntaje del matching", x.Obra)
	}
}

func TestExplicarUnAsientoAnteriorA187SigueExplicandose(t *testing.T) {
	t.Parallel()
	b := corridaAsentada(t)
	b.asientos = append(b.asientos, Asiento{
		ID: "as-viejo", Hecho: HechoRepartoObraValorizada, RefTipo: RefObra, RefID: "obra-1",
		Payload: []byte(`{"proceso_id":"proc-1","periodo":"2026-01","puntos":"5616","importe":"280000.00","retenida":false,"declaracion":{"version":3,"vigente_desde":"2025-12-01"},"titulares":[{"titular_id":"titular-1","ipi":"IPI-1","porcentaje":"100","importe":"280000.00"}],"usos":[{"uso_id":"u-1","reporte_id":"rep-1","escalon":"difuso","puntaje":"0.91"}]}`),
	})

	x, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), auditor, "proc-1:obra-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if x.Valorizacion == nil || len(x.Valorizacion) != 0 {
		t.Fatalf("valorizacion = %#v, se esperaba lista vacia no nil", x.Valorizacion)
	}
	if x.Obra.Puntos != "5616" || !x.Neto.Equal(d("280000")) {
		t.Fatalf("obra.puntos = %q, neto = %s", x.Obra.Puntos, x.Neto)
	}
}

// Una obra valorizada antes de #187 en proc-1 y despues en otra corrida: explicar
// proc-1 no puede heredar el desglose del asiento descartado de proc-2.
func TestExplicarUnAsientoViejoNoHeredaElDesgloseDeOtraCorrida(t *testing.T) {
	t.Parallel()
	b := corridaAsentada(t)
	asientos := b.asientos[:0:0]
	for _, a := range b.asientos {
		if a.Hecho == HechoRepartoObraValorizada && a.RefID == "obra-1" {
			continue
		}
		asientos = append(asientos, a)
	}
	b.asientos = append(asientos,
		Asiento{
			ID: "as-viejo", Hecho: HechoRepartoObraValorizada, RefTipo: RefObra, RefID: "obra-1",
			Payload: []byte(`{"proceso_id":"proc-1","periodo":"2026-01","puntos":"5616","importe":"280000.00","retenida":false,"declaracion":{"version":3,"vigente_desde":"2025-12-01"},"titulares":[{"titular_id":"titular-1","ipi":"IPI-1","porcentaje":"100","importe":"280000.00"}],"usos":[{"uso_id":"u-1","reporte_id":"rep-1","escalon":"difuso","puntaje":"0.91"}]}`),
		},
		Asiento{
			ID: "as-nuevo", Hecho: HechoRepartoObraValorizada, RefTipo: RefObra, RefID: "obra-1",
			Payload: []byte(`{"proceso_id":"proc-2","periodo":"2026-02","puntos":"9999","importe":"1.00","retenida":false,"titulares":[],"usos":[],"valorizacion":[{"uso_id":"u-9","formula":"RD 9.1.1","puntos":"9999","terminos":[{"factores":[{"nombre":"emisiones","valor":"9999","origen":"uso"}],"producto":"9999"}]}]}`),
		},
	)

	x, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), auditor, "proc-1:obra-1")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if x.Valorizacion == nil || len(x.Valorizacion) != 0 {
		t.Fatalf("valorizacion = %#v, se esperaba lista vacia: es de otra corrida", x.Valorizacion)
	}
	if x.Obra.Puntos != "5616" {
		t.Fatalf("obra.puntos = %q, se esperaba el de proc-1", x.Obra.Puntos)
	}
}

func asientoDeLiberacion(importe string) Asiento {
	payload := fmt.Sprintf(`{"origen":{"proceso_id":"p1","periodo":"2026-01","circuito":"nacional"},"destino":{"proceso_id":"p2","periodo":"2027-01","circuito":"nacional"},"vigencia_rendimiento":"2026","lineas":[{"obra_id":"obra-1","titular_id":"titular-a","ipi":"111","porcentaje":"40","importe":"%s"}]}`, importe)
	return Asiento{
		Hecho: HechoReservaLiberada, RefTipo: RefProceso, RefID: "p2",
		ActorID: "actor-1", Payload: []byte(payload),
	}
}

func TestExplicarUnaCifraLiberadaNombraOrigenYDestino(t *testing.T) {
	t.Parallel()
	b := &bitacoraFalsa{asientos: []Asiento{asientoDeLiberacion("20.00")}}
	ref := FormarRefReservaLiberada("p1", "p2", "obra-1", "titular-a")

	x, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), auditor, ref)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if x.Origen == nil || x.Destino == nil {
		t.Fatal("la cifra liberada tiene que nombrar las dos corridas")
	}
	if x.Origen.ProcesoID != "p1" || x.Origen.Periodo != "2026-01" || x.Origen.Circuito != "nacional" {
		t.Fatalf("origen = %+v", x.Origen)
	}
	if x.Destino.ProcesoID != "p2" || x.Destino.Periodo != "2027-01" || x.Corrida.ProcesoID != "p2" {
		t.Fatalf("destino = %+v, corrida = %+v", x.Destino, x.Corrida)
	}
	if !x.Neto.Equal(decimal.RequireFromString("20.00")) || !x.Bruto.Equal(x.Neto) {
		t.Fatalf("neto/bruto = %s/%s", x.Neto, x.Bruto)
	}
	if x.TitularID != "titular-a" || x.Obra.ID != "obra-1" || x.Split == nil || x.Split.IPI != "111" {
		t.Fatalf("linea = titular %q obra %q split %+v", x.TitularID, x.Obra.ID, x.Split)
	}
}

func TestExplicarUnaCifraLiberadaAjenaEsNoAutorizado(t *testing.T) {
	t.Parallel()
	b := &bitacoraFalsa{asientos: []Asiento{asientoDeLiberacion("20.00")}}
	ref := FormarRefReservaLiberada("p1", "p2", "obra-1", "titular-a")
	ajeno := Usuario{ID: "usr-z", Rol: RolTitular, TitularID: "titular-z"}

	if _, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), ajeno, ref); !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("error = %v, se esperaba ErrNoAutorizado", err)
	}
}

func TestExplicarUnaRedistribucionEnCeroNoEscondeElPago(t *testing.T) {
	t.Parallel()
	b := &bitacoraFalsa{asientos: []Asiento{asientoDeLiberacion("20.00"), asientoDeLiberacion("0.00")}}
	ref := FormarRefReservaLiberada("p1", "p2", "obra-1", "titular-a")

	x, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), auditor, ref)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if !x.Neto.Equal(decimal.RequireFromString("20.00")) {
		t.Fatalf("neto = %s, el asiento en cero no debio tapar el pago", x.Neto)
	}
}

func TestExplicarUnRendimientoNombraOrigenYDestino(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"origen":{"proceso_id":"p1","periodo":"2026-01","circuito":"nacional"},"destino":{"proceso_id":"p2","periodo":"2027-01","circuito":"internacional"},"circuito":"nacional","vigencia":"2026","lineas":[{"obra_id":"obra-1","titular_id":"titular-a","ipi":"111","porcentaje":"40","importe":"40.00"}]}`)
	b := &bitacoraFalsa{asientos: []Asiento{{
		Hecho: HechoRendimientosDistribuidos, RefTipo: RefProceso, RefID: "p2",
		Payload: payload,
	}}}
	ref := FormarRefRendimiento("p1", "p2", "obra-1", "titular-a")

	x, err := (ExplicarCifra{Bitacora: b}).Explicar(t.Context(), auditor, ref)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if x.Origen == nil || x.Destino == nil || x.Origen.ProcesoID != "p1" || x.Destino.Circuito != "internacional" {
		t.Fatalf("origen/destino = %+v / %+v", x.Origen, x.Destino)
	}
	if !x.Neto.Equal(decimal.RequireFromString("40.00")) {
		t.Fatalf("neto = %s", x.Neto)
	}
}
