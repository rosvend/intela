package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ RepositorioLiquidacion = (*repoLiqMemoria)(nil)

// repoLiqMemoria es el doble de [RepositorioLiquidacion].
//
// Cuenta los cerrojos que se le piden (bloqueos, cerrojosDeDiferidas) porque el
// contrato del puerto dice que la generacion los toma, y un doble que los
// ignorase dejaria pasar una version del caso de uso que no los pide: la
// serializacion de verdad se comprueba contra Postgres, pero que se PIDA se
// comprueba aqui.
type repoLiqMemoria struct {
	ordenes  []liquidacion.OrdenDePago
	procesos map[string]MetaProceso
	insumos  map[string]InsumoLiquidacion

	smmlv    decimal.Decimal
	smmlvErr error
	docs     map[string]liquidacion.Documentos

	emisiones           int
	bloqueos            int
	cerrojosDeDiferidas int
}

func (r *repoLiqMemoria) sembrar(meta MetaProceso, insumo InsumoLiquidacion) {
	if r.procesos == nil {
		r.procesos = map[string]MetaProceso{}
	}
	if r.insumos == nil {
		r.insumos = map[string]InsumoLiquidacion{}
	}
	insumo.ProcesoID = meta.ID
	insumo.Periodo = meta.Periodo
	r.procesos[meta.ID] = meta
	r.insumos[meta.ID] = insumo
}

