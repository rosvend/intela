package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anticipos"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Pruebas de punta a punta de #56 contra Postgres real: el caso de uso, la
// liquidacion que dispara el flujo de RD 13.5 y el adaptador, cableados como en
// cmd/api.

func ordenDePeriodo(t *testing.T, s *Store, periodo, titularID string) liquidacion.OrdenDePago {
	t.Helper()
	ordenes, err := s.DeTitular(t.Context(), titularID)
	if err != nil {
		t.Fatalf("ordenes de %s: %v", titularID, err)
	}
	for _, o := range ordenes {
		if o.Periodo == periodo {
			return o
		}
	}
	t.Fatalf("no hay orden de %s en %s", titularID, periodo)
	return liquidacion.OrdenDePago{}
}

func renglonAnticipo(o liquidacion.OrdenDePago) (decimal.Decimal, bool) {
	for _, d := range o.Deducciones {
		if d.Concepto == liquidacion.ConceptoAnticipo {
			return d.Monto, true
		}
	}
	return decimal.Zero, false
}

// TestAnticipoDePuntaAPunta es el criterio de integracion de la issue: aprobar
// un anticipo -> la liquidacion siguiente lo descuenta -> el saldo llega a
// cero -> el pago entero vuelve.
//
// Ana es socio. Una primera corrida le liquida 4.000.000 (los derechos de la
// ventana), asi que su tope es el 25%: 1.000.000, por debajo de 5 SMMLV
// (6.500.000 con el SMMLV sembrado). Tres corridas despues:
//
//	2026-02  600.000 -> descuenta 600.000, neto 0,       saldo 400.000
//	2026-03  700.000 -> descuenta 400.000, neto 300.000, saldo 0 (descontado)
//	2026-04  500.000 -> sin descuento,     neto 500.000
func TestAnticipoDePuntaAPunta(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	uc := procesosConLiquidacion(s, s)
	svc := anticiposDe(s, reloj.Sistema{})
	socio := aplicacion.Usuario{ID: usuarioTitular, Rol: aplicacion.RolTitular, TitularID: titularAna}
	consejo := aplicacion.Usuario{ID: usuarioAdmin, Rol: aplicacion.RolAdministrador}

	liquidar := func(proceso, bolsa, periodo, importe string) {
		t.Helper()
		sembrarCorridaEnVerificacion(t, pool, proceso, bolsa, periodo, importe)
		if _, err := uc.AvanzarEtapa(ctx, proceso, usuarioDistribucion); err != nil {
			t.Fatalf("%s a liquidacion_final: %v", proceso, err)
		}
	}
	pedir := func(monto string) (aplicacion.AnticipoRegistrado, error) {
		return svc.Solicitar(ctx, socio, aplicacion.SolicitudAnticipo{
			Monto: pgDec(monto), Justificacion: "tratamiento medico no cubierto (RA 2.3.b)",
		})
	}

	liquidar("prc-0", "bolsa-0", "2026-01", "4000000")

	// R-31 contra las cifras de la base: un centavo sobre el 25% no cabe.
	if _, err := pedir("1000000.01"); !errors.Is(err, anticipos.ErrSuperaTope) {
		t.Fatalf("un centavo sobre el tope: err = %v", err)
	}
	pedido, err := pedir("1000000")
	if err != nil {
		t.Fatalf("Solicitar dentro del tope: %v", err)
	}
	// R-32: con una solicitud en tramite no se pide otra.
	if _, err := pedir("1"); !errors.Is(err, anticipos.ErrAnticipoVigente) {
		t.Fatalf("segunda solicitud en tramite: err = %v", err)
	}
	aprobado, err := svc.Aprobar(ctx, consejo, pedido.Anticipo.ID, "acta 7 de 2026, resolucion 12")
	if err != nil {
		t.Fatalf("Aprobar: %v", err)
	}
	if !aprobado.Anticipo.Saldo.Equal(pgDec("1000000")) {
		t.Fatalf("saldo al aprobar = %s", aprobado.Anticipo.Saldo)
	}
	// R-32: con saldo pendiente tampoco.
	if _, err := pedir("1"); !errors.Is(err, anticipos.ErrSaldoPendiente) {
		t.Fatalf("solicitud con saldo pendiente: err = %v", err)
	}

	pasos := []struct {
		proceso, periodo, importe string
		descuento, neto, saldo    string
		estado                    anticipos.Estado
	}{
		{"prc-1", "2026-02", "600000", "600000", "0", "400000", anticipos.EstadoAprobado},
		{"prc-2", "2026-03", "700000", "400000", "300000", "0", anticipos.EstadoDescontado},
		{"prc-3", "2026-04", "500000", "", "500000", "0", anticipos.EstadoDescontado},
	}
	for _, p := range pasos {
		liquidar(p.proceso, "bolsa-"+p.proceso, p.periodo, p.importe)
		o := ordenDePeriodo(t, s, p.periodo, titularAna)
		descuento, hay := renglonAnticipo(o)
		switch {
		case p.descuento == "" && hay:
			t.Fatalf("%s: renglon de anticipo %s con el anticipo ya saldado", p.periodo, descuento)
		case p.descuento != "" && (!hay || !descuento.Equal(pgDec(p.descuento))):
			t.Fatalf("%s: descuento = %s (hay=%v), se esperaba %s", p.periodo, descuento, hay, p.descuento)
		}
		if !o.Neto.Equal(pgDec(p.neto)) || !o.Bruto.Equal(pgDec(p.importe)) {
			t.Fatalf("%s: bruto %s neto %s, se esperaba bruto %s neto %s", p.periodo, o.Bruto, o.Neto, p.importe, p.neto)
		}
		despues, err := s.Anticipos().PorID(ctx, pedido.Anticipo.ID)
		if err != nil {
			t.Fatalf("PorID: %v", err)
		}
		if !despues.Anticipo.Saldo.Equal(pgDec(p.saldo)) || despues.Anticipo.Estado != p.estado {
			t.Fatalf("%s: anticipo = %s/%s, se esperaba %s/%s",
				p.periodo, despues.Anticipo.Estado, despues.Anticipo.Saldo, p.estado, p.saldo)
		}
	}

	// El libro del anticipo (ADR 0006): solicitud, aprobacion y un descuento
	// por cada orden que lo cubrio, con la orden y el monto.
	libro, err := s.De(ctx, aplicacion.RefAnticipo, pedido.Anticipo.ID)
	if err != nil {
		t.Fatalf("asientos del anticipo: %v", err)
	}
	var hechos []string
	descuentos := map[string]string{}
	for _, a := range libro {
		hechos = append(hechos, a.Hecho)
		if a.Hecho != aplicacion.HechoAnticipoDescontado {
			continue
		}
		var p aplicacion.AsientoAnticipo
		if err := json.Unmarshal(a.Payload, &p); err != nil {
			t.Fatalf("payload del descuento: %v", err)
		}
		descuentos[p.OrdenID] = p.Descontado
	}
	quiero := []string{
		aplicacion.HechoAnticipoSolicitado, aplicacion.HechoAnticipoAprobado,
		aplicacion.HechoAnticipoDescontado, aplicacion.HechoAnticipoDescontado,
	}
	if len(hechos) != len(quiero) {
		t.Fatalf("hechos del anticipo = %v, se esperaba %v", hechos, quiero)
	}
	for i := range quiero {
		if hechos[i] != quiero[i] {
			t.Fatalf("hechos del anticipo = %v, se esperaba %v", hechos, quiero)
		}
	}
	if descuentos["liq-2026-02-nacional-"+titularAna] != "600000.00" ||
		descuentos["liq-2026-03-nacional-"+titularAna] != "400000.00" {
		t.Fatalf("descuentos asentados = %v", descuentos)
	}

	// Saldado, el socio puede volver a pedir (R-32 ya no lo bloquea).
	if _, err := pedir("1"); err != nil {
		t.Fatalf("solicitud despues de saldar el anticipo: %v", err)
	}
}

