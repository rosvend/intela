package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// ---------------------------------------------------------------------------
// Dobles

// entregasFalsas sirve los dos metodos de lectura que la evaluacion necesita,
// y APUNTA con que periodo se le pidieron las cargas: que se pidan TODAS -y no
// solo las del periodo- es la mitad del detector de duplicado por huella, y sin
// comprobarlo ese filtro se puede colar sin que nada falle.
type entregasFalsas struct {
	usos   []UsoPersistido
	cargas []CargaReporte

	periodoDeUsos   string
	periodoDeCargas string
	llamadasCargas  int

	errUsos   error
	errCargas error
}

func (e *entregasFalsas) UsosDePeriodo(_ context.Context, periodo string) ([]UsoPersistido, error) {
	e.periodoDeUsos = periodo
	return e.usos, e.errUsos
}

func (e *entregasFalsas) ListarCargas(_ context.Context, periodo string) ([]CargaReporte, error) {
	e.periodoDeCargas = periodo
	e.llamadasCargas++
	return e.cargas, e.errCargas
}

// coautoresFalsos apunta con cuantos ids se le llamo: con N obras tiene que
// ser UNA sola llamada, que es la razon de ser de LectorDeCoautores.
type coautoresFalsos struct {
	porObra    map[string][]repertorio.Coautor
	idsPedidos []string
	llamadas   int
	err        error
}

func (c *coautoresFalsos) CoautoresDeObras(
	_ context.Context, obraIDs []string,
) (map[string][]repertorio.Coautor, error) {
	c.llamadas++
	c.idsPedidos = append([]string(nil), obraIDs...)
	return c.porObra, c.err
}

// vigentesFalsas satisface LectorDeDeclaraciones. Una obra ausente del mapa es
// una obra sin declaracion, que es el contrato de VigentesDeObras.
type vigentesFalsas struct {
	porObra map[string]VersionDeclaracion
	err     error
}

func (v *vigentesFalsas) VigentesDeObras(
	_ context.Context, _ []string,
) (map[string]VersionDeclaracion, error) {
	return v.porObra, v.err
}

// alertasFalsas es una bandeja en memoria CON la clave natural aplicada: sin
// ella, el doble aceptaria dos veces el mismo hallazgo y la prueba de
// idempotencia pasaria sin probar nada.
type alertasFalsas struct {
	filas     []Alerta
	guardados int
	err       error
	errGuarda error
}

func claveNatural(a Alerta) string {
	return strings.Join([]string{a.Periodo, a.Tipo, a.RefTipo, a.RefID, a.RefTitular}, "\x00")
}

func (f *alertasFalsas) GuardarAlertas(_ context.Context, alertas []Alerta) (int, error) {
	f.guardados++
	if f.errGuarda != nil {
		return 0, f.errGuarda
	}
	vistas := map[string]bool{}
	for _, ya := range f.filas {
		vistas[claveNatural(ya)] = true
	}
	nuevas := 0
	for _, a := range alertas {
		if vistas[claveNatural(a)] {
			continue
		}
		vistas[claveNatural(a)] = true
		a.ID = fmt.Sprintf("al-%d", len(f.filas)+1)
		f.filas = append(f.filas, a)
		nuevas++
	}
	return nuevas, nil
}