func (r *repoLiqMemoria) DeTitular(_ context.Context, titularID string) ([]liquidacion.OrdenDePago, error) {
	out := []liquidacion.OrdenDePago{}
	for _, o := range r.ordenes {
		if o.TitularID == titularID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *repoLiqMemoria) Listar(context.Context) ([]liquidacion.OrdenDePago, error) {
	return append([]liquidacion.OrdenDePago{}, r.ordenes...), nil
}

func (r *repoLiqMemoria) DeProceso(_ context.Context, procesoID string) ([]liquidacion.OrdenDePago, error) {
	out := []liquidacion.OrdenDePago{}
	for _, o := range r.ordenes {
		if o.ProcesoID == procesoID || slices.Contains(o.Procesos, procesoID) {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *repoLiqMemoria) DePeriodoCircuito(
	_ context.Context, periodo string, circuito reparto.Circuito,
) ([]liquidacion.OrdenDePago, error) {
	out := []liquidacion.OrdenDePago{}
	for _, o := range r.ordenes {
		if o.Periodo == periodo && o.Circuito == string(circuito) {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *repoLiqMemoria) BloquearPeriodo(context.Context, string, reparto.Circuito) error {
	r.bloqueos++
	return nil
}

func (r *repoLiqMemoria) DiferidasDeTitular(
	_ context.Context, titularID string, circuito reparto.Circuito, antesDe string,
) ([]liquidacion.OrdenDePago, error) {
	r.cerrojosDeDiferidas++
	out := []liquidacion.OrdenDePago{}
	for _, o := range r.ordenes {
		if o.TitularID != titularID || o.Estado != liquidacion.EstadoDiferida {
			continue
		}
		if o.Circuito != string(circuito) || o.Periodo >= antesDe {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

// EmitirOrdenes no pisa lo que ya existe, igual que el adaptador real: el
// contrato del puerto es lo que se prueba, no la comodidad del doble.
func (r *repoLiqMemoria) EmitirOrdenes(_ context.Context, ordenes []liquidacion.OrdenDePago) error {
	r.emisiones++
	for _, o := range ordenes {
		if slices.ContainsFunc(r.ordenes, func(x liquidacion.OrdenDePago) bool { return x.ID == o.ID }) {
			continue
		}
		r.ordenes = append(r.ordenes, o)
	}
	return nil
}

func (r *repoLiqMemoria) TransicionarOrdenes(
	_ context.Context, ordenes []liquidacion.OrdenDePago, desde liquidacion.Estado,
) ([]liquidacion.OrdenDePago, error) {
	aplicadas := []liquidacion.OrdenDePago{}
	for _, o := range ordenes {
		for i := range r.ordenes {
			if r.ordenes[i].ID != o.ID || r.ordenes[i].Estado != desde {
				continue
			}
			r.ordenes[i].Estado = o.Estado
			aplicadas = append(aplicadas, r.ordenes[i])
			break
		}
	}
	return aplicadas, nil
}

func (r *repoLiqMemoria) DocumentosDe(_ context.Context, titularID string) (liquidacion.Documentos, error) {
	return r.docs[titularID], nil
}

func (r *repoLiqMemoria) Documentos(context.Context) (map[string]liquidacion.Documentos, error) {
	if r.docs == nil {
		return map[string]liquidacion.Documentos{}, nil
	}
	return r.docs, nil
}

func (r *repoLiqMemoria) MetaDeProceso(_ context.Context, procesoID string) (MetaProceso, error) {
	meta, hay := r.procesos[procesoID]
	if !hay {
		return MetaProceso{}, ErrNoEncontrado
	}
	return meta, nil
}

func (r *repoLiqMemoria) ProcesosListos(
	_ context.Context, periodo string, circuito reparto.Circuito,
) ([]string, error) {
	ids := []string{}
	for id, meta := range r.procesos {
		if meta.Periodo != periodo || meta.Circuito != circuito {
			continue
		}
		if meta.ExigirListoParaLiquidar() != nil {
			continue
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func (r *repoLiqMemoria) InsumoDeProceso(_ context.Context, procesoID string) (InsumoLiquidacion, error) {
	insumo, hay := r.insumos[procesoID]
	if !hay {
		return InsumoLiquidacion{}, ErrNoEncontrado
	}
	return insumo, nil
}

func (r *repoLiqMemoria) SMMLVVigente(context.Context, time.Time) (decimal.Decimal, error) {
	return r.smmlv, r.smmlvErr
}

// notificadorFalso apunta cada aviso y devuelve un acuse distinto por envio.
// El acuse importa: es lo que el asiento de la emision guarda como prueba de
// que el plazo de R-10 empezo a correr.
type notificadorFalso struct {
	enviados []avisoEnviado
	err      error
}

type avisoEnviado struct {
	Dest   string
	Asunto string
	Cuerpo string
}

func (n *notificadorFalso) Notificar(_ context.Context, dest, asunto, cuerpo string) (string, error) {
	if n.err != nil {
		return "", n.err
	}
	n.enviados = append(n.enviados, avisoEnviado{Dest: dest, Asunto: asunto, Cuerpo: cuerpo})
	return fmt.Sprintf("acuse-%d", len(n.enviados)), nil
}

func liqDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func envio() time.Time {
	return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
}

// entornoLiq cablea los cinco puertos como lo hace cmd/api, y deja a mano los
// dobles sobre los que se comprueba el ADR 0006 (el libro) y R-10 (los avisos).
type entornoLiq struct {
	repo   *repoLiqMemoria
	libro  *bitacoraFalsa
	avisos *notificadorFalso
	unidad *unidadFalsa
	svc    Liquidaciones
}

func montar(repo *repoLiqMemoria, instante time.Time) *entornoLiq {
	e := &entornoLiq{
		repo:   repo,
		libro:  &bitacoraFalsa{},
		avisos: &notificadorFalso{},
		unidad: &unidadFalsa{},
	}
	e.svc = Liquidaciones{
		Ordenes:     repo,
		Reloj:       relojFijo{instante: instante},
		Notificador: e.avisos,
		Bitacora:    e.libro,
		Unidad:      e.unidad,
	}
	return e
}

func servicio(repo *repoLiqMemoria, instante time.Time) Liquidaciones {
	return montar(repo, instante).svc
}

// metaLista es una corrida que YA paso la compuerta del RD 13.5: etapa
// liquidacion_final y las dos firmas sobre la revision vigente.
func metaLista(id, periodo string, circuito reparto.Circuito) MetaProceso {
	return MetaProceso{
		ID:       id,
		Periodo:  periodo,
		Circuito: circuito,
		Etapa:    reparto.EtapaLiquidacionFinal,
		Revision: 1,
		Firmas: []reparto.Firma{
			{Rol: string(RolDistribucion), ActorID: "usr-dist", SobreRev: 1},
			{Rol: string(RolContabilidad), ActorID: "usr-cont", SobreRev: 1},
		},
	}
}

// insumoDosTitulares es una corrida cuyo neto se reparte ENTERO: bruto 1000,
// deducciones 350, neto 650, y las lineas suman exactamente 650.
//
// Que sumen el neto no es un detalle del fixture, es la invariante de una
// corrida sin retenido, y antes no se cumplia: las lineas sumaban 910 sobre un
// neto de 650, asi que las proporciones sumaban 1,4 y cada orden cargaba mas
// deducciones de las que la corrida aplico. Ana lleva DOS lineas a proposito,
// para que se compruebe tambien que se agrupan por titular.
func insumoDosTitulares() InsumoLiquidacion {
	return InsumoLiquidacion{
		Bruto:   liqDec("1000"),
		Admin:   liqDec("200"),
		Social:  liqDec("100"),
		Reserva: liqDec("50"),
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-a", TitularID: "tit-ana", IPI: "1", Porcentaje: liqDec("60"), Importe: liqDec("234")},
			{ObraID: "obra-b", TitularID: "tit-ana", IPI: "1", Porcentaje: liqDec("100"), Importe: liqDec("156")},
			{ObraID: "obra-a", TitularID: "tit-beto", IPI: "2", Porcentaje: liqDec("40"), Importe: liqDec("260")},
		},
	}
}

func repoDosTitulares() *repoLiqMemoria {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs: map[string]liquidacion.Documentos{
			"tit-ana":  {RUT: true, CertificacionBancaria: true},
			"tit-beto": {RUT: true, CertificacionBancaria: true},
		},
	}
	repo.sembrar(metaLista("prc-1", "2026", reparto.Nacional), insumoDosTitulares())
	return repo
}

func porTitular(vistas []OrdenVista) map[string]OrdenVista {
	out := map[string]OrdenVista{}
	for _, v := range vistas {
		out[v.Orden.TitularID] = v
	}
	return out
}

func TestGenerarLiquidacionItemizaDeduccionesPorTitular(t *testing.T) {
	repo := repoDosTitulares()

	vistas, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	if len(vistas) != 2 {
		t.Fatalf("se esperaban 2 ordenes (una por titular), llegaron %d", len(vistas))
	}

	ana := porTitular(vistas)["tit-ana"].Orden
	if !ana.Neto.Equal(liqDec("390")) {
		t.Fatalf("neto ana = %s, se esperaba 390 (234+156)", ana.Neto)
	}
	if len(ana.Deducciones) != 3 {
		t.Fatalf("ana: %d deducciones, se esperaban 3 itemizadas", len(ana.Deducciones))
	}
	// 390/650 de cada deduccion de la corrida. El denominador es el NETO DE LA
	// CORRIDA (1000-200-100-50), no la suma de las lineas.
	if !ana.Deducciones[0].Monto.Equal(liqDec("120")) { // 200 * 390/650
		t.Fatalf("admin ana = %s, se esperaba 120.00", ana.Deducciones[0].Monto)
	}
	if ana.Deducciones[0].Concepto != liquidacion.ConceptoAdministracion {
		t.Fatalf("concepto admin = %q", ana.Deducciones[0].Concepto)
	}
	if !ana.Deducciones[1].Monto.Equal(liqDec("60")) { // 100 * 390/650
		t.Fatalf("social ana = %s, se esperaba 60.00", ana.Deducciones[1].Monto)
	}
	if !ana.Deducciones[2].Monto.Equal(liqDec("30")) { // 50 * 390/650
		t.Fatalf("reserva ana = %s, se esperaba 30.00", ana.Deducciones[2].Monto)
	}
	if !ana.Bruto.Equal(liqDec("600")) {
		t.Fatalf("bruto ana = %s, se esperaba 600 (390 + 210)", ana.Bruto)
	}
	if ana.Circuito != string(reparto.Nacional) {
		t.Fatalf("circuito ana = %q, se esperaba nacional", ana.Circuito)
	}

	beto := porTitular(vistas)["tit-beto"].Orden
	if !beto.Neto.Equal(liqDec("260")) {
		t.Fatalf("neto beto = %s", beto.Neto)
	}
	if !beto.Bruto.Equal(liqDec("400")) {
		t.Fatalf("bruto beto = %s, se esperaba 400 (260 + 140)", beto.Bruto)
	}
	// Los dos brutos suman el bruto de la corrida y las deducciones tambien:
	// el prorrateo reparte el cierre, no una cifra nueva.
	if !ana.Bruto.Add(beto.Bruto).Equal(liqDec("1000")) {
		t.Fatalf("los brutos suman %s y la corrida cerro 1000", ana.Bruto.Add(beto.Bruto))
	}
	if ana.Estado != liquidacion.EstadoEnviada || beto.Estado != liquidacion.EstadoEnviada {
		t.Fatal("recien generadas tienen que estar enviadas")
	}
}

// TestGenerarLiquidacionProrrateaConRetenido es el caso que distingue los dos
// denominadores posibles.
//
// La corrida cierra con bruto 1000 y neto 650, pero solo distribuye 325: la
// otra mitad queda retenida. Ana carga la MITAD de las deducciones (175) y su
// tasa es el 35%, la misma que aplico la corrida. Con la suma de las lineas
// como denominador cargaria las 350 enteras: un 51,85% que nadie aplico.
func TestGenerarLiquidacionProrrateaConRetenido(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{},
	}
	repo.sembrar(metaLista("prc-1", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto:   liqDec("1000"),
		Admin:   liqDec("200"),
		Social:  liqDec("100"),
		Reserva: liqDec("50"),
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-a", TitularID: "tit-ana", IPI: "1", Importe: liqDec("325")},
		},
	})

	vistas, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	ana := vistas[0].Orden
	if !ana.Neto.Equal(liqDec("325")) {
		t.Fatalf("neto ana = %s, se esperaba 325 (el motor manda)", ana.Neto)
	}
	deducido := decimal.Zero
	for _, d := range ana.Deducciones {
		deducido = deducido.Add(d.Monto)
	}
	if !deducido.Equal(liqDec("175")) {
		t.Fatalf("deducciones ana = %s, se esperaban 175 (la mitad de 350)", deducido)
	}
	if !ana.Bruto.Equal(liqDec("500")) {
		t.Fatalf("bruto ana = %s, se esperaba 500", ana.Bruto)
	}
	tasaOrden := deducido.Div(ana.Bruto)
	tasaCorrida := liqDec("350").Div(liqDec("1000"))
	if !tasaOrden.Equal(tasaCorrida) {
		t.Fatalf("tasa de la orden = %s, la de la corrida = %s (35%%)", tasaOrden, tasaCorrida)
	}
}

// TestGenerarLiquidacionAgregaLasCorridasDelPeriodoYCircuito es el ADR 0019.
//
// Dos corridas nacionales de 2026 -- dos bolsas del mismo periodo -- y una
// internacional. Ana tiene que recibir UNA orden nacional con las dos lineas
// nacionales sumadas, y otra distinta por el internacional: una orden por
// corrida partiria su periodo en dos avisos con dos plazos.
func TestGenerarLiquidacionAgregaLasCorridasDelPeriodoYCircuito(t *testing.T) {
	repo := &repoLiqMemoria{smmlv: liqDec("1300000"), docs: map[string]liquidacion.Documentos{}}
	unaLinea := func(importe string) []reparto.LineaTitular {
		return []reparto.LineaTitular{{ObraID: "obra-a", TitularID: "tit-ana", IPI: "1", Importe: liqDec(importe)}}
	}
	repo.sembrar(metaLista("prc-nac-a", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto: liqDec("100000"), Admin: liqDec("20000"), Social: liqDec("10000"), Reserva: liqDec("5000"),
		Titulares: unaLinea("65000"),
	})
	repo.sembrar(metaLista("prc-nac-b", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto: liqDec("200000"), Admin: liqDec("40000"), Social: liqDec("20000"), Reserva: liqDec("10000"),
		Titulares: unaLinea("130000"),
	})
	repo.sembrar(metaLista("prc-int", "2026", reparto.Internacional), InsumoLiquidacion{
		Bruto: liqDec("50000"), Admin: liqDec("10000"), Social: liqDec("5000"), Reserva: liqDec("2500"),
		Titulares: unaLinea("32500"),
	})

	// Se dispara con la SEGUNDA corrida nacional: procesoID es el disparador,
	// no el alcance.
	vistas, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-nac-b")
	if err != nil {
		t.Fatalf("GenerarLiquidacion nacional: %v", err)
	}
	if len(vistas) != 1 {
		t.Fatalf("se esperaba 1 orden nacional para Ana, llegaron %d", len(vistas))
	}
	nac := vistas[0].Orden
	if !nac.Neto.Equal(liqDec("195000")) {
		t.Fatalf("neto nacional = %s, se esperaban 195000 (65000+130000)", nac.Neto)
	}
	if !nac.Bruto.Equal(liqDec("300000")) {
		t.Fatalf("bruto nacional = %s, se esperaban 300000 (100000+200000)", nac.Bruto)
	}
	if nac.ID != "liq-2026-nacional-tit-ana" {
		t.Fatalf("id = %q; el id es (periodo, circuito, titular) y no lleva la corrida", nac.ID)
	}
	if !slices.Equal(nac.Procesos, []string{"prc-nac-a", "prc-nac-b"}) {
		t.Fatalf("Procesos = %v, se esperaban las dos corridas contribuyentes", nac.Procesos)
	}
	if nac.ProcesoID != "prc-nac-a" {
		t.Fatalf("ProcesoID de referencia = %q, se esperaba la primera lexicografica", nac.ProcesoID)
	}

	// El internacional es OTRA orden: RD 7.4 son dos recorridos distintos.
	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-int"); err != nil {
		t.Fatalf("GenerarLiquidacion internacional: %v", err)
	}
	if len(repo.ordenes) != 2 {
		t.Fatalf("%d ordenes; se esperaban 2 (una por circuito)", len(repo.ordenes))
	}
	deAna, err := repo.DeTitular(context.Background(), "tit-ana")
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	circuitos := []string{}
	for _, o := range deAna {
		circuitos = append(circuitos, o.Circuito)
	}
	slices.Sort(circuitos)
	if !slices.Equal(circuitos, []string{"internacional", "nacional"}) {
		t.Fatalf("circuitos de Ana = %v", circuitos)
	}
}

func TestGenerarLiquidacionEsIdempotente(t *testing.T) {
	repo := repoDosTitulares()
	svc := servicio(repo, envio())
	if _, err := svc.GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("primera: %v", err)
	}
	n := len(repo.ordenes)
	if _, err := svc.GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("segunda: %v", err)
	}
	if len(repo.ordenes) != n {
		t.Fatalf("la segunda corrida duplico ordenes: %d -> %d", n, len(repo.ordenes))
	}
	if repo.bloqueos != 2 {
		t.Fatalf("bloqueos = %d; cada generacion toma el cerrojo del periodo antes de decidir", repo.bloqueos)
	}
	if repo.emisiones != 1 {
		t.Fatalf("emisiones = %d; la segunda no puede volver a emitir", repo.emisiones)
	}
}

// TestGenerarLiquidacionNoRegeneraTrasDiferida es el defecto que la guarda de
// idempotencia existe para impedir.
//
// Una orden diferida por R-11 sigue siendo la orden del periodo. Sin la
// guarda, volver a generar el mismo proceso la lee como "diferida de este
// titular", se la incorpora a una orden nueva del MISMO id, la marca acumulada
// y deja el periodo con una orden acumulada de su propio neto: el saldo se
// duplica y el arrastre real desaparece.
func TestGenerarLiquidacionNoRegeneraTrasDiferida(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true, CertificacionBancaria: true}},
	}
	repo.sembrar(metaLista("prc-1", "2026-01", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("1000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("1000")}},
	})

	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("generar: %v", err)
	}

	// Dia 15 sin respuesta y 1000 <= 26000: se difiere.
	dia15 := envio().AddDate(0, 0, 15)
	if _, err := servicio(repo, dia15).Listar(context.Background(), Usuario{Rol: RolAdministrador}); err != nil {
		t.Fatalf("silencio: %v", err)
	}
	if repo.ordenes[0].Estado != liquidacion.EstadoDiferida {
		t.Fatalf("Estado = %q, se esperaba diferida", repo.ordenes[0].Estado)
	}

	vistas, err := servicio(repo, dia15).GenerarLiquidacion(context.Background(), "prc-1")
	if err != nil {
		t.Fatalf("regenerar: %v", err)
	}
	if len(repo.ordenes) != 1 {
		t.Fatalf("%d ordenes tras regenerar; se esperaba 1", len(repo.ordenes))
	}
	if len(vistas) != 1 {
		t.Fatalf("%d vistas", len(vistas))
	}
	got := vistas[0].Orden
	if got.Estado != liquidacion.EstadoDiferida {
		t.Fatalf("Estado = %q; una diferida no se acumula de si misma", got.Estado)
	}
	if !got.Neto.Equal(liqDec("1000")) {
		t.Fatalf("neto = %s, se esperaba 1000 intacto", got.Neto)
	}
	if len(got.Arrastres) != 0 {
		t.Fatalf("Arrastres = %v; no se arrastro nada", got.Arrastres)
	}
}

func TestGenerarLiquidacionNoRegeneraTrasAceptadaPorSilencio(t *testing.T) {
	repo := repoDosTitulares()
	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("generar: %v", err)
	}

	// 390 y 260 no superan el umbral de 26000, asi que se diferirian. Para
	// probar el otro estado hace falta un neto por encima: se baja el SMMLV.
	repo.smmlv = liqDec("100")
	dia15 := envio().AddDate(0, 0, 15)
	if _, err := servicio(repo, dia15).Listar(context.Background(), Usuario{Rol: RolAdministrador}); err != nil {
		t.Fatalf("silencio: %v", err)
	}
	for _, o := range repo.ordenes {
		if o.Estado != liquidacion.EstadoAceptadaPorSilencio {
			t.Fatalf("%s: Estado = %q, se esperaba aceptada_por_silencio", o.ID, o.Estado)
		}
	}
	netos := map[string]decimal.Decimal{}
	for _, o := range repo.ordenes {
		netos[o.ID] = o.Neto
	}

	if _, err := servicio(repo, dia15).GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("regenerar: %v", err)
	}
	if len(repo.ordenes) != 2 {
		t.Fatalf("%d ordenes tras regenerar; se esperaban 2", len(repo.ordenes))
	}
	for _, o := range repo.ordenes {
		if o.Estado != liquidacion.EstadoAceptadaPorSilencio {
			t.Fatalf("%s volvio a %q: regenerar reabriria un plazo ya vencido", o.ID, o.Estado)
		}
		if !o.Neto.Equal(netos[o.ID]) {
			t.Fatalf("%s: neto %s -> %s", o.ID, netos[o.ID], o.Neto)
		}
	}
}

