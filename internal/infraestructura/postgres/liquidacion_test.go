package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// notificadorDePruebas satisface [aplicacion.Notificador] con un acuse
// determinista. Se declara aqui y no se reutiliza el adaptador de
// `infraestructura/notificaciones` para que estas pruebas no aten el adaptador
// de persistencia a otro adaptador: lo que se comprueba es que el caso de uso
// NOTIFICA antes de dejar la orden en `enviada`, no como se notifica.
type notificadorDePruebas struct{}

func (notificadorDePruebas) Notificar(_ context.Context, dest, asunto, _ string) (string, error) {
	return "acuse-" + dest + "-" + asunto, nil
}

const (
	procesoNac = "prc-nac-2026"
	bolsaNac   = "bolsa-nac-2026"

	// Las dos firmas de la compuerta del RD 13.5 las tienen que poner dos
	// actores DISTINTOS: `firma_actor_unico_por_revision` existe para que un
	// solo usuario no cubra los dos roles.
	usuarioDistribucion = "usr-distribucion"
	usuarioContabilidad = "usr-contabilidad"
)

func pgDec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// liquidacionesDe cablea el caso de uso como lo hace cmd/api: el mismo *Store
// satisface el repositorio, la bitacora y la unidad de trabajo, y el
// notificador de desarrollo devuelve un acuse determinista.
func liquidacionesDe(s *Store, instante time.Time) *aplicacion.Liquidaciones {
	return &aplicacion.Liquidaciones{
		Ordenes:     s,
		Reloj:       reloj.Fijo{Instante: instante},
		Notificador: notificadorDePruebas{},
		Bitacora:    s,
		Unidad:      s,
	}
}

// sembrarCorrida deja un proceso nacional con resultado y lineas de
// titular, el SMMLV vigente y, si conDocs, RUT + banco de Ana.
func sembrarCorrida(t *testing.T, conDocs bool) (*Store, *aplicacion.Liquidaciones) {
	t.Helper()
	s, pool := sembrar(t)

	sembrarFirmantes(t, pool)
	sembrarBolsa(t, pool, bolsaNac, "2026", "nacional")
	sembrarProcesoListo(t, pool, procesoNac, bolsaNac, "2026", "nacional")
	sembrarResultado(t, pool, procesoNac, "1000000", "200000", "100000", "50000")

	ejecutar := ejecutorDePruebas(t, pool)
	ejecutar(`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
	          VALUES ($1, $2, 10, 650000, FALSE)`, procesoNac, obraCompleta)
	// 60/40 de la obra completa: 390000 y 260000, suman el neto 650000.
	// Ambos superan el 2% de 1_300_000 (26000), para que el silencio sea
	// R-10 y no el arrastre de R-11.
	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
	          VALUES ($1, $2, $3, 'IPI-00000001', 60, 390000),
	                 ($1, $2, $4, 'IPI-00000002', 40, 260000)`,
		procesoNac, obraCompleta, titularAna, titularBeto)

	sembrarSMMLV(t, pool)

	if conDocs {
		ejecutar(`INSERT INTO documentos_titular (titular_id, tipo, clave_objeto)
		          VALUES ($1, 'rut', 'docs/ana/rut'),
		                 ($1, 'certificacion_bancaria', 'docs/ana/banco')`, titularAna)
	}

	return s, liquidacionesDe(s, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
}

func ejecutorDePruebas(t *testing.T, pool *pgxpool.Pool) func(string, ...any) {
	t.Helper()
	return func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("sembrar corrida (%s): %v", sql, err)
		}
	}
}

// sembrarFirmantes crea los dos usuarios que firman la compuerta. Sin ellos no
// hay firmas posibles: `firmas.actor_id` referencia `usuarios(id)`.
func sembrarFirmantes(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ejecutar := ejecutorDePruebas(t, pool)
	ejecutar(`INSERT INTO usuarios (id, email, nombre, rol, password_hash)
	          VALUES ($1, 'dist@redes.co', 'Distribucion', 'distribucion', $3),
	                 ($2, 'conta@redes.co', 'Contabilidad', 'contabilidad', $3)`,
		usuarioDistribucion, usuarioContabilidad, hashBcrypt)
}

// sembrarBolsa crea una bolsa con su PROPIO pagador.
//
// Uno por bolsa y no uno compartido: `bolsas` lleva
// UNIQUE (usuario_id, periodo, circuito) desde la 00009, asi que dos bolsas del
// mismo periodo y circuito -- que es justo lo que agrega el ADR 0019 -- no
// pueden ser del mismo pagador.
func sembrarBolsa(t *testing.T, pool *pgxpool.Pool, bolsaID, periodo, circuito string) {
	t.Helper()
	ejecutar := ejecutorDePruebas(t, pool)
	// bolsas.usuario_id referencia usuarios_recaudo (#27), no usuarios.
	pagador := "usr-" + bolsaID
	ejecutar(`INSERT INTO usuarios_recaudo (id, nombre, nit, categoria)
	          VALUES ($1, 'Pagador de ' || $1, '', 'tv_abierta')`, pagador)
	ejecutar(`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
	          VALUES ($1, $2, $3, $4, 1000000)`, bolsaID, pagador, periodo, circuito)
}

// sembrarProcesoListo deja una corrida que YA paso la compuerta del RD 13.5:
// etapa liquidacion_final y las dos firmas sobre la revision vigente.
//
// Las firmas no son decoracion del fixture: desde el gate de
// GenerarLiquidacion, una corrida sin ellas no liquida, y sembrarlas es lo que
// separa "esta prueba mira otra cosa" de "esta prueba mira la compuerta".
func sembrarProcesoListo(t *testing.T, pool *pgxpool.Pool, procesoID, bolsaID, periodo, circuito string) {
	t.Helper()
	ejecutar := ejecutorDePruebas(t, pool)
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
	          VALUES ($1, $4, 'liquidacion_final', $2, $3, 'snap-1', 'RD-IX')`,
		procesoID, periodo, bolsaID, circuito)
	ejecutar(`INSERT INTO firmas (proceso_id, rol, revision, actor_id)
	          VALUES ($1, 'distribucion', 1, $2),
	                 ($1, 'contabilidad', 1, $3)`,
		procesoID, usuarioDistribucion, usuarioContabilidad)
}