func (f *alertasFalsas) ListarAlertas(_ context.Context, filtro FiltroAlertas) ([]Alerta, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]Alerta, 0, len(f.filas))
	for _, a := range f.filas {
		if filtro.Periodo != "" && a.Periodo != filtro.Periodo {
			continue
		}
		if filtro.Tipo != "" && a.Tipo != filtro.Tipo {
			continue
		}
		if filtro.Resueltas != nil && a.Resuelta != *filtro.Resueltas {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (f *alertasFalsas) ResolverAlerta(
	_ context.Context, id, actorID, nota string, cuando time.Time,
) (Alerta, error) {
	if f.err != nil {
		return Alerta{}, f.err
	}
	for i := range f.filas {
		if f.filas[i].ID != id {
			continue
		}
		if f.filas[i].Resuelta {
			return Alerta{}, ErrAlertaYaResuelta
		}
		f.filas[i].Resuelta = true
		f.filas[i].ResueltaPor = actorID
		f.filas[i].ResueltaEn = &cuando
		f.filas[i].Nota = nota
		return f.filas[i], nil
	}
	return Alerta{}, ErrNoEncontrado
}

func (f *alertasFalsas) ContarAlertasSinResolver(
	_ context.Context, periodo string, tipos []string,
) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	n := 0
	for _, a := range f.filas {
		if a.Periodo != periodo || a.Resuelta {
			continue
		}
		if len(tipos) > 0 && !slices.Contains(tipos, a.Tipo) {
			continue
		}
		n++
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Armado

const periodoDePrueba = "2025-01"

var instanteAnomalias = time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)

// periodoSembrado devuelve un caso con una anomalia de cada tipo, montado
// sobre los MODELOS DE LA CAPA DE APLICACION -- no sobre los del dominio --
// para que la traduccion que hace armarPeriodo entre en la prueba.
func periodoSembrado() (*entregasFalsas, *coautoresFalsos, *vigentesFalsas) {
	entregas := &entregasFalsas{
		usos: []UsoPersistido{
			// ONI.
			{ID: "u-oni", ReporteID: "r1", Fuente: "caracol", Titulo: "Sin casar", Escalon: "oni"},
			// Duplicado de registro: misma clave logica en dos entregas.
			// ids_fuente va en el formato del contrato (ADR 0018).
			{ID: "u-dup1", ReporteID: "r1", Fuente: "caracol", ObraID: "o-ok", Escalon: "alias",
				TipoObra: "serie", IDsFuente: "id_ficha=7", Fecha: "2025-01-02", Hora: "20:00:00"},
			{ID: "u-dup2", ReporteID: "r2", Fuente: "caracol", ObraID: "o-ok", Escalon: "alias",
				TipoObra: "serie", IDsFuente: "id_ficha=7", Fecha: "2025-01-02", Hora: "20:00:00"},
			// tipo_obra sin mapear.
			{ID: "u-sintipo", ReporteID: "r1", Fuente: "caracol", ObraID: "o-ok", Escalon: "alias",
				IDsFuente: "id_ficha=8", Fecha: "2025-01-03", Hora: "21:00:00"},
			// La obra con declaracion incompleta entra por su uso.
			{ID: "u-inc", ReporteID: "r1", Fuente: "caracol", ObraID: "o-inc", Escalon: "alias",
				TipoObra: "unitario", IDsFuente: "id_ficha=9", Fecha: "2025-01-04", Hora: "19:00:00"},
		},
		cargas: []CargaReporte{
			{Reporte: Reporte{ID: "r1", Fuente: "caracol", Periodo: periodoDePrueba, SHA256: "aa"}},
			{Reporte: Reporte{ID: "r2", Fuente: "caracol", Periodo: periodoDePrueba, SHA256: "bb"}},
			// La otra pata de la colision vive en OTRO periodo y bajo otra
			// fuente: es justo lo que el UNIQUE (sha256, fuente) deja pasar.
			{Reporte: Reporte{ID: "r3", Fuente: "netflix", Periodo: "2024-12", SHA256: "aa"}},
		},
	}
	coautores := &coautoresFalsos{porObra: map[string][]repertorio.Coautor{
		"o-inc": {
			{IPI: "ipi-a", Nombre: "Ana", Rol: repertorio.RolGuionista},
			{IPI: "ipi-b", Nombre: "Beto", Rol: repertorio.RolLibretista},
		},
		"o-ok": {{IPI: "ipi-c", Nombre: "Cris", Rol: repertorio.RolGuionista}},
	}}
	vigentes := &vigentesFalsas{porObra: map[string]VersionDeclaracion{
		"o-inc": {Version: 1, Declaracion: repertorio.Declaracion{
			ObraID: "o-inc",
			Partes: []repertorio.Parte{{TitularID: "tit-a", IPI: "ipi-a", Porcentaje: decimal.NewFromInt(60)}},
		}},
		"o-ok": {Version: 1, Declaracion: repertorio.Declaracion{
			ObraID: "o-ok",
			Partes: []repertorio.Parte{{TitularID: "tit-c", IPI: "ipi-c", Porcentaje: decimal.NewFromInt(100)}},
		}},
	}}
	return entregas, coautores, vigentes
}

func servicioSembrado() (Anomalias, *entregasFalsas, *alertasFalsas, *bitacoraFalsa, *unidadFalsa) {
	entregas, coautores, vigentes := periodoSembrado()
	alertas := &alertasFalsas{}
	bitacora := &bitacoraFalsa{}
	unidad := &unidadFalsa{}
	return Anomalias{
		Entregas:      entregas,
		Declaraciones: vigentes,
		Coautores:     coautores,
		Alertas:       alertas,
		Bitacora:      bitacora,
		Unidad:        unidad,
		Reloj:         relojFijo{instante: instanteAnomalias},
	}, entregas, alertas, bitacora, unidad
}

func tiposDe(alertas []Alerta) []string {
	out := make([]string, 0, len(alertas))
	for _, a := range alertas {
		out = append(out, a.Tipo)
	}
	slices.Sort(out)
	return out
}

// ---------------------------------------------------------------------------
// Evaluar

func TestEvaluarLevantaUnaAnomaliaDeCadaTipo(t *testing.T) {
	svc, _, alertas, _, _ := servicioSembrado()

	resumen, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1")
	if err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	if resumen.Detectadas != 6 || resumen.Nuevas != 6 {
		t.Fatalf("resumen = %d detectadas / %d nuevas, se esperaban 6 y 6", resumen.Detectadas, resumen.Nuevas)
	}

	quiero := []string{
		anomalias.TipoDuplicadoArchivo,
		anomalias.TipoDuplicadoRegistro,
		anomalias.TipoONI,
		anomalias.TipoReservaDeclaracionIncompleta,
		anomalias.TipoTipoObraSinMapear,
		anomalias.TipoTitularSinPorcentaje,
	}
	if got := tiposDe(alertas.filas); !slices.Equal(got, quiero) {
		t.Fatalf("tipos guardados = %v, se esperaba %v", got, quiero)
	}

	// PorTipo lleva los SEIS tipos aunque alguno vaya a cero: el tablero pinta
	// una tarjeta por tipo y una clave ausente no se distingue de un cero al
	// otro lado del JSON.
	if len(resumen.PorTipo) != len(anomalias.Tipos()) {
		t.Fatalf("PorTipo = %v, se esperaban %d claves", resumen.PorTipo, len(anomalias.Tipos()))
	}
}

// Cada alerta lleva su tipo y una referencia al registro EXACTO que la
// disparo: es criterio de aceptacion del issue.
func TestEvaluarReferenciaElRegistroExacto(t *testing.T) {
	svc, _, alertas, _, _ := servicioSembrado()

	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	quiero := map[string]string{
		anomalias.TipoONI:                          "uso:u-oni",
		anomalias.TipoDuplicadoRegistro:            "uso:u-dup2",
		anomalias.TipoTipoObraSinMapear:            "uso:u-sintipo",
		anomalias.TipoDuplicadoArchivo:             "reporte:r1",
		anomalias.TipoReservaDeclaracionIncompleta: "obra:o-inc",
		anomalias.TipoTitularSinPorcentaje:         "obra:o-inc",
	}
	for _, a := range alertas.filas {
		ref := a.RefTipo + ":" + a.RefID
		if ref != quiero[a.Tipo] {
			t.Fatalf("la alerta %q referencia %q, se esperaba %q", a.Tipo, ref, quiero[a.Tipo])
		}
		if a.Periodo != periodoDePrueba {
			t.Fatalf("la alerta %q quedo en el periodo %q", a.Tipo, a.Periodo)
		}
		if a.Detectada != instanteAnomalias {
			t.Fatalf("la alerta %q se detecto en %v y el reloj marca %v", a.Tipo, a.Detectada, instanteAnomalias)
		}
		if a.Detalle == "" {
			t.Fatalf("la alerta %q no trae detalle", a.Tipo)
		}
	}

	// La segunda coordenada nombra a la PERSONA a la que le falta declarar.
	for _, a := range alertas.filas {
		if a.Tipo != anomalias.TipoTitularSinPorcentaje {
			continue
		}
		if a.RefTitular != "ipi-b" {
			t.Fatalf("titular_sin_porcentaje apunta a %q, se esperaba ipi-b", a.RefTitular)
		}
	}
}

// El harness de pruebas NUNCA ejercita filas preexistentes por su cuenta: la
// plantilla se toma con las tablas vacias. La idempotencia hay que provocarla
// llamando dos veces, o el verde no prueba nada.
func TestEvaluarDosVecesNoDuplicaAlertas(t *testing.T) {
	svc, _, alertas, bitacora, _ := servicioSembrado()

	primera, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1")
	if err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	segunda, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1")
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}

	if len(alertas.filas) != primera.Nuevas {
		t.Fatalf("tras dos pasadas hay %d filas, se esperaban %d", len(alertas.filas), primera.Nuevas)
	}
	// Detectadas NO baja: el periodo sigue teniendo seis anomalias. Lo que
	// baja es Nuevas. Con una sola cifra, la segunda pasada diria "0" y se
	// leeria como "el periodo esta limpio".
	if segunda.Detectadas != primera.Detectadas {
		t.Fatalf("la segunda pasada detecto %d y la primera %d", segunda.Detectadas, primera.Detectadas)
	}
	if segunda.Nuevas != 0 {
		t.Fatalf("la segunda pasada declaro %d nuevas, se esperaban 0", segunda.Nuevas)
	}
	// Cada pasada deja su asiento aunque no escriba ninguna alerta: lo que se
	// registra es que alguien MIRO el periodo y cuando.
	if len(bitacora.asientos) != 2 {
		t.Fatalf("hay %d asientos tras dos pasadas, se esperaban 2", len(bitacora.asientos))
	}
}

func TestEvaluarAsientaLaPasadaEnLaMismaUnidad(t *testing.T) {
	svc, _, _, bitacora, unidad := servicioSembrado()

	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	if unidad.entradas != 1 || !unidad.confirmo {
		t.Fatalf("unidad: %d entradas, confirmo=%v", unidad.entradas, unidad.confirmo)
	}
	if len(bitacora.asientos) != 1 {
		t.Fatalf("se esperaba un asiento, hubo %d", len(bitacora.asientos))
	}

	a := bitacora.asientos[0]
	if a.Hecho != HechoAnomaliasEvaluadas || a.RefTipo != RefPeriodo || a.RefID != periodoDePrueba {
		t.Fatalf("asiento = %q sobre %s %q", a.Hecho, a.RefTipo, a.RefID)
	}
	if a.ActorID != "usr-1" || a.Cuando != instanteAnomalias {
		t.Fatalf("asiento firmado por %q en %v", a.ActorID, a.Cuando)
	}

	var payload struct {
		Detectadas int            `json:"detectadas"`
		Nuevas     int            `json:"nuevas"`
		PorTipo    map[string]int `json:"por_tipo"`
	}
	if err := json.Unmarshal(a.Payload, &payload); err != nil {
		t.Fatalf("payload del asiento: %v", err)
	}
	if payload.Detectadas != 6 || payload.Nuevas != 6 {
		t.Fatalf("payload = %d detectadas / %d nuevas", payload.Detectadas, payload.Nuevas)
	}
}

// ADR 0006: un caso de uso cuyo asiento fallo no esta hecho. El error tiene
// que llegar hasta el limite de la unidad -- que es lo que en el adaptador
// dispara el rollback -- y no tragarse.
func TestEvaluarFallaSiElAsientoFalla(t *testing.T) {
	svc, _, _, bitacora, unidad := servicioSembrado()
	bitacora.err = errors.New("bitacora caida")

	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err == nil {
		t.Fatal("Evaluar no fallo con la bitacora caida")
	}
	if unidad.confirmo {
		t.Fatal("la unidad se confirmo aunque el asiento fallo")
	}
}

// Las entregas se piden TODAS y los usos solo los del periodo. Si alguien
// "optimiza" pidiendo solo las cargas del periodo, el detector de duplicado
// por huella se queda ciego a la mitad de los casos y nada mas falla.
func TestEvaluarPideTodasLasEntregasYSoloLosUsosDelPeriodo(t *testing.T) {
	svc, entregas, alertas, _, _ := servicioSembrado()

	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	if entregas.periodoDeUsos != periodoDePrueba {
		t.Fatalf("los usos se pidieron del periodo %q", entregas.periodoDeUsos)
	}
	if entregas.periodoDeCargas != "" {
		t.Fatalf("las cargas se pidieron filtradas por %q; tienen que pedirse todas", entregas.periodoDeCargas)
	}

	// Y la colision que eso caza es la de r1 (2025-01, caracol) con r3
	// (2024-12, netflix): misma huella, otra fuente, otro mes.
	hayDuplicadoDeArchivo := slices.ContainsFunc(alertas.filas, func(a Alerta) bool {
		return a.Tipo == anomalias.TipoDuplicadoArchivo && a.RefID == "r1"
	})
	if !hayDuplicadoDeArchivo {
		t.Fatal("no se detecto la colision de huella que cruza periodos")
	}
}

// Los coautores de las N obras del periodo se piden en UNA llamada. Es la
// razon de ser de LectorDeCoautores, y sin comprobarlo el N+1 vuelve solo.
func TestEvaluarPideLosCoautoresEnUnaSolaConsulta(t *testing.T) {
	svc, _, _, _, _ := servicioSembrado()
	coautores := svc.Coautores.(*coautoresFalsos)

	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	if coautores.llamadas != 1 {
		t.Fatalf("se llamo %d veces a CoautoresDeObras", coautores.llamadas)
	}
	// Ordenados y sin repetir: los usos vienen por id de uso, no por obra, y
	// el mismo id repetido pediria la misma obra tres veces.
	if quiero := []string{"o-inc", "o-ok"}; !slices.Equal(coautores.idsPedidos, quiero) {
		t.Fatalf("ids pedidos = %v, se esperaba %v", coautores.idsPedidos, quiero)
	}
}

// Un periodo mal formado se rechaza ANTES de tocar la base. Ignorarlo o
// devolver vacio haria pasar `2025-13` por "ese mes no tuvo anomalias".
func TestEvaluarRechazaUnPeriodoMalFormado(t *testing.T) {
	for _, periodo := range []string{"", "enero", "2025-13", "2025-00", "25-01"} {
		t.Run(periodo, func(t *testing.T) {
			svc, entregas, _, _, _ := servicioSembrado()
			if _, err := svc.Evaluar(t.Context(), periodo, "usr-1"); !errors.Is(err, recaudo.ErrBolsaInvalida) {
				t.Fatalf("Evaluar(%q) = %v, se esperaba ErrBolsaInvalida", periodo, err)
			}
			if entregas.llamadasCargas != 0 {
				t.Fatal("se consulto la base con un periodo invalido")
			}
		})
	}
}

// El actor puede venir vacio: una pasada la puede disparar el pipeline sin que
// haya nadie delante, y el ADR 0006 pide la firma para la DECISION MANUAL. Lo
// que NO puede es venir vacio al resolver (ver mas abajo).
func TestEvaluarAdmiteUnaPasadaSinActor(t *testing.T) {
	svc, _, _, bitacora, _ := servicioSembrado()

	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, ""); err != nil {
		t.Fatalf("Evaluar sin actor: %v", err)
	}
	if bitacora.asientos[0].ActorID != "" {
		t.Fatalf("el asiento quedo firmado por %q", bitacora.asientos[0].ActorID)
	}
}