func TestGenerarLiquidacionExigeEtapaLiquidacionFinal(t *testing.T) {
	repo := repoDosTitulares()
	meta := repo.procesos["prc-1"]
	meta.Etapa = reparto.EtapaVerificacion
	repo.procesos["prc-1"] = meta

	_, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if !errors.Is(err, ErrProcesoNoListo) {
		t.Fatalf("se esperaba ErrProcesoNoListo, se obtuvo %v", err)
	}
	if len(repo.ordenes) != 0 {
		t.Fatalf("no se puede emitir nada: %d ordenes", len(repo.ordenes))
	}
	if repo.emisiones != 0 {
		t.Fatal("la compuerta corre ANTES de abrir la unidad")
	}
}

func TestGenerarLiquidacionExigeLasDosFirmasDeLaRevisionVigente(t *testing.T) {
	casos := []struct {
		nombre string
		firmas []reparto.Firma
	}{
		{nombre: "sin firmas", firmas: nil},
		{
			nombre: "solo distribucion",
			firmas: []reparto.Firma{{Rol: string(RolDistribucion), ActorID: "usr-dist", SobreRev: 2}},
		},
		{
			// Las dos firmas, pero de la revision que el rechazo invalido.
			nombre: "las dos sobre una revision anterior",
			firmas: []reparto.Firma{
				{Rol: string(RolDistribucion), ActorID: "usr-dist", SobreRev: 1},
				{Rol: string(RolContabilidad), ActorID: "usr-cont", SobreRev: 1},
			},
		},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			repo := repoDosTitulares()
			meta := repo.procesos["prc-1"]
			meta.Revision = 2
			meta.Firmas = tt.firmas
			repo.procesos["prc-1"] = meta

			_, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
			if !errors.Is(err, ErrProcesoNoListo) {
				t.Fatalf("se esperaba ErrProcesoNoListo, se obtuvo %v", err)
			}
			if len(repo.ordenes) != 0 {
				t.Fatalf("%d ordenes emitidas sin la doble firma", len(repo.ordenes))
			}
		})
	}
}