func sembrarResultado(t *testing.T, pool *pgxpool.Pool, procesoID, bruto, admin, social, reserva string) {
	t.Helper()
	neto := pgDec(bruto).Sub(pgDec(admin)).Sub(pgDec(social)).Sub(pgDec(reserva))
	ejecutorDePruebas(t, pool)(`INSERT INTO resultados_proceso
	            (proceso_id, bruto, admin, social, reserva, neto, retenido, residuo,
	             valor_punto, snapshot_id, reglamento)
	          VALUES ($1, $2, $3, $4, $5, $6, 0, 0, 1, 'snap-1', 'RD-IX')`,
		procesoID, pgDec(bruto), pgDec(admin), pgDec(social), pgDec(reserva), neto)
}

func sembrarSMMLV(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ejecutorDePruebas(t, pool)(`INSERT INTO parametros (clave, valor, vigente_desde, organo, reglamento)
	          VALUES ('smmlv', 1300000, '2026-01-01', 'Gobierno Nacional', 'Decreto SMMLV 2026')`)
}

func TestGenerarLiquidacionDesdeResultadoYDeTitular(t *testing.T) {
	s, liq := sembrarCorrida(t, true)
	ctx := t.Context()

	vistas, err := liq.GenerarLiquidacion(ctx, procesoNac)
	if err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}
	if len(vistas) != 2 {
		t.Fatalf("se esperaban 2 ordenes, llegaron %d", len(vistas))
	}

	ana, err := s.DeTitular(ctx, titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(ana) != 1 {
		t.Fatalf("Ana tenia %d ordenes, se esperaba 1", len(ana))
	}
	if !ana[0].Neto.Equal(pgDec("390000")) {
		t.Fatalf("neto ana = %s, se esperaba 390000", ana[0].Neto)
	}
	if ana[0].ID != "liq-2026-nacional-"+titularAna {
		t.Fatalf("id = %q; la clave de una orden es (periodo, circuito, titular)", ana[0].ID)
	}
	if ana[0].Circuito != string(reparto.Nacional) {
		t.Fatalf("circuito = %q", ana[0].Circuito)
	}
	if len(ana[0].Procesos) != 1 || ana[0].Procesos[0] != procesoNac {
		t.Fatalf("Procesos = %v", ana[0].Procesos)
	}
	if len(ana[0].Deducciones) != 3 {
		t.Fatalf("ana: %d deducciones, el desglose tiene que persistirse", len(ana[0].Deducciones))
	}
	if ana[0].Estado != liquidacion.EstadoEnviada {
		t.Fatalf("Estado = %q", ana[0].Estado)
	}
	if ana[0].EnviadaDia != "2026-01-01" {
		t.Fatalf("EnviadaDia = %q", ana[0].EnviadaDia)
	}

	// El ADR 0006 pide un asiento por hecho, y en la MISMA transaccion que la
	// orden: si la orden esta, su asiento esta.
	asientos, err := s.De(ctx, aplicacion.RefOrdenDePago, ana[0].ID)
	if err != nil {
		t.Fatalf("asientos de la orden: %v", err)
	}
	if len(asientos) == 0 {
		t.Fatal("una orden emitida sin asiento no esta hecha (ADR 0006)")
	}
	if asientos[0].Hecho != aplicacion.HechoLiquidacionEmitida {
		t.Fatalf("hecho = %q", asientos[0].Hecho)
	}

	beto, err := s.DeTitular(ctx, titularBeto)
	if err != nil {
		t.Fatalf("DeTitular beto: %v", err)
	}
	if len(beto) != 1 || !beto[0].Neto.Equal(pgDec("260000")) {
		t.Fatalf("beto = %+v", beto)
	}
}