// ---------------------------------------------------------------------------
// Listar

func TestListarMarcaLasCriticas(t *testing.T) {
	svc, _, _, _, _ := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	alertas, err := svc.Listar(t.Context(), FiltroAlertas{Periodo: periodoDePrueba})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	for _, a := range alertas {
		if a.Critica != anomalias.EsCritica(a.Tipo) {
			t.Fatalf("la alerta %q salio con critica=%v", a.Tipo, a.Critica)
		}
	}
}

func TestListarRechazaFiltrosInvalidos(t *testing.T) {
	svc, _, _, _, _ := servicioSembrado()

	if _, err := svc.Listar(t.Context(), FiltroAlertas{Periodo: "2025-13"}); !errors.Is(err, recaudo.ErrBolsaInvalida) {
		t.Fatalf("un periodo imposible dio %v", err)
	}
	// Una errata de grafia devolviendo lista vacia se lee como "no hay
	// ninguna de ese tipo", que es una afirmacion falsa.
	if _, err := svc.Listar(t.Context(), FiltroAlertas{Tipo: "onni"}); !errors.Is(err, ErrFiltroInvalido) {
		t.Fatalf("un tipo inventado dio %v", err)
	}
}

func TestListarSinPeriodoNoFiltra(t *testing.T) {
	svc, _, _, _, _ := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	alertas, err := svc.Listar(t.Context(), FiltroAlertas{})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(alertas) != 6 {
		t.Fatalf("sin filtro salieron %d alertas, se esperaban 6", len(alertas))
	}
}