func TestGenerarLiquidacionNotificaYAsientaLaEmision(t *testing.T) {
	e := montar(repoDosTitulares(), envio())

	if _, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}

	if len(e.avisos.enviados) != 2 {
		t.Fatalf("%d avisos; R-10 cuenta desde el envio, asi que hay uno por orden", len(e.avisos.enviados))
	}
	emisiones, residuos := 0, 0
	for _, a := range e.libro.asientos {
		switch a.Hecho {
		case HechoLiquidacionEmitida:
			emisiones++
			if a.RefTipo != RefOrdenDePago {
				t.Fatalf("ref_tipo emision = %q", a.RefTipo)
			}
			if a.ActorID != "" {
				t.Fatalf("actor = %q; la emision la produce el sistema y actor_id referencia usuarios(id)", a.ActorID)
			}
			if !a.Cuando.Equal(envio()) {
				t.Fatalf("cuando = %s; el instante entra por el Reloj", a.Cuando)
			}
			if !strings.Contains(string(a.Payload), `"acuse":"acuse-`) {
				t.Fatalf("el asiento no lleva el acuse de la notificacion: %s", a.Payload)
			}
			if strings.Contains(string(a.Payload), "residuo_prorrateo") {
				t.Fatal("el residuo no viaja en el asiento de cada orden")
			}
		case HechoLiquidacionResiduoProrrateo:
			residuos++
			if a.RefTipo != RefLiquidacionLote || a.RefID != "2026-nacional" {
				t.Fatalf("residuo ref = %s/%s", a.RefTipo, a.RefID)
			}
		default:
			t.Fatalf("hecho inesperado = %q", a.Hecho)
		}
	}
	if emisiones != 2 {
		t.Fatalf("%d asientos de emision; el ADR 0006 pide uno por orden", emisiones)
	}
	if residuos != 1 {
		t.Fatalf("%d asientos de residuo; el lote asienta UNA vez", residuos)
	}
	if !e.unidad.confirmo {
		t.Fatal("la orden, su asiento y el cierre de las diferidas son un solo hecho")
	}
}