// TestGenerarLiquidacionExigeLaCompuertaDelRD135 es el gate.
//
// Una corrida en verificacion y sin firmas no liquida, y no deja nada escrito:
// emitir ordenes de pago es lo que hace salir el dinero, y una llamada que
// devuelve 200 sobre una corrida a medio verificar es el defecto mas caro de
// este modulo.
func TestGenerarLiquidacionExigeLaCompuertaDelRD135(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	sembrarBolsa(t, pool, bolsaNac, "2026", "nacional")
	ejecutorDePruebas(t, pool)(
		`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
		 VALUES ($1, 'nacional', 'verificacion', '2026', $2, 'snap-1', 'RD-IX')`,
		procesoNac, bolsaNac)
	sembrarResultado(t, pool, procesoNac, "1000000", "200000", "100000", "50000")
	ejecutorDePruebas(t, pool)(
		`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
		 VALUES ($1, $2, 10, 650000, FALSE)`, procesoNac, obraCompleta)
	ejecutorDePruebas(t, pool)(
		`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
		 VALUES ($1, $2, $3, 'IPI-00000001', 100, 650000)`,
		procesoNac, obraCompleta, titularAna)
	sembrarSMMLV(t, pool)

	liq := liquidacionesDe(s, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	_, err := liq.GenerarLiquidacion(ctx, procesoNac)
	if !errors.Is(err, aplicacion.ErrProcesoNoListo) {
		t.Fatalf("se esperaba ErrProcesoNoListo, se obtuvo %v", err)
	}
	ordenes, err := s.Listar(ctx)
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(ordenes) != 0 {
		t.Fatalf("%d ordenes emitidas de una corrida sin compuerta", len(ordenes))
	}

	// Avanzar la etapa NO basta: faltan las dos firmas de la revision.
	ejecutorDePruebas(t, pool)(`UPDATE procesos SET etapa = 'liquidacion_final' WHERE id = $1`, procesoNac)
	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); !errors.Is(err, aplicacion.ErrProcesoNoListo) {
		t.Fatalf("sin firmas: se esperaba ErrProcesoNoListo, se obtuvo %v", err)
	}

	// Con una sola firma tampoco: la doble firma es el control.
	sembrarFirmantes(t, pool)
	ejecutorDePruebas(t, pool)(
		`INSERT INTO firmas (proceso_id, rol, revision, actor_id) VALUES ($1, 'distribucion', 1, $2)`,
		procesoNac, usuarioDistribucion)
	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); !errors.Is(err, aplicacion.ErrProcesoNoListo) {
		t.Fatalf("con una firma: se esperaba ErrProcesoNoListo, se obtuvo %v", err)
	}

	ejecutorDePruebas(t, pool)(
		`INSERT INTO firmas (proceso_id, rol, revision, actor_id) VALUES ($1, 'contabilidad', 1, $2)`,
		procesoNac, usuarioContabilidad)
	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); err != nil {
		t.Fatalf("con las dos firmas tiene que liquidar: %v", err)
	}
}