// ---------------------------------------------------------------------------
// Resolver

func TestResolverFirmaYAsienta(t *testing.T) {
	svc, _, alertas, bitacora, unidad := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	id := alertas.filas[0].ID

	resuelta, err := svc.Resolver(t.Context(), id, "usr-2", "asignada a mano")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if !resuelta.Resuelta || resuelta.ResueltaPor != "usr-2" || resuelta.Nota != "asignada a mano" {
		t.Fatalf("resuelta = %+v", resuelta)
	}
	if resuelta.ResueltaEn == nil || *resuelta.ResueltaEn != instanteAnomalias {
		t.Fatalf("resuelta_en = %v, el reloj marca %v", resuelta.ResueltaEn, instanteAnomalias)
	}

	// Dos entradas a la unidad: la de Evaluar y la de Resolver.
	if unidad.entradas != 2 {
		t.Fatalf("la unidad se abrio %d veces", unidad.entradas)
	}
	ultimo := bitacora.asientos[len(bitacora.asientos)-1]
	if ultimo.Hecho != HechoAlertaResuelta || ultimo.RefTipo != RefAlerta || ultimo.RefID != id {
		t.Fatalf("asiento = %q sobre %s %q", ultimo.Hecho, ultimo.RefTipo, ultimo.RefID)
	}
	if ultimo.ActorID != "usr-2" {
		t.Fatalf("el asiento lo firma %q", ultimo.ActorID)
	}
	// El payload conserva el estado ANTERIOR: `alertas` se sobreescribe, asi
	// que si no queda ahi no queda en ningun sitio.
	if !strings.Contains(string(ultimo.Payload), "\"ref_id\"") {
		t.Fatalf("el payload no conserva la referencia: %s", ultimo.Payload)
	}
}