func TestGenerarLiquidacionNoEmiteSiLaNotificacionFalla(t *testing.T) {
	e := montar(repoDosTitulares(), envio())
	e.avisos.err = errors.New("smtp caido")

	if _, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1"); err == nil {
		t.Fatal("una orden enviada cuyo aviso no salio corre un plazo contra alguien que no sabe nada")
	}
	if len(e.repo.ordenes) != 0 {
		t.Fatalf("%d ordenes emitidas sin aviso", len(e.repo.ordenes))
	}
	if e.unidad.confirmo {
		t.Fatal("la unidad no puede confirmar")
	}
}

func TestSilencioAsientaLaTransicion(t *testing.T) {
	e := montar(repoDosTitulares(), envio())
	if _, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("generar: %v", err)
	}
	asientosDeEmision := len(e.libro.asientos)

	tarde := montar(e.repo, envio().AddDate(0, 0, 15))
	if _, err := tarde.svc.Listar(context.Background(), Usuario{Rol: RolAdministrador}); err != nil {
		t.Fatalf("silencio: %v", err)
	}
	if len(tarde.libro.asientos) == 0 {
		t.Fatal("una transicion de R-10 / R-11 es un hecho y va al libro (ADR 0006)")
	}
	for _, a := range tarde.libro.asientos {
		if a.Hecho != HechoLiquidacionDiferida {
			t.Fatalf("hecho = %q, se esperaba %q (390 y 260 no llegan a 26000)", a.Hecho, HechoLiquidacionDiferida)
		}
	}
	if !tarde.unidad.confirmo {
		t.Fatal("la transicion y su asiento van en la misma unidad")
	}
	if asientosDeEmision != 3 { // 2 emitida + 1 residuo de lote
		t.Fatalf("asientos de emision = %d", asientosDeEmision)
	}
}

