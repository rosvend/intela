package aplicacion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

var _ RepositorioLiquidacion = (*repoLiqMemoria)(nil)

type repoLiqMemoria struct {
	ordenes   []liquidacion.OrdenDePago
	insumo    InsumoLiquidacion
	insumoErr error
	smmlv     decimal.Decimal
	smmlvErr  error
	docs      map[string]liquidacion.Documentos
	guardadas int
}

func (r *repoLiqMemoria) DeTitular(_ context.Context, titularID string) ([]liquidacion.OrdenDePago, error) {
	var out []liquidacion.OrdenDePago
	for _, o := range r.ordenes {
		if o.TitularID == titularID {
			out = append(out, o)
		}
	}
	if out == nil {
		out = []liquidacion.OrdenDePago{}
	}
	return out, nil
}

func (r *repoLiqMemoria) Listar(context.Context) ([]liquidacion.OrdenDePago, error) {
	out := append([]liquidacion.OrdenDePago{}, r.ordenes...)
	return out, nil
}

func (r *repoLiqMemoria) DeProceso(_ context.Context, procesoID string) ([]liquidacion.OrdenDePago, error) {
	var out []liquidacion.OrdenDePago
	for _, o := range r.ordenes {
		if o.ProcesoID == procesoID {
			out = append(out, o)
		}
	}
	if out == nil {
		out = []liquidacion.OrdenDePago{}
	}
	return out, nil
}

func (r *repoLiqMemoria) Guardar(_ context.Context, ordenes []liquidacion.OrdenDePago) error {
	r.guardadas++
	porID := map[string]int{}
	for i, o := range r.ordenes {
		porID[o.ID] = i
	}
	for _, o := range ordenes {
		if i, ok := porID[o.ID]; ok {
			r.ordenes[i] = o
			continue
		}
		porID[o.ID] = len(r.ordenes)
		r.ordenes = append(r.ordenes, o)
	}
	return nil
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

func (r *repoLiqMemoria) InsumoDeProceso(context.Context, string) (InsumoLiquidacion, error) {
	return r.insumo, r.insumoErr
}

func (r *repoLiqMemoria) SMMLVVigente(context.Context, time.Time) (decimal.Decimal, error) {
	return r.smmlv, r.smmlvErr
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

func servicio(repo *repoLiqMemoria, instante time.Time) Liquidaciones {
	return Liquidaciones{Ordenes: repo, Reloj: relojFijo{instante: instante}}
}

func insumoDosTitulares() InsumoLiquidacion {
	return InsumoLiquidacion{
		ProcesoID: "prc-1",
		Periodo:   "2026",
		Bruto:     liqDec("1000"),
		Admin:     liqDec("200"),
		Social:    liqDec("100"),
		Reserva:   liqDec("50"),
		Titulares: []reparto.LineaTitular{
			{ObraID: "obra-a", TitularID: "tit-ana", IPI: "1", Porcentaje: liqDec("60"), Importe: liqDec("390")},
			{ObraID: "obra-b", TitularID: "tit-ana", IPI: "1", Porcentaje: liqDec("100"), Importe: liqDec("260")},
			{ObraID: "obra-a", TitularID: "tit-beto", IPI: "2", Porcentaje: liqDec("40"), Importe: liqDec("260")},
		},
	}
}

func TestGenerarLiquidacionItemizaDeduccionesPorTitular(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv:  liqDec("1300000"),
		insumo: insumoDosTitulares(),
		docs: map[string]liquidacion.Documentos{
			"tit-ana":  {RUT: true, CertificacionBancaria: true},
			"tit-beto": {RUT: true, CertificacionBancaria: true},
		},
	}

	vistas, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	if len(vistas) != 2 {
		t.Fatalf("se esperaban 2 ordenes (una por titular), llegaron %d", len(vistas))
	}

	porTitular := map[string]OrdenVista{}
	for _, v := range vistas {
		porTitular[v.Orden.TitularID] = v
	}

	ana := porTitular["tit-ana"].Orden
	if !ana.Neto.Equal(liqDec("650")) {
		t.Fatalf("neto ana = %s, se esperaba 650 (390+260)", ana.Neto)
	}
	if len(ana.Deducciones) != 3 {
		t.Fatalf("ana: %d deducciones, se esperaban 3 itemizadas", len(ana.Deducciones))
	}
	// 650/910 de cada deduccion de bolsa.
	if !ana.Deducciones[0].Monto.Equal(liqDec("142.86")) { // 200 * 650/910
		t.Fatalf("admin ana = %s", ana.Deducciones[0].Monto)
	}
	if ana.Deducciones[0].Concepto != liquidacion.ConceptoAdministracion {
		t.Fatalf("concepto admin = %q", ana.Deducciones[0].Concepto)
	}
	if !ana.Bruto.Equal(ana.Neto.Add(ana.Deducciones[0].Monto).Add(ana.Deducciones[1].Monto).Add(ana.Deducciones[2].Monto)) {
		t.Fatalf("bruto ana %s no es neto + deducciones", ana.Bruto)
	}

	beto := porTitular["tit-beto"].Orden
	if !beto.Neto.Equal(liqDec("260")) {
		t.Fatalf("neto beto = %s", beto.Neto)
	}
	if ana.Estado != liquidacion.EstadoEnviada || beto.Estado != liquidacion.EstadoEnviada {
		t.Fatal("recien generadas tienen que estar enviadas")
	}
}

func TestGenerarLiquidacionEsIdempotente(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv:  liqDec("1300000"),
		insumo: insumoDosTitulares(),
		docs:   map[string]liquidacion.Documentos{},
	}
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
}