// TestGenerarLiquidacionAgregaLasDosCorridasDelPeriodo es el ADR 0019 contra la
// base: dos bolsas nacionales del mismo periodo producen UNA orden por titular,
// y el UNIQUE (titular_id, periodo, circuito) es lo que lo sostiene.
func TestGenerarLiquidacionAgregaLasDosCorridasDelPeriodo(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	for _, c := range []struct{ proceso, bolsa, importe string }{
		{proceso: "prc-nac-a", bolsa: "bolsa-a", importe: "390000"},
		{proceso: "prc-nac-b", bolsa: "bolsa-b", importe: "260000"},
	} {
		sembrarBolsa(t, pool, c.bolsa, "2026", "nacional")
		sembrarProcesoListo(t, pool, c.proceso, c.bolsa, "2026", "nacional")
		sembrarResultado(t, pool, c.proceso, "1000000", "200000", "100000", "50000")
		ejecutorDePruebas(t, pool)(
			`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
			 VALUES ($1, $2, 10, $3, FALSE)`, c.proceso, obraCompleta, pgDec(c.importe))
		ejecutorDePruebas(t, pool)(
			`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
			 VALUES ($1, $2, $3, 'IPI-00000001', 100, $4)`,
			c.proceso, obraCompleta, titularAna, pgDec(c.importe))
	}

	liq := liquidacionesDe(s, time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	// Se dispara con la segunda: procesoID es el disparador, no el alcance.
	if _, err := liq.GenerarLiquidacion(ctx, "prc-nac-b"); err != nil {
		t.Fatalf("GenerarLiquidacion: %v", err)
	}

	ana, err := s.DeTitular(ctx, titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(ana) != 1 {
		t.Fatalf("Ana tiene %d ordenes; el ADR 0019 pide una por (titular, periodo, circuito)", len(ana))
	}
	if !ana[0].Neto.Equal(pgDec("650000")) {
		t.Fatalf("neto = %s, se esperaban 650000 (390000+260000 de las dos bolsas)", ana[0].Neto)
	}
	if len(ana[0].Procesos) != 2 {
		t.Fatalf("Procesos = %v, se esperaban las dos corridas", ana[0].Procesos)
	}

	// Y las dos corridas llevan a la misma orden desde DeProceso, que es como
	// se audita de donde salio.
	for _, procesoID := range []string{"prc-nac-a", "prc-nac-b"} {
		ordenes, err := s.DeProceso(ctx, procesoID)
		if err != nil {
			t.Fatalf("DeProceso %s: %v", procesoID, err)
		}
		if len(ordenes) != 1 || ordenes[0].ID != ana[0].ID {
			t.Fatalf("DeProceso %s = %+v", procesoID, ordenes)
		}
	}
}

// repoConCita es el *Store con un punto de cita ANTES de tomar el cerrojo del
// periodo.
//
// Existe para que las pruebas de concurrencia sean deterministas sin tocar el
// codigo de produccion: la cita se pone en el unico sitio que sirve -- la
// primera operacion de la unidad --, asi que las dos goroutines entran en el
// tramo critico a la vez en vez de depender de como las planifique el runtime.
// Sin ella, una prueba de carrera pasa por suerte el 99% de las veces y no
// distingue el codigo con cerrojo del codigo sin el.
type repoConCita struct {
	*Store
	cita func()
}

func (r repoConCita) BloquearPeriodo(ctx context.Context, periodo string, circuito reparto.Circuito) error {
	r.cita()
	return r.Store.BloquearPeriodo(ctx, periodo, circuito)
}

// citaDe devuelve una funcion que bloquea hasta que n goroutines la hayan
// llamado.
func citaDe(n int) func() {
	var (
		mu       sync.Mutex
		llegadas int
		listo    = make(chan struct{})
	)
	return func() {
		mu.Lock()
		llegadas++
		ultima := llegadas == n
		mu.Unlock()
		if ultima {
			close(listo)
			return
		}
		select {
		case <-listo:
		case <-time.After(10 * time.Second):
		}
	}
}

// TestArrastreNoSePagaDosVecesConGeneracionesConcurrentes es B3a.
//
// Ana tiene una diferida de 1000 (R-11) y dos periodos nuevos de 30000 cada
// uno se liquidan A LA VEZ. El arrastre es UNO, asi que la suma de los netos
// vivos tiene que ser 61000 y no 62000.
//
// Los dos periodos son distintos, asi que NO comparten el cerrojo de aviso de
// BloquearPeriodo: lo que impide el doble pago aqui es el FOR UPDATE de
// DiferidasDeTitular. Sin el, las dos transacciones leen la misma fila como
// diferida, cada una se la suma y cada una la marca acumulada, y el saldo sale
// dos veces sin que nada lo registre.
func TestArrastreNoSePagaDosVecesConGeneracionesConcurrentes(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)

	// La diferida de partida: 1000 no llega al 2% de un SMMLV.
	sembrarBolsa(t, pool, "bolsa-00", "2026-01", "nacional")
	sembrarProcesoListo(t, pool, "prc-00", "bolsa-00", "2026-01", "nacional")
	ejecutorDePruebas(t, pool)(
		`INSERT INTO ordenes_pago
		   (id, proceso_id, procesos, titular_id, periodo, circuito,
		    bruto, neto, estado, enviada, arrastres)
		 VALUES ('liq-2026-01-nacional-'||$1, 'prc-00', ARRAY['prc-00'], $1, '2026-01',
		         'nacional', 1000, 1000, 'diferida', '2026-01-01', '{}')`, titularAna)

	for _, c := range []struct{ proceso, bolsa, periodo string }{
		{proceso: "prc-02", bolsa: "bolsa-02", periodo: "2026-02"},
		{proceso: "prc-03", bolsa: "bolsa-03", periodo: "2026-03"},
	} {
		sembrarBolsa(t, pool, c.bolsa, c.periodo, "nacional")
		sembrarProcesoListo(t, pool, c.proceso, c.bolsa, c.periodo, "nacional")
		sembrarResultado(t, pool, c.proceso, "30000", "0", "0", "0")
		ejecutorDePruebas(t, pool)(
			`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
			 VALUES ($1, $2, 10, 30000, FALSE)`, c.proceso, obraCompleta)
		ejecutorDePruebas(t, pool)(
			`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
			 VALUES ($1, $2, $3, 'IPI-00000001', 100, 30000)`,
			c.proceso, obraCompleta, titularAna)
	}

	cita := citaDe(2)
	generar := func(procesoID string) error {
		liq := liquidacionesDe(s, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
		liq.Ordenes = repoConCita{Store: s, cita: cita}
		_, err := liq.GenerarLiquidacion(ctx, procesoID)
		return err
	}

	var (
		grupo  sync.WaitGroup
		mu     sync.Mutex
		fallos []error
	)
	for _, procesoID := range []string{"prc-02", "prc-03"} {
		grupo.Add(1)
		go func(id string) {
			defer grupo.Done()
			if err := generar(id); err != nil {
				mu.Lock()
				fallos = append(fallos, err)
				mu.Unlock()
			}
		}(procesoID)
	}
	grupo.Wait()
	// Un fallo por contencion seria aceptable -- lo que no es aceptable es
	// pagar dos veces -- pero aqui no debe haber ninguno.
	for _, err := range fallos {
		t.Fatalf("generacion concurrente: %v", err)
	}

	ordenes, err := s.DeTitular(ctx, titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	vivos := decimal.Zero
	arrastres := 0
	for _, o := range ordenes {
		if o.Estado == liquidacion.EstadoAcumulada {
			continue
		}
		vivos = vivos.Add(o.Neto)
		arrastres += len(o.Arrastres)
	}
	if !vivos.Equal(pgDec("61000")) {
		t.Fatalf("suma de netos vivos = %s, se esperaba 61000 (30000+30000+1000); "+
			"62000 significa que el arrastre de R-11 se pago dos veces", vivos)
	}
	if arrastres != 1 {
		t.Fatalf("%d arrastres incorporados, la diferida es UNA", arrastres)
	}
}

// TestGeneracionesConcurrentesDelMismoPeriodoNoDuplicanNiPierdenElArrastre es B3b.
//
// La MISMA corrida se liquida dos veces a la vez. Las dos pasan la compuerta,
// las dos leen "no hay ordenes para 2026-02/nacional" si nada las serializa, y
// las dos calculan una orden con el arrastre de 1000. Como el id es estable,
// la segunda insercion se descarta -- EmitirOrdenes no pisa -- y la orden que
// queda es la de la primera: si las dos hubieran marcado acumulada la diferida
// y la que sobrevive fuera la que no la incorporo, el saldo desapareceria.
//
// El cerrojo de aviso de BloquearPeriodo es lo que lo cierra: la segunda espera
// el commit de la primera, ve la orden ya emitida y devuelve lo que hay.
func TestGeneracionesConcurrentesDelMismoPeriodoNoDuplicanNiPierdenElArrastre(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()

	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)

	sembrarBolsa(t, pool, "bolsa-00", "2026-01", "nacional")
	sembrarProcesoListo(t, pool, "prc-00", "bolsa-00", "2026-01", "nacional")
	ejecutorDePruebas(t, pool)(
		`INSERT INTO ordenes_pago
		   (id, proceso_id, procesos, titular_id, periodo, circuito,
		    bruto, neto, estado, enviada, arrastres)
		 VALUES ('liq-2026-01-nacional-'||$1, 'prc-00', ARRAY['prc-00'], $1, '2026-01',
		         'nacional', 1000, 1000, 'diferida', '2026-01-01', '{}')`, titularAna)

	sembrarBolsa(t, pool, "bolsa-02", "2026-02", "nacional")
	sembrarProcesoListo(t, pool, "prc-02", "bolsa-02", "2026-02", "nacional")
	sembrarResultado(t, pool, "prc-02", "30000", "0", "0", "0")
	ejecutorDePruebas(t, pool)(
		`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
		 VALUES ('prc-02', $1, 10, 30000, FALSE)`, obraCompleta)
	ejecutorDePruebas(t, pool)(
		`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
		 VALUES ('prc-02', $1, $2, 'IPI-00000001', 100, 30000)`, obraCompleta, titularAna)

	cita := citaDe(2)
	var (
		grupo  sync.WaitGroup
		mu     sync.Mutex
		fallos []error
	)
	for i := 0; i < 2; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			liq := liquidacionesDe(s, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
			liq.Ordenes = repoConCita{Store: s, cita: cita}
			if _, err := liq.GenerarLiquidacion(ctx, "prc-02"); err != nil {
				mu.Lock()
				fallos = append(fallos, err)
				mu.Unlock()
			}
		}()
	}
	grupo.Wait()
	for _, err := range fallos {
		t.Fatalf("generacion concurrente: %v", err)
	}

	ordenes, err := s.DeTitular(ctx, titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if len(ordenes) != 2 {
		t.Fatalf("%d ordenes; se esperaban 2 (la diferida acumulada y la de 2026-02)", len(ordenes))
	}

	var nueva, previa liquidacion.OrdenDePago
	for _, o := range ordenes {
		switch o.Periodo {
		case "2026-01":
			previa = o
		case "2026-02":
			nueva = o
		}
	}
	if !nueva.Neto.Equal(pgDec("31000")) {
		t.Fatalf("neto de 2026-02 = %s, se esperaba 31000 (30000 + 1000 arrastrados); "+
			"30000 significa que el arrastre se perdio", nueva.Neto)
	}
	if len(nueva.Arrastres) != 1 || nueva.Arrastres[0] != previa.ID {
		t.Fatalf("Arrastres = %v, se esperaba [%s]", nueva.Arrastres, previa.ID)
	}
	if previa.Estado != liquidacion.EstadoAcumulada {
		t.Fatalf("la diferida incorporada tiene que quedar acumulada: %q", previa.Estado)
	}

	// Un solo asiento de emision: dos habrian significado dos hechos para una
	// sola orden.
	asientos, err := s.De(ctx, aplicacion.RefOrdenDePago, nueva.ID)
	if err != nil {
		t.Fatalf("asientos: %v", err)
	}
	emisiones := 0
	for _, a := range asientos {
		if a.Hecho == aplicacion.HechoLiquidacionEmitida {
			emisiones++
		}
	}
	if emisiones != 1 {
		t.Fatalf("%d asientos de emision para una sola orden", emisiones)
	}
}

func TestBloquearPeriodoSeNiegaFueraDeUnaUnidad(t *testing.T) {
	s, _ := sembrar(t)
	if err := s.BloquearPeriodo(t.Context(), "2026", reparto.Nacional); err == nil {
		t.Fatal("un cerrojo de transaccion tomado contra el pool se suelta al instante")
	}
	if _, err := s.DiferidasDeTitular(t.Context(), titularAna); err == nil {
		t.Fatal("un FOR UPDATE contra el pool no protege nada")
	}
}

func TestDeTitularListaVaciaSinOrdenes(t *testing.T) {
	s, _ := sembrarCorrida(t, false)

	got, err := s.DeTitular(t.Context(), titularAna)
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if got == nil {
		t.Fatal("una lista vacia no puede ser nil")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestSilencioALos15DiasPersisteAceptadaPorSilencio(t *testing.T) {
	s, liq := sembrarCorrida(t, true)
	ctx := t.Context()

	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); err != nil {
		t.Fatalf("generar: %v", err)
	}

	actor := aplicacion.Usuario{Rol: aplicacion.RolTitular, TitularID: titularAna}

	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)}
	dia14, err := liq.DeTitular(ctx, actor)
	if err != nil {
		t.Fatalf("dia 14: %v", err)
	}
	if dia14[0].Orden.Estado != liquidacion.EstadoEnviada {
		t.Fatalf("dia 14: %q", dia14[0].Orden.Estado)
	}

	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)}
	dia15, err := liq.DeTitular(ctx, actor)
	if err != nil {
		t.Fatalf("dia 15: %v", err)
	}
	if dia15[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("dia 15: %q", dia15[0].Orden.Estado)
	}
	if !dia15[0].Pagable {
		t.Fatal("Ana tiene RUT y banco: tiene que ser pagable")
	}

	// La transicion es un hecho y deja su asiento, en la misma transaccion
	// (ADR 0006).
	asientos, err := s.De(ctx, aplicacion.RefOrdenDePago, dia15[0].Orden.ID)
	if err != nil {
		t.Fatalf("asientos: %v", err)
	}
	silencios := 0
	for _, a := range asientos {
		if a.Hecho == aplicacion.HechoLiquidacionAceptadaPorSilencio {
			silencios++
		}
	}
	if silencios != 1 {
		t.Fatalf("%d asientos de aceptacion por silencio, se esperaba 1", silencios)
	}

	// La transicion quedo escrita: un reloj que volviera atras no la
	// deshace, porque EvaluarPlazo no toca un estado distinto de enviada.
	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)}
	despues, err := liq.DeTitular(ctx, actor)
	if err != nil {
		t.Fatalf("releer: %v", err)
	}
	if despues[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("la aceptacion por silencio no persistio: %q", despues[0].Orden.Estado)
	}
}