func TestSilencioALos15DiasConRelojFijo(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true, CertificacionBancaria: true}},
	}
	repo.sembrar(metaLista("prc-1", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("100000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("100000")}},
	})

	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("generar: %v", err)
	}

	actor := Usuario{Rol: RolTitular, TitularID: "tit-ana"}

	dia14 := envio().AddDate(0, 0, 14) // 2026-01-15
	vistas, err := servicio(repo, dia14).DeTitular(context.Background(), actor)
	if err != nil {
		t.Fatalf("dia 14: %v", err)
	}
	if vistas[0].Orden.Estado != liquidacion.EstadoEnviada {
		t.Fatalf("dia 14: Estado = %q, se esperaba enviada", vistas[0].Orden.Estado)
	}
	if vistas[0].Pagable {
		t.Fatal("dia 14 no es pagable: el titular todavia puede objetar")
	}

	dia15 := envio().AddDate(0, 0, 15) // 2026-01-16
	vistas, err = servicio(repo, dia15).DeTitular(context.Background(), actor)
	if err != nil {
		t.Fatalf("dia 15: %v", err)
	}
	if vistas[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("dia 15: Estado = %q, se esperaba aceptada_por_silencio", vistas[0].Orden.Estado)
	}
	if !vistas[0].Pagable {
		t.Fatal("aceptada por silencio, con documentos, neto sobre umbral: pagable")
	}
}

func TestMenorCuantiaSeAcumulaYSePagaAlSuperarUmbral(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true, CertificacionBancaria: true}},
	}
	repo.sembrar(metaLista("prc-1", "2026-01", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("1000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("1000")}},
	})

	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("periodo 1: %v", err)
	}

	actor := Usuario{Rol: RolAdministrador}
	dia15 := envio().AddDate(0, 0, 15)
	vistas, err := servicio(repo, dia15).Listar(context.Background(), actor)
	if err != nil {
		t.Fatalf("silencio periodo 1: %v", err)
	}
	if vistas[0].Orden.Estado != liquidacion.EstadoDiferida {
		t.Fatalf("1000 <= 26000: Estado = %q, se esperaba diferida", vistas[0].Orden.Estado)
	}
	if vistas[0].Pagable {
		t.Fatal("una diferida no es pagable")
	}

	repo.sembrar(metaLista("prc-2", "2026-02", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("30000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("30000")}},
	})
	envio2 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	e2 := montar(repo, envio2)
	vistas, err = e2.svc.GenerarLiquidacion(context.Background(), "prc-2")
	if err != nil {
		t.Fatalf("periodo 2: %v", err)
	}
	if len(vistas) != 1 {
		t.Fatalf("periodo 2 produjo %d ordenes", len(vistas))
	}
	if !vistas[0].Orden.Neto.Equal(liqDec("31000")) {
		t.Fatalf("neto periodo 2 = %s, se esperaba 31000 (30000+1000)", vistas[0].Orden.Neto)
	}
	if repo.cerrojosDeDiferidas == 0 {
		t.Fatal("el arrastre se lee con la fila bloqueada, no de cualquier manera")
	}
	// B2: el aviso de R-10 lleva la cifra FINAL (con arrastre), no el neto
	// de la corrida antes de IncorporarArrastre.
	if len(e2.avisos.enviados) != 1 {
		t.Fatalf("%d avisos en periodo 2", len(e2.avisos.enviados))
	}
	cuerpo := e2.avisos.enviados[0].Cuerpo
	if !strings.Contains(cuerpo, "31000.00") {
		t.Fatalf("el aviso no lleva el neto con arrastre (31000.00): %q", cuerpo)
	}
	if strings.Contains(cuerpo, "Bruto 30000.00") || strings.Contains(cuerpo, "neto 30000.00") {
		t.Fatalf("el aviso anuncia 30000 sin el arrastre: %q", cuerpo)
	}
	if !strings.Contains(cuerpo, "Incluye arrastre de liq-2026-01-nacional-tit-ana") {
		t.Fatalf("el aviso no menciona el arrastre absorbido: %q", cuerpo)
	}

	var p1 liquidacion.OrdenDePago
	for _, o := range repo.ordenes {
		if o.Periodo == "2026-01" {
			p1 = o
		}
	}
	if p1.Estado != liquidacion.EstadoAcumulada {
		t.Fatalf("periodo 1 tiene que quedar acumulado: %q", p1.Estado)
	}

	vistas, err = servicio(repo, envio2.AddDate(0, 0, 15)).Listar(context.Background(), actor)
	if err != nil {
		t.Fatalf("silencio periodo 2: %v", err)
	}
	var p2 OrdenVista
	for _, v := range vistas {
		if v.Orden.Periodo == "2026-02" {
			p2 = v
		}
	}
	if p2.Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("31000 > 26000: Estado = %q", p2.Orden.Estado)
	}
	if !p2.Pagable {
		t.Fatal("superado el umbral, con documentos: pagable")
	}
}