// TestUnTitularAdministradoNoPideAnticipoAunqueTengaDerechos es R-30 contra la
// base: Beto figura como `administrado` en el padron, y aunque tenga derechos
// liquidados la solicitud no se registra.
func TestUnTitularAdministradoNoPideAnticipoAunqueTengaDerechos(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	ejecutorDePruebas(t, pool)(`INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash)
	          VALUES ('usr-beto', 'beto@redes.co', 'Beto', 'titular', $1, $2)`, titularBeto, hashBcrypt)
	sembrarCorridaEnVerificacionTitular(t, pool, "prc-0", "bolsa-0", "2026-01", "4000000", titularBeto)
	if _, err := procesosConLiquidacion(s, s).AvanzarEtapa(ctx, "prc-0", usuarioDistribucion); err != nil {
		t.Fatalf("liquidar a Beto: %v", err)
	}

	beto := aplicacion.Usuario{ID: "usr-beto", Rol: aplicacion.RolTitular, TitularID: titularBeto}
	_, err := anticiposDe(s, reloj.Sistema{}).Solicitar(ctx, beto,
		aplicacion.SolicitudAnticipo{Monto: pgDec("1000"), Justificacion: "x"})
	if !errors.Is(err, anticipos.ErrNoEsSocio) {
		t.Fatalf("err = %v, se esperaba ErrNoEsSocio (R-30)", err)
	}
	if lista, err := s.Anticipos().DeTitular(ctx, titularBeto); err != nil || len(lista) != 0 {
		t.Fatalf("anticipos de Beto = %v, %v; un rechazo no deja la solicitud", lista, err)
	}
}