func TestOrdenSinDocumentosNoEsPagableTrasSilencio(t *testing.T) {
	_, liq := sembrarCorrida(t, false)
	ctx := t.Context()

	if _, err := liq.GenerarLiquidacion(ctx, procesoNac); err != nil {
		t.Fatalf("generar: %v", err)
	}

	liq.Reloj = reloj.Fijo{Instante: time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC)}
	vistas, err := liq.DeTitular(ctx, aplicacion.Usuario{Rol: aplicacion.RolTitular, TitularID: titularAna})
	if err != nil {
		t.Fatalf("DeTitular: %v", err)
	}
	if vistas[0].Orden.Estado != liquidacion.EstadoAceptadaPorSilencio {
		t.Fatalf("Estado = %q", vistas[0].Orden.Estado)
	}
	if vistas[0].Pagable {
		t.Fatal("sin RUT y banco no es pagable (R-12)")
	}
}

func TestSMMLVAusenteEsParametroAusente(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.SMMLVVigente(t.Context(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, aplicacion.ErrParametroAusente) {
		t.Fatalf("se esperaba ErrParametroAusente, se obtuvo %v", err)
	}
}

func TestInsumoDeProcesoNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.InsumoDeProceso(t.Context(), "prc-inexistente")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}

func TestMetaDeProcesoNoEncontrado(t *testing.T) {
	s, _ := sembrar(t)
	_, err := s.MetaDeProceso(t.Context(), "prc-inexistente")
	if !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se esperaba ErrNoEncontrado, se obtuvo %v", err)
	}
}