func TestOrdenSinDocumentosNoEsPagable(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true}}, // sin certificacion bancaria
	}
	repo.sembrar(metaLista("prc-1", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("100000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("100000")}},
	})
	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("generar: %v", err)
	}
	vistas, err := servicio(repo, envio().AddDate(0, 0, 15)).DeTitular(
		context.Background(), Usuario{Rol: RolTitular, TitularID: "tit-ana"})
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if vistas[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("Estado = %q", vistas[0].Orden.Estado)
	}
	if vistas[0].Pagable {
		t.Fatal("sin certificacion bancaria la orden no es pagable (R-12)")
	}
}

func TestGenerarLiquidacionRechazaUnaCorridaQueNoCuadra(t *testing.T) {
	repo := &repoLiqMemoria{smmlv: liqDec("1300000")}
	// Bruto 1000, deducciones 350, neto 650, y las lineas reparten 900: las
	// proporciones sumarian 1,38 y cada orden mostraria menos deducciones de
	// las que la corrida aplico.
	repo.sembrar(metaLista("prc-1", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto:   liqDec("1000"),
		Admin:   liqDec("200"),
		Social:  liqDec("100"),
		Reserva: liqDec("50"),
		Titulares: []reparto.LineaTitular{
			{TitularID: "tit-ana", Importe: liqDec("900")},
		},
	})
	_, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if !errors.Is(err, ErrCorridaNoCuadra) {
		t.Fatalf("se esperaba ErrCorridaNoCuadra, se obtuvo %v", err)
	}
	if len(repo.ordenes) != 0 {
		t.Fatalf("%d ordenes emitidas de una corrida que no cuadra", len(repo.ordenes))
	}
}

func TestListarRechazaAlTitular(t *testing.T) {
	repo := &repoLiqMemoria{smmlv: liqDec("1300000")}
	_, err := servicio(repo, envio()).Listar(context.Background(), Usuario{Rol: RolTitular, TitularID: "tit-ana"})
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("se esperaba ErrNoAutorizado, se obtuvo %v", err)
	}
}

func TestDeTitularRechazaAlAdmin(t *testing.T) {
	repo := &repoLiqMemoria{smmlv: liqDec("1300000")}
	_, err := servicio(repo, envio()).DeTitular(context.Background(), Usuario{Rol: RolAdministrador})
	if !errors.Is(err, ErrNoAutorizado) {
		t.Fatalf("se esperaba ErrNoAutorizado, se obtuvo %v", err)
	}
}

func TestGenerarLiquidacionFallaSinSMMLV(t *testing.T) {
	repo := &repoLiqMemoria{smmlvErr: ErrParametroAusente}
	repo.sembrar(metaLista("prc-1", "2026", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("1"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("1")}},
	})
	_, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if !errors.Is(err, ErrParametroAusente) {
		t.Fatalf("se esperaba ErrParametroAusente, se obtuvo %v", err)
	}
	// Y sin haber tocado nada: fallar despues de insertar y notificar dejaria
	// al titular con un aviso de una orden que no se puede servir.
	if len(repo.ordenes) != 0 || repo.emisiones != 0 {
		t.Fatal("el SMMLV se resuelve antes de emitir")
	}
}

func TestGenerarLiquidacionExigeEstarCableada(t *testing.T) {
	repo := repoDosTitulares()
	_, err := Liquidaciones{Ordenes: repo, Reloj: relojFijo{instante: envio()}}.
		GenerarLiquidacion(context.Background(), "prc-1")
	if err == nil {
		t.Fatal("sin UnidadDeTrabajo, Bitacora ni Notificador no se puede emitir")
	}
	if len(repo.ordenes) != 0 {
		t.Fatal("no se puede haber escrito nada")
	}
}

// TestArrastreNoCruzaCircuitos es B1: una diferida nacional no entra en una
// orden internacional del mismo titular (RD 7.4, ADR 0008 / 0019).
func TestArrastreNoCruzaCircuitos(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true, CertificacionBancaria: true}},
	}
	repo.sembrar(metaLista("prc-nac", "2026-01", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("1000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("1000")}},
	})
	if _, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-nac"); err != nil {
		t.Fatalf("nacional: %v", err)
	}
	if _, err := servicio(repo, envio().AddDate(0, 0, 15)).Listar(context.Background(), Usuario{Rol: RolAdministrador}); err != nil {
		t.Fatalf("diferir nacional: %v", err)
	}

	repo.sembrar(metaLista("prc-int", "2026-02", reparto.Internacional), InsumoLiquidacion{
		Bruto:     liqDec("30000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("30000")}},
	})
	vistas, err := servicio(repo, time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)).
		GenerarLiquidacion(context.Background(), "prc-int")
	if err != nil {
		t.Fatalf("internacional: %v", err)
	}
	if len(vistas) != 1 {
		t.Fatalf("%d ordenes", len(vistas))
	}
	o := vistas[0].Orden
	if !o.Neto.Equal(liqDec("30000")) {
		t.Fatalf("neto = %s; la diferida nacional no puede entrar en el internacional", o.Neto)
	}
	if len(o.Arrastres) != 0 {
		t.Fatalf("arrastres = %v; el circuito no se cruza", o.Arrastres)
	}
	for _, prev := range repo.ordenes {
		if prev.Periodo == "2026-01" && prev.Estado != liquidacion.EstadoDiferida {
			t.Fatalf("nacional 2026-01 quedo %q; sigue diferida hasta un nacional posterior", prev.Estado)
		}
	}
}

