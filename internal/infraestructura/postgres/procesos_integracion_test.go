package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
)

// sembrarProcesoNacionalListoParaValorizar deja una bolsa, un uso
// identificado (el ejemplo de la Serie Y de RD 9.1.1: 10 emisiones de 48
// min, rating 9), su declaracion al 100%, los parametros normativos y los
// actores de firma -- todo lo que [aplicacion.Procesos.IniciarProceso] y
// [aplicacion.Procesos.AvanzarEtapa] necesitan para valorizar de verdad
// contra Postgres real.
func sembrarProcesoNacionalListoParaValorizar(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool := testhelp.Pool(t)
	return sembrarProcesoNacionalEn(t, pool), pool
}

// sembrarProcesoNacionalEn es [sembrarProcesoNacionalListoParaValorizar] sobre
// un pool ya abierto. La prueba del cerrojo de periodo necesita MaxConns>1:
// el pool de testhelp deja 1 y la entrega concurrente se quedaria esperando
// una conexion, no el cerrojo.
func sembrarProcesoNacionalEn(t *testing.T, pool *pgxpool.Pool) *Store {
	t.Helper()
	s := sembrarReportesEn(t, pool)
	ctx := t.Context()

	// El usuario de recaudo de television ES el canal (ADR 0019): "caracol"
	// tiene que ser el mismo valor en usuarios_recaudo, bolsas.usuario_id y
	// usos.canal_id.
	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios_recaudo (id, nombre, categoria) VALUES ('caracol', 'Caracol', 'tv_abierta')`); err != nil {
		t.Fatalf("sembrar usuario_recaudo: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO bolsas (id, usuario_id, periodo, circuito, bruto)
		 VALUES ('bolsa-1', 'caracol', '2026-01', 'nacional', 1000000.00)`); err != nil {
		t.Fatalf("sembrar bolsa: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ('obra-y', 'Obra Y', 'Drama', 2020, 'serie')`); err != nil {
		t.Fatalf("sembrar obra: %v", err)
	}
	uso := usoPendiente("uso-y", reporteEnero, "Obra Y")
	uso.CanalID = "caracol"
	uso.TipoObra = "serie"
	uso.Emisiones = 10
	uso.DuracionMin = dec("48")
	uso.Rating = dec("9")
	uso.Escalon = "alias"
	uso.ONI = false
	uso.ObraID = "obra-y"
	uso.Evidencia = "alias caracol/id=uso-y"
	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{uso}); err != nil {
		t.Fatalf("sembrar uso: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO titulares (id, nombre, ipi, clase) VALUES ('titular-y', 'Titular Y', 'IPI-Y', 'socio')`); err != nil {
		t.Fatalf("sembrar titular: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO usuarios (id, email, nombre, rol, password_hash) VALUES
		 ('actor-declaracion', 'actor-declaracion@redes.test', 'Declaracion', 'administrador', 'hash-de-prueba-suficientemente-larga'),
		 ('actor-dist', 'actor-dist@redes.test', 'Distribucion', 'distribucion', 'hash-de-prueba-suficientemente-larga'),
		 ('actor-conta', 'actor-conta@redes.test', 'Contabilidad', 'contabilidad', 'hash-de-prueba-suficientemente-larga')`); err != nil {
		t.Fatalf("sembrar actores: %v", err)
	}

	decl, err := repertorio.NuevaDeclaracion("obra-y", []repertorio.Parte{
		{TitularID: "titular-y", IPI: "IPI-Y", Porcentaje: dec("100")},
	})
	if err != nil {
		t.Fatalf("construir declaracion: %v", err)
	}
	if _, _, err := s.Guardar(ctx, decl, time.Now(), "actor-declaracion"); err != nil {
		t.Fatalf("guardar declaracion: %v", err)
	}

	sembrarParametros(t, pool, "2026-01-01")
	return s
}

// TestProcesoNacionalDePuntaAPunta es la prueba de verificacion manual del
// plan de #34: abre una corrida Nacional contra Postgres real, la avanza
// hasta que el motor de #33 valoriza de verdad y el resultado queda en
// resultados_*, la firma en su compuerta con los dos roles, y confirma que
// llega a Auditoria. No usa ningun doble: [aplicacion.Procesos] entero,
// resuelto contra *Store, igual que lo cablea cmd/api.
func TestProcesoNacionalDePuntaAPunta(t *testing.T) {
	s, _ := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()

	uc := aplicacion.Procesos{
		Repo:          s,
		Parametros:    s,
		Bolsas:        s,
		Declaraciones: s,
		Usos:          s,
		Resultados:    s,
		Unidad:        s,
		Anomalias:     servicioDeAnomalias(s, time.Now()),
	}

	p, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1")
	if err != nil {
		t.Fatalf("iniciar proceso: %v", err)
	}
	if p.Etapa != reparto.EtapaRecaudo {
		t.Fatalf("etapa = %q, se esperaba recaudo", p.Etapa)
	}

	// Idempotente: reabrir el mismo id no reinicia nada (prueba lo que un
	// reintento del trabajo de cola haria).
	if _, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1"); err != nil {
		t.Fatalf("reabrir proceso (idempotente): %v", err)
	}

	// recaudo -> deducciones -> importe_obra (aqui valoriza de verdad).
	for _, esperada := range []reparto.Etapa{reparto.EtapaDeducciones, reparto.EtapaImporteObra} {
		p, err = uc.AvanzarEtapa(ctx, "proc-y")
		if err != nil {
			t.Fatalf("avanzar a %q: %v", esperada, err)
		}
		if p.Etapa != esperada {
			t.Fatalf("etapa = %q, se esperaba %q", p.Etapa, esperada)
		}
	}

	resultado, err := s.ResultadoPorProceso(ctx, "proc-y")
	if err != nil {
		t.Fatalf("leer el resultado persistido: %v", err)
	}
	if len(resultado.Obras) != 1 || resultado.Obras[0].ObraID != "obra-y" {
		t.Fatalf("resultado.Obras = %+v, se esperaba una linea de obra-y", resultado.Obras)
	}
	if len(resultado.Titulares) != 1 || resultado.Titulares[0].TitularID != "titular-y" {
		t.Fatalf("resultado.Titulares = %+v, se esperaba una linea de titular-y", resultado.Titulares)
	}
	if resultado.Titulares[0].Importe.IsZero() {
		t.Fatal("el importe del titular no puede quedar en cero: la valorizacion no corrio de verdad")
	}

	// importe_titular -> liquidacion_parcial -> verificacion (compuerta).
	for _, esperada := range []reparto.Etapa{reparto.EtapaImporteTitular, reparto.EtapaLiquidacionParcial, reparto.EtapaVerificacion} {
		p, err = uc.AvanzarEtapa(ctx, "proc-y")
		if err != nil {
			t.Fatalf("avanzar a %q: %v", esperada, err)
		}
		if p.Etapa != esperada {
			t.Fatalf("etapa = %q, se esperaba %q", p.Etapa, esperada)
		}
	}

	// Verificacion sin firmas no deja pasar (RD 13.5).
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err == nil {
		t.Fatal("se esperaba error: verificacion sin firmas no avanza")
	}

	if _, err := uc.Firmar(ctx, "proc-y", reparto.RolDistribucion, "actor-dist"); err != nil {
		t.Fatalf("firmar distribucion: %v", err)
	}
	if _, err := uc.Firmar(ctx, "proc-y", reparto.RolContabilidad, "actor-conta"); err != nil {
		t.Fatalf("firmar contabilidad: %v", err)
	}

	// liquidacion_final -> pago_registro (otra compuerta).
	p, err = uc.AvanzarEtapa(ctx, "proc-y")
	if err != nil {
		t.Fatalf("avanzar a liquidacion_final: %v", err)
	}
	if p.Etapa != reparto.EtapaLiquidacionFinal {
		t.Fatalf("etapa = %q, se esperaba liquidacion_final", p.Etapa)
	}
	p, err = uc.AvanzarEtapa(ctx, "proc-y")
	if err != nil {
		t.Fatalf("avanzar a pago_registro: %v", err)
	}
	if p.Etapa != reparto.EtapaPagoRegistro {
		t.Fatalf("etapa = %q, se esperaba pago_registro", p.Etapa)
	}

	// Las firmas de la compuerta de verificacion no cuentan para esta:
	// avanzar en la revision actual tiene que fallar hasta firmar de nuevo.
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err == nil {
		t.Fatal("se esperaba error: pago_registro exige sus propias firmas, no reusa las de verificacion")
	}
	if _, err := uc.Firmar(ctx, "proc-y", reparto.RolDistribucion, "actor-dist"); err != nil {
		t.Fatalf("firmar distribucion (pago_registro): %v", err)
	}
	if _, err := uc.Firmar(ctx, "proc-y", reparto.RolContabilidad, "actor-conta"); err != nil {
		t.Fatalf("firmar contabilidad (pago_registro): %v", err)
	}

	p, err = uc.AvanzarEtapa(ctx, "proc-y")
	if err != nil {
		t.Fatalf("avanzar a auditoria: %v", err)
	}
	if p.Etapa != reparto.EtapaAuditoria {
		t.Fatalf("etapa = %q, se esperaba auditoria", p.Etapa)
	}

	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err == nil {
		t.Fatal("se esperaba error: auditoria es terminal")
	}
}

// TestProcesoVerificacionRechazadaRetrocedeYSubeRevision prueba el otro
// camino de la compuerta contra Postgres real: un rechazo retrocede una
// etapa y las firmas anteriores dejan de contar (ADR 0008), no solo en el
// agregado en memoria sino releido de la base.
func TestProcesoVerificacionRechazadaRetrocedeYSubeRevision(t *testing.T) {
	s := sembrarBolsaParaProceso(t)
	ctx := t.Context()

	p := procesoDePrueba()
	p.Etapa = reparto.EtapaVerificacion
	if err := s.GuardarProceso(ctx, p, 0); err != nil {
		t.Fatalf("sembrar proceso en verificacion: %v", err)
	}

	uc := aplicacion.Procesos{Repo: s}

	if _, err := uc.Firmar(ctx, "proc-1", reparto.RolDistribucion, "actor-dist"); err != nil {
		t.Fatalf("firmar distribucion: %v", err)
	}

	leido, err := uc.RechazarGate(ctx, "proc-1", "faltan soportes")
	if err != nil {
		t.Fatalf("rechazar compuerta: %v", err)
	}
	if leido.Etapa != reparto.EtapaLiquidacionParcial {
		t.Fatalf("etapa = %q, se esperaba retroceder a liquidacion_parcial", leido.Etapa)
	}
	if leido.Revision != 2 {
		t.Fatalf("revision = %d, se esperaba 2", leido.Revision)
	}

	releido, err := s.ProcesoPorID(ctx, "proc-1")
	if err != nil {
		t.Fatalf("releer proceso: %v", err)
	}
	if releido.Revision != 2 || releido.Etapa != reparto.EtapaLiquidacionParcial || releido.RechazoMotivo != "faltan soportes" {
		t.Fatalf("releido = %+v", releido)
	}
}

// repoProcesosQueFallaUnaVez envuelve un *Store real y hace fallar
// GuardarProceso la primera vez que se llama, para forzar el camino de
// rollback de AvanzarEtapa sin tocar SQL a mano.
type repoProcesosQueFallaUnaVez struct {
	*Store
	fallar bool
}

func (r *repoProcesosQueFallaUnaVez) GuardarProceso(ctx context.Context, p aplicacion.ProcesoVista, revisionAnterior int) error {
	if r.fallar {
		r.fallar = false
		return errors.New("fallo simulado de infraestructura, DESPUES de que el motor ya valorizo")
	}
	return r.Store.GuardarProceso(ctx, p, revisionAnterior)
}

// TestAvanzarEtapaSinAtomicidadDejaHuerfanoYRompeElReintento es la
// reproduccion exacta del hallazgo bloqueante de la revision de #159: si
// GuardarProceso falla DESPUES de que valorizar ya escribio en
// resultados_*, sin que las dos escrituras compartan una transaccion real,
// (a) resultados_proceso queda con una fila huerfana para una corrida que
// nunca avanzo, y (b) reintentar AvanzarEtapa revienta con una violacion de
// la PK de resultados_proceso en vez de reintentar limpio.
//
// Con GuardarResultado y GuardarProceso participando de verdad en la
// UnidadDeTrabajo (via enTransaccionDe/ejecutorDe), el fallo de
// GuardarProceso revierte TAMBIEN el INSERT de resultados_proceso -- asi que
// no hay huerfano, y el reintento vuelve a valorizar y guardar limpio.
func TestAvanzarEtapaSinAtomicidadDejaHuerfanoYRompeElReintento(t *testing.T) {
	s, _ := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()

	repoQueFalla := &repoProcesosQueFallaUnaVez{Store: s}
	uc := aplicacion.Procesos{
		Repo:          repoQueFalla,
		Parametros:    s,
		Bolsas:        s,
		Declaraciones: s,
		Usos:          s,
		Resultados:    s,
		Unidad:        s,
		Anomalias:     servicioDeAnomalias(s, time.Now()),
	}

	if _, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1"); err != nil {
		t.Fatalf("iniciar proceso: %v", err)
	}
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err != nil {
		t.Fatalf("avanzar a deducciones: %v", err)
	}

	// Solo a partir de aqui falla: este AvanzarEtapa entra a importe_obra,
	// valorizar corre de verdad, y GuardarProceso falla justo despues.
	repoQueFalla.fallar = true
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err == nil {
		t.Fatal("se esperaba el fallo simulado de GuardarProceso")
	}

	if _, err := s.ResultadoPorProceso(ctx, "proc-y"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("resultados_proceso quedo con una fila huerfana tras el rollback: %v", err)
	}
	p, err := s.ProcesoPorID(ctx, "proc-y")
	if err != nil {
		t.Fatalf("leer proceso: %v", err)
	}
	if p.Etapa != reparto.EtapaDeducciones {
		t.Fatalf("etapa = %q, se esperaba que el fallo dejara el proceso donde estaba (deducciones)", p.Etapa)
	}

	// El reintento -- el mismo AvanzarEtapa, ahora con GuardarProceso sin
	// fallar -- tiene que valorizar y guardar limpio, no reventar con
	// "duplicate key value violates unique constraint resultados_proceso_pkey".
	p, err = uc.AvanzarEtapa(ctx, "proc-y")
	if err != nil {
		t.Fatalf("el reintento de avanzar a importe_obra fallo: %v", err)
	}
	if p.Etapa != reparto.EtapaImporteObra {
		t.Fatalf("etapa = %q, se esperaba importe_obra tras el reintento", p.Etapa)
	}
	resultado, err := s.ResultadoPorProceso(ctx, "proc-y")
	if err != nil {
		t.Fatalf("leer el resultado tras el reintento: %v", err)
	}
	if len(resultado.Titulares) != 1 || resultado.Titulares[0].Importe.IsZero() {
		t.Fatalf("resultado tras el reintento = %+v, se esperaba una linea de titular con importe", resultado)
	}
}

// TestLaCompuertaDeAnomaliasBloqueaLaSalidaDeDeducciones: sin evaluar -> evalua y bloquea; resuelta -> avanza (#37).
func TestLaCompuertaDeAnomaliasBloqueaLaSalidaDeDeducciones(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()

	// Un duplicado de registro en 2026-01: la misma emision de caracol en dos entregas.
	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ('rep-caracol-enero-bis', 'caracol', '2026-01', repeat('c', 64), 'reportes/c.csv', 64)`); err != nil {
		t.Fatalf("sembrar la segunda entrega: %v", err)
	}
	insertarUsoDeAlertas(t, pool, "u-dup1", reporteEnero, "alias", "obra-y", "serie", "id_ficha=7", "2026-01-02", "20:00:00")
	insertarUsoDeAlertas(t, pool, "u-dup2", "rep-caracol-enero-bis", "alias", "obra-y", "serie", "id_ficha=7", "2026-01-02", "20:00:00")

	anomalias := servicioDeAnomalias(s, time.Now())
	uc := aplicacion.Procesos{
		Repo:          s,
		Parametros:    s,
		Bolsas:        s,
		Declaraciones: s,
		Usos:          s,
		Resultados:    s,
		Unidad:        s,
		Anomalias:     anomalias,
	}
	if _, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1"); err != nil {
		t.Fatalf("iniciar proceso: %v", err)
	}
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err != nil {
		t.Fatalf("avanzar a deducciones: %v", err)
	}

	// Nadie evaluo el periodo: la bandeja esta vacia, y aun asi la compuerta no abre.
	var guardadas int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM alertas WHERE periodo = '2026-01'`).Scan(&guardadas); err != nil {
		t.Fatalf("contar alertas: %v", err)
	}
	if guardadas != 0 {
		t.Fatalf("el periodo no deberia estar evaluado todavia, hay %d alertas", guardadas)
	}
	_, err := uc.AvanzarEtapa(ctx, "proc-y")
	if !errors.Is(err, aplicacion.ErrAnomaliasCriticasAbiertas) {
		t.Fatalf("err = %v, se esperaba ErrAnomaliasCriticasAbiertas", err)
	}
	p, err := s.ProcesoPorID(ctx, "proc-y")
	if err != nil {
		t.Fatalf("leer proceso: %v", err)
	}
	if p.Etapa != reparto.EtapaDeducciones {
		t.Fatalf("etapa = %q, la compuerta debio dejarlo en deducciones", p.Etapa)
	}
	if _, err := s.ResultadoPorProceso(ctx, "proc-y"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("se valorizo con criticas abiertas: %v", err)
	}

	// Un segundo intento sigue bloqueado: la evaluacion es idempotente, no "ya mire".
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); !errors.Is(err, aplicacion.ErrAnomaliasCriticasAbiertas) {
		t.Fatalf("segundo intento: err = %v, se esperaba seguir bloqueado", err)
	}

	sinResolver := false
	criticas, err := anomalias.Listar(ctx, aplicacion.FiltroAlertas{Periodo: "2026-01", Resueltas: &sinResolver})
	if err != nil {
		t.Fatalf("listar abiertas: %v", err)
	}
	resueltas := 0
	for _, a := range criticas {
		if !a.Critica {
			continue
		}
		if _, err := anomalias.Resolver(ctx, a.ID, "actor-dist", "entrega bis es reenvio, se excluye en #39"); err != nil {
			t.Fatalf("resolver %s: %v", a.ID, err)
		}
		resueltas++
	}
	if resueltas == 0 {
		t.Fatal("la evaluacion no dejo ninguna critica que resolver")
	}

	p, err = uc.AvanzarEtapa(ctx, "proc-y")
	if err != nil {
		t.Fatalf("con las criticas resueltas deberia avanzar: %v", err)
	}
	if p.Etapa != reparto.EtapaImporteObra {
		t.Fatalf("etapa = %q, se esperaba importe_obra", p.Etapa)
	}
}

// poolDePrueba abre un pool con mas de una conexion. testhelp.Pool deja
// MaxConns=1, y una prueba que fuerza dos transacciones a la vez se quedaria
// esperando el pool, no el cerrojo.
func poolDePrueba(t *testing.T, max int32) *pgxpool.Pool {
	t.Helper()
	dsn := testhelp.DSN(t)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("configurar el pool: %v", err)
	}
	cfg.MaxConns = max
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("abrir el pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestEntregaDuranteValorizarEsperaElCerrojoYNoPondera cierra el TOCTOU de #166.
//
// La compuerta y la valorizacion comparten el cerrojo de alertas. Esta prueba
// fuerza la ventana: la valorizacion queda esperando la fila del proceso
// (con el cerrojo de alertas ya tomado) y la entrega no puede confirmarse
// hasta que esa unidad termina. El resultado no incluye la obra que la
// entrega traia.
func TestEntregaDuranteValorizarEsperaElCerrojoYNoPondera(t *testing.T) {
	pool := poolDePrueba(t, 4)
	s := sembrarProcesoNacionalEn(t, pool)
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ('obra-z', 'Obra Z', 'Drama', 2021, 'serie')`); err != nil {
		t.Fatalf("sembrar obra-z: %v", err)
	}

	uc := aplicacion.Procesos{
		Repo: s, Parametros: s, Bolsas: s, Declaraciones: s, Usos: s, Resultados: s, Unidad: s,
		Anomalias: servicioDeAnomalias(s, time.Now()),
	}
	if _, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1"); err != nil {
		t.Fatalf("iniciar proceso: %v", err)
	}
	if _, err := uc.AvanzarEtapa(ctx, "proc-y"); err != nil {
		t.Fatalf("avanzar a deducciones: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("abrir la transaccion que retiene el proceso: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SELECT id FROM procesos WHERE id = 'proc-y' FOR UPDATE`); err != nil {
		t.Fatalf("retener el proceso: %v", err)
	}

	var grupo sync.WaitGroup
	avanceErr := make(chan error, 1)
	grupo.Add(1)
	go func() {
		defer grupo.Done()
		_, err := uc.AvanzarEtapa(ctx, "proc-y")
		avanceErr <- err
	}()
	esperarConsultaBloqueada(t, s, "INSERT INTO resultados_proceso")

	uso := usoPendiente("uso-z", "rep-durante-valorizar", "Obra Z")
	uso.CanalID = "caracol"
	uso.TipoObra = "serie"
	uso.Emisiones = 100
	uso.DuracionMin = dec("48")
	uso.Rating = dec("9")
	uso.Escalon = "alias"
	uso.ONI = false
	uso.ObraID = "obra-z"
	uso.Evidencia = "alias caracol/obra-z"
	rep := aplicacion.Reporte{
		ID: "rep-durante-valorizar", Fuente: "caracol", Periodo: "2026-01",
		SHA256:      "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ClaveObjeto: "reportes/d.csv", NBytes: 64,
	}
	entregaErr := make(chan error, 1)
	grupo.Add(1)
	go func() {
		defer grupo.Done()
		entregaErr <- s.GuardarEntrega(ctx, rep, []aplicacion.UsoPersistido{uso})
	}()
	esperarCerrojoDeAvisoEnEspera(t, s)

	var yaEscrita int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos WHERE id = 'uso-z'`).Scan(&yaEscrita); err != nil {
		t.Fatalf("contar uso-z mientras espera: %v", err)
	}
	if yaEscrita != 0 {
		t.Fatal("la entrega se confirmo con el cerrojo de alertas tomado por la valorizacion")
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("soltar el proceso: %v", err)
	}
	grupo.Wait()
	if err := <-avanceErr; err != nil {
		t.Fatalf("avanzar a importe_obra: %v", err)
	}
	if err := <-entregaErr; err != nil {
		t.Fatalf("guardar la entrega tardia: %v", err)
	}

	var enResultado int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM resultados_obra WHERE proceso_id = 'proc-y' AND obra_id = 'obra-z'`).Scan(&enResultado); err != nil {
		t.Fatalf("leer el resultado: %v", err)
	}
	if enResultado != 0 {
		t.Fatal("obra-z pondero: la entrega entro entre evaluar y valorizar")
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos WHERE id = 'uso-z'`).Scan(&yaEscrita); err != nil {
		t.Fatalf("contar uso-z al final: %v", err)
	}
	if yaEscrita != 1 {
		t.Fatal("la entrega no quedo escrita despues de soltar el cerrojo")
	}
}