// TestUnEmbargoReportadoImpideAprobar es R-32 (RA 3.1.e) contra la base: el
// oficio llega entre la solicitud y la aprobacion, y lo que el reglamento
// prohibe es desembolsar.
func TestUnEmbargoReportadoImpideAprobar(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	sembrarCorridaEnVerificacion(t, pool, "prc-0", "bolsa-0", "2026-01", "4000000")
	if _, err := procesosConLiquidacion(s, s).AvanzarEtapa(ctx, "prc-0", usuarioDistribucion); err != nil {
		t.Fatalf("liquidar a Ana: %v", err)
	}
	svc := anticiposDe(s, reloj.Sistema{})
	socio := aplicacion.Usuario{ID: usuarioTitular, Rol: aplicacion.RolTitular, TitularID: titularAna}
	pedido, err := svc.Solicitar(ctx, socio, aplicacion.SolicitudAnticipo{Monto: pgDec("1000"), Justificacion: "x"})
	if err != nil {
		t.Fatalf("Solicitar: %v", err)
	}

	ejecutorDePruebas(t, pool)(`INSERT INTO embargos (titular_id, reportado, soporte)
	          VALUES ($1, CURRENT_DATE - 1, 'oficio juzgado 3')`, titularAna)

	consejo := aplicacion.Usuario{ID: usuarioAdmin, Rol: aplicacion.RolAdministrador}
	if _, err := svc.Aprobar(ctx, consejo, pedido.Anticipo.ID, "acta 7"); !errors.Is(err, anticipos.ErrEmbargoReportado) {
		t.Fatalf("aprobar con embargo: err = %v, se esperaba ErrEmbargoReportado", err)
	}
	got, err := s.Anticipos().PorID(ctx, pedido.Anticipo.ID)
	if err != nil || got.Anticipo.Estado != anticipos.EstadoSolicitado || !got.Anticipo.Saldo.IsZero() {
		t.Fatalf("anticipo = %+v, %v; una aprobacion negada no gira nada", got, err)
	}
	if _, err := svc.Solicitar(ctx, socio, aplicacion.SolicitudAnticipo{Monto: pgDec("1"), Justificacion: "x"}); !errors.Is(err, anticipos.ErrEmbargoReportado) {
		t.Fatalf("pedir con embargo: err = %v", err)
	}
}