// TestArrastreNoAbsorbePeriodoPosterior es B1: una diferida de un periodo
// posterior no se absorbe al liquidar uno anterior (R-11: "siguiente periodo").
func TestArrastreNoAbsorbePeriodoPosterior(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  map[string]liquidacion.Documentos{"tit-ana": {RUT: true, CertificacionBancaria: true}},
	}
	// Diferida "futura" ya en el libro (laboratorio: se liquido 2026-03 antes).
	futura, err := liquidacion.NuevaOrden(liquidacion.DatosOrden{
		ID: "liq-2026-03-nacional-tit-ana", ProcesoID: "prc-03",
		TitularID: "tit-ana", Periodo: "2026-03", Circuito: "nacional",
		EnviadaDia: "2026-03-01", Bruto: liqDec("1000"),
	})
	if err != nil {
		t.Fatalf("futura: %v", err)
	}
	futura.Estado = liquidacion.EstadoDiferida
	repo.ordenes = append(repo.ordenes, futura)

	repo.sembrar(metaLista("prc-02", "2026-02", reparto.Nacional), InsumoLiquidacion{
		Bruto:     liqDec("30000"),
		Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("30000")}},
	})
	vistas, err := servicio(repo, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)).
		GenerarLiquidacion(context.Background(), "prc-02")
	if err != nil {
		t.Fatalf("2026-02: %v", err)
	}
	o := vistas[0].Orden
	if !o.Neto.Equal(liqDec("30000")) {
		t.Fatalf("neto = %s; no puede absorber una diferida de periodo posterior", o.Neto)
	}
	if len(o.Arrastres) != 0 {
		t.Fatalf("arrastres = %v", o.Arrastres)
	}
	for _, o := range repo.ordenes {
		if o.ID == "liq-2026-03-nacional-tit-ana" && o.Estado != liquidacion.EstadoDiferida {
			t.Fatalf("2026-03 quedo %q; una liquidacion anterior no la cierra", o.Estado)
		}
	}
}

// TestResiduoProrrateoSeAsientaUnaVezPorLote es N1/N2: el residuo del lote
// aparece exactamente una vez en el libro, con los montos exactos, y no en
// cada asiento de orden.
func TestResiduoProrrateoSeAsientaUnaVezPorLote(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs: map[string]liquidacion.Documentos{
			"t1": {RUT: true, CertificacionBancaria: true},
			"t2": {RUT: true, CertificacionBancaria: true},
			"t3": {RUT: true, CertificacionBancaria: true},
			"t4": {RUT: true, CertificacionBancaria: true},
			"t5": {RUT: true, CertificacionBancaria: true},
			"t6": {RUT: true, CertificacionBancaria: true},
			"t7": {RUT: true, CertificacionBancaria: true},
		},
	}
	repo.sembrar(metaLista("prc-1", "2026-01", reparto.Nacional), InsumoLiquidacion{
		Bruto:   liqDec("1000"),
		Admin:   liqDec("200"),
		Social:  liqDec("100"),
		Reserva: liqDec("50"),
		Titulares: []reparto.LineaTitular{
			{TitularID: "t1", Importe: liqDec("92.86")},
			{TitularID: "t2", Importe: liqDec("92.86")},
			{TitularID: "t3", Importe: liqDec("92.86")},
			{TitularID: "t4", Importe: liqDec("92.86")},
			{TitularID: "t5", Importe: liqDec("92.86")},
			{TitularID: "t6", Importe: liqDec("92.86")},
			{TitularID: "t7", Importe: liqDec("92.84")},
		},
	})
	e := montar(repo, envio())
	if _, err := e.svc.GenerarLiquidacion(context.Background(), "prc-1"); err != nil {
		t.Fatalf("generar: %v", err)
	}

	var residuo Asiento
	nResiduos := 0
	for _, a := range e.libro.asientos {
		if a.Hecho == HechoLiquidacionResiduoProrrateo {
			nResiduos++
			residuo = a
		}
		if a.Hecho == HechoLiquidacionEmitida && strings.Contains(string(a.Payload), "residuo") {
			t.Fatalf("residuo en asiento de orden %s: %s", a.RefID, a.Payload)
		}
	}
	if nResiduos != 1 {
		t.Fatalf("%d asientos de residuo; se esperaba 1 por lote", nResiduos)
	}
	if residuo.RefTipo != RefLiquidacionLote || residuo.RefID != "2026-01-nacional" {
		t.Fatalf("ref = %s/%s", residuo.RefTipo, residuo.RefID)
	}
	cuerpo := string(residuo.Payload)
	if !strings.Contains(cuerpo, `"admin":"0.01"`) ||
		!strings.Contains(cuerpo, `"social":"-0.02"`) ||
		!strings.Contains(cuerpo, `"reserva":"0.02"`) {
		t.Fatalf("payload del residuo = %s; se esperaban admin 0.01, social -0.02, reserva 0.02", cuerpo)
	}
}