// El actor se exige ANTES de escribir. Un actorID vacio dejaria un asiento sin
// firmar -- valido para la base, invalido para el ADR 0006 -- y el UPDATE ya
// estaria hecho cuando eso se notara.
func TestResolverExigeActorAntesDeEscribir(t *testing.T) {
	svc, _, alertas, _, unidad := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	entradasAntes := unidad.entradas
	id := alertas.filas[0].ID

	for _, actor := range []string{"", "   "} {
		if _, err := svc.Resolver(t.Context(), id, actor, ""); !errors.Is(err, ErrActorAusente) {
			t.Fatalf("Resolver con actor %q dio %v", actor, err)
		}
	}
	if unidad.entradas != entradasAntes {
		t.Fatal("se abrio una transaccion pese a faltar el actor")
	}
	if alertas.filas[0].Resuelta {
		t.Fatal("la alerta quedo resuelta sin firma")
	}
}

func TestResolverDistingueNoExisteDeYaResuelta(t *testing.T) {
	svc, _, alertas, _, _ := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	id := alertas.filas[0].ID

	if _, err := svc.Resolver(t.Context(), "no-existe", "usr-2", ""); !errors.Is(err, ErrNoEncontrado) {
		t.Fatalf("un id inventado dio %v", err)
	}
	if _, err := svc.Resolver(t.Context(), id, "usr-2", ""); err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	// Dos personas mirando el mismo tablero es el caso normal: quien llega
	// segundo tiene que saber que la firma escrita no es la suya.
	if _, err := svc.Resolver(t.Context(), id, "usr-3", ""); !errors.Is(err, ErrAlertaYaResuelta) {
		t.Fatalf("la segunda resolucion dio %v", err)
	}
}