// esperarCerrojoSobreAnticipos espera, consultando pg_stat_activity, a que otra
// sesion quede bloqueada por un cerrojo en una sentencia sobre `anticipos`.
func esperarCerrojoSobreAnticipos(t *testing.T, vigia *pgx.Conn, ctx context.Context) {
	t.Helper()
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		var bloqueada bool
		if err := vigia.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM pg_stat_activity
			    WHERE wait_event_type = 'Lock' AND query ILIKE '%anticipos%')`).Scan(&bloqueada); err != nil {
			t.Fatalf("consultar pg_stat_activity: %v", err)
		}
		if bloqueada {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("T2 nunca quedo esperando el cerrojo de T1 sobre el anticipo")
}

// TestDosDescuentosALaVezNoCobranDosVecesElMismoSaldo: dos liquidaciones del
// mismo socio en periodos distintos NO comparten el cerrojo de aviso del
// periodo, asi que lo que las serializa es el FOR UPDATE de PorDescontar.
//
// Se fuerza la ventana exacta, como en la prueba de concurrencia del catalogo:
// T1 descuenta 600 de un saldo de 1000 y se pausa ANTES de asentar (fila
// escrita y bloqueada, sin confirmar); T2 arranca ahi y el test espera, contra
// el servidor, a que este bloqueada. Al soltar a T1, T2 tiene que leer el saldo
// que T1 dejo (400) y descontar 400, no 600: con 1000 de deuda el socio paga
// 1000, ni un peso mas.
//
// Sin el cerrojo T2 leeria 1000 sin confirmar y descontaria 600 sobre el; si
// ademas Descontar no comparara el saldo anterior, escribiria 400 encima del
// 400 de T1 y el socio habria pagado 1200 y seguiria debiendo 400.
func TestDosDescuentosALaVezNoCobranDosVecesElMismoSaldo(t *testing.T) {
	cfg, err := pgxpool.ParseConfig(testhelp.DSN(t))
	if err != nil {
		t.Fatalf("configurar el pool concurrente: %v", err)
	}
	cfg.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("abrir el pool concurrente: %v", err)
	}
	t.Cleanup(pool.Close)
	s := sembrarEn(t, pool)
	ctx := t.Context()
	sembrarAprobado(t, s, "ant-1", titularAna, "1000")

	vigia, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		t.Fatalf("abrir conexion de vigilancia: %v", err)
	}
	defer func() { _ = vigia.Close(ctx) }()

	listoT1 := make(chan struct{})
	liberarT1 := make(chan struct{})
	var enVuelo sync.WaitGroup
	liberarUnaVez := sync.OnceFunc(func() { close(liberarT1) })
	defer enVuelo.Wait()
	defer liberarUnaVez()

	recuperar := func(svc aplicacion.Anticipos, orden string, descuento *decimal.Decimal) error {
		return s.EnUnidad(ctx, func(ctx context.Context) error {
			d, err := svc.Recuperar(ctx, titularAna, orden, pgDec("600"))
			*descuento = d
			return err
		})
	}

	t1 := anticiposDe(s, reloj.Sistema{})
	t1.Bitacora = &bitacoraConPausa{Store: s, listo: listoT1, seguir: liberarT1}
	var d1, d2 decimal.Decimal
	fin1, fin2 := make(chan error, 1), make(chan error, 1)
	enVuelo.Add(1)
	go func() {
		defer enVuelo.Done()
		fin1 <- recuperar(t1, "liq-2026-02-nacional-tit-ana", &d1)
	}()
	select {
	case <-listoT1:
	case <-time.After(10 * time.Second):
		t.Fatal("T1 nunca llego a asentar su descuento")
	}

	enVuelo.Add(1)
	go func() {
		defer enVuelo.Done()
		fin2 <- recuperar(anticiposDe(s, reloj.Sistema{}), "liq-2026-03-nacional-tit-ana", &d2)
	}()
	esperarCerrojoSobreAnticipos(t, vigia, ctx)

	liberarUnaVez()
	if err := <-fin1; err != nil {
		t.Fatalf("descuento de T1: %v", err)
	}
	if err := <-fin2; err != nil {
		t.Fatalf("descuento de T2: %v", err)
	}
	if !d1.Equal(pgDec("600")) || !d2.Equal(pgDec("400")) {
		t.Fatalf("descuentos = %s y %s, se esperaba 600 y 400: T2 tiene que ver el saldo que dejo T1", d1, d2)
	}
	got, err := s.Anticipos().PorID(ctx, "ant-1")
	if err != nil {
		t.Fatalf("PorID: %v", err)
	}
	if !got.Anticipo.Saldo.IsZero() || got.Anticipo.Estado != anticipos.EstadoDescontado {
		t.Fatalf("anticipo = %s/%s; con 1000 descontados tiene que quedar saldado", got.Anticipo.Estado, got.Anticipo.Saldo)
	}
}