func TestSilencioALos15DiasConRelojFijo(t *testing.T) {
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		insumo: InsumoLiquidacion{
			ProcesoID: "prc-1",
			Periodo:   "2026",
			Bruto:     liqDec("100000"),
			Titulares: []reparto.LineaTitular{
				{TitularID: "tit-ana", Importe: liqDec("100000")},
			},
		},
		docs: map[string]liquidacion.Documentos{
			"tit-ana": {RUT: true, CertificacionBancaria: true},
		},
	}

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
	docs := map[string]liquidacion.Documentos{
		"tit-ana": {RUT: true, CertificacionBancaria: true},
	}
	repo := &repoLiqMemoria{
		smmlv: liqDec("1300000"),
		docs:  docs,
		insumo: InsumoLiquidacion{
			ProcesoID: "prc-1",
			Periodo:   "2026-1",
			Bruto:     liqDec("1000"),
			Titulares: []reparto.LineaTitular{
				{TitularID: "tit-ana", Importe: liqDec("1000")},
			},
		},
	}

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

	repo.insumo = InsumoLiquidacion{
		ProcesoID: "prc-2",
		Periodo:   "2026-2",
		Bruto:     liqDec("30000"),
		Titulares: []reparto.LineaTitular{
			{TitularID: "tit-ana", Importe: liqDec("30000")},
		},
	}
	envio2 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	vistas, err = servicio(repo, envio2).GenerarLiquidacion(context.Background(), "prc-2")
	if err != nil {
		t.Fatalf("periodo 2: %v", err)
	}
	if len(vistas) != 1 {
		t.Fatalf("periodo 2 produjo %d ordenes", len(vistas))
	}
	if !vistas[0].Orden.Neto.Equal(liqDec("31000")) {
		t.Fatalf("neto periodo 2 = %s, se esperaba 31000 (30000+1000)", vistas[0].Orden.Neto)
	}

	var p1 liquidacion.OrdenDePago
	for _, o := range repo.ordenes {
		if o.ProcesoID == "prc-1" {
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
		if v.Orden.ProcesoID == "prc-2" {
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
		insumo: InsumoLiquidacion{
			ProcesoID: "prc-1",
			Periodo:   "2026",
			Bruto:     liqDec("100000"),
			Titulares: []reparto.LineaTitular{
				{TitularID: "tit-ana", Importe: liqDec("100000")},
			},
		},
		docs: map[string]liquidacion.Documentos{
			"tit-ana": {RUT: true}, // sin certificacion bancaria
		},
	}
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
	repo := &repoLiqMemoria{
		smmlvErr: ErrParametroAusente,
		insumo: InsumoLiquidacion{
			ProcesoID: "prc-1",
			Periodo:   "2026",
			Titulares: []reparto.LineaTitular{{TitularID: "tit-ana", Importe: liqDec("1")}},
		},
	}
	_, err := servicio(repo, envio()).GenerarLiquidacion(context.Background(), "prc-1")
	if !errors.Is(err, ErrParametroAusente) {
		t.Fatalf("se esperaba ErrParametroAusente, se obtuvo %v", err)
	}
}