// esperarConsultaBloqueada espera a que alguna sesion este de verdad parada en
// un Lock con esa sentencia, no un sleep a ciegas.
func esperarConsultaBloqueada(t *testing.T, s *Store, fragmento string) {
	t.Helper()
	conn, err := pgx.ConnectConfig(t.Context(), s.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("conexion de observacion: %v", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	limite := time.Now().Add(15 * time.Second)
	for time.Now().Before(limite) {
		var bloqueada bool
		err := conn.QueryRow(t.Context(),
			`SELECT EXISTS (
			   SELECT 1 FROM pg_stat_activity
			    WHERE wait_event_type = 'Lock' AND query ILIKE '%' || $1 || '%'
			 )`, fragmento).Scan(&bloqueada)
		if err != nil {
			t.Fatalf("consultar pg_stat_activity: %v", err)
		}
		if bloqueada {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nadie quedo bloqueado en %q\n%s", fragmento, actividad(t, conn))
}

func actividad(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var dump string
	err := conn.QueryRow(context.Background(),
		`SELECT coalesce(string_agg(
		    pid::text || ' ' || state || ' ' || coalesce(wait_event_type,'-') || '/' || coalesce(wait_event,'-')
		    || E'\n' || left(query, 300), E'\n---\n'), '')
		   FROM pg_stat_activity WHERE datname = current_database()`).Scan(&dump)
	if err != nil {
		return err.Error()
	}
	return dump
}