// ---------------------------------------------------------------------------
// La compuerta de #34

func TestCriticasAbiertasSoloCuentaLosTiposQueBloquean(t *testing.T) {
	svc, _, alertas, _, _ := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	// De las seis, bloquean tres: duplicado de archivo, duplicado de registro
	// y tipo_obra sin mapear. ONI y la retencion de R-04 son estados validos
	// del modelo.
	n, err := svc.CriticasAbiertas(t.Context(), periodoDePrueba)
	if err != nil {
		t.Fatalf("CriticasAbiertas: %v", err)
	}
	if n != 3 {
		t.Fatalf("CriticasAbiertas = %d, se esperaban 3", n)
	}

	// Resolver una critica baja el contador; resolver una informativa no.
	var critica, informativa string
	for _, a := range alertas.filas {
		if anomalias.EsCritica(a.Tipo) && critica == "" {
			critica = a.ID
		}
		if !anomalias.EsCritica(a.Tipo) && informativa == "" {
			informativa = a.ID
		}
	}
	if _, err := svc.Resolver(t.Context(), informativa, "usr-2", ""); err != nil {
		t.Fatalf("Resolver informativa: %v", err)
	}
	if n, _ := svc.CriticasAbiertas(t.Context(), periodoDePrueba); n != 3 {
		t.Fatalf("resolver una informativa dejo %d criticas, se esperaban 3", n)
	}
	if _, err := svc.Resolver(t.Context(), critica, "usr-2", ""); err != nil {
		t.Fatalf("Resolver critica: %v", err)
	}
	if n, _ := svc.CriticasAbiertas(t.Context(), periodoDePrueba); n != 2 {
		t.Fatalf("resolver una critica dejo %d, se esperaban 2", n)
	}
}

// ---------------------------------------------------------------------------
// Cableado

func TestUnServicioMalCableadoFallaAntesDeTocarLaBase(t *testing.T) {
	casos := map[string]Anomalias{
		"sin RepositorioAlertas": {Unidad: &unidadFalsa{}, Bitacora: &bitacoraFalsa{}, Reloj: relojFijo{}},
		"sin UnidadDeTrabajo":    {Alertas: &alertasFalsas{}, Bitacora: &bitacoraFalsa{}, Reloj: relojFijo{}},
		"sin BitacoraAuditoria":  {Alertas: &alertasFalsas{}, Unidad: &unidadFalsa{}, Reloj: relojFijo{}},
		"sin Reloj":              {Alertas: &alertasFalsas{}, Unidad: &unidadFalsa{}, Bitacora: &bitacoraFalsa{}},
	}
	for nombre, svc := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err == nil {
				t.Fatal("Evaluar no fallo con el servicio a medias")
			}
			if _, err := svc.Resolver(t.Context(), "al-1", "usr-1", ""); err == nil {
				t.Fatal("Resolver no fallo con el servicio a medias")
			}
		})
	}
}
