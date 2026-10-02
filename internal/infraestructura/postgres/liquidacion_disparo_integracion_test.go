package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/infraestructura/httpapi"
	"github.com/rosvend/intela/internal/infraestructura/postgres/testhelp"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Pruebas de #193 contra Postgres real: que avanzar una corrida nacional a
// liquidacion_final emite las ordenes del periodo en la misma transaccion, que
// el periodo espera a todas sus corridas (ADR 0024), y que el aviso queda en
// `notificaciones` con la orden.

// comprobarLiquidacionEmitida comprueba el estado persistido por la emision a
// traves de los casos de uso de aplicacion ([aplicacion.Liquidaciones.Listar] y
// [aplicacion.Liquidaciones.DeTitular]), el aviso de portal y el libro. Hay UNA
// orden del titular con el neto dado, UN aviso, UN asiento de emision que lleva
// el acuse de ese aviso, y UN asiento de lote.
func comprobarLiquidacionEmitida(
	t *testing.T, s *Store, pool *pgxpool.Pool, procesoID, titularID string, neto decimal.Decimal,
) {
	t.Helper()
	ctx := t.Context()
	liq := aplicacion.Liquidaciones{
		Ordenes: s, Reloj: reloj.Sistema{}, Notificador: s.AvisoPortal(), Bitacora: s, Unidad: s,
	}

	admin, err := liq.Listar(ctx, aplicacion.Usuario{Rol: aplicacion.RolAdministrador})
	if err != nil {
		t.Fatalf("Listar liquidaciones: %v", err)
	}
	if len(admin) != 1 || admin[0].Orden.TitularID != titularID {
		t.Fatalf("Listar liquidaciones = %+v, se esperaba una orden de %s", admin, titularID)
	}
	orden := admin[0].Orden
	if !orden.Neto.Equal(neto) {
		t.Fatalf("neto = %s, se esperaba %s", orden.Neto, neto)
	}
	if !slices.Contains(orden.Procesos, procesoID) {
		t.Fatalf("Procesos = %v, falta %s", orden.Procesos, procesoID)
	}

	suyas, err := liq.DeTitular(ctx, aplicacion.Usuario{Rol: aplicacion.RolTitular, TitularID: titularID})
	if err != nil {
		t.Fatalf("DeTitular liquidaciones: %v", err)
	}
	if len(suyas) != 1 || suyas[0].Orden.ID != orden.ID {
		t.Fatalf("DeTitular liquidaciones = %+v, se esperaba %s", suyas, orden.ID)
	}

	var avisos int
	var acuse, destino, procesoAviso string
	if err := pool.QueryRow(ctx,
		`SELECT count(*) OVER (), acuse, destino, proceso_id FROM notificaciones
		  WHERE titular_id = $1 AND via = 'portal'`, titularID).Scan(&avisos, &acuse, &destino, &procesoAviso); err != nil {
		t.Fatalf("leer el aviso de portal: %v", err)
	}
	if avisos != 1 || destino != destinoPortal {
		t.Fatalf("avisos = %d, destino = %q; se esperaba uno en %s", avisos, destino, destinoPortal)
	}
	if procesoAviso != orden.ProcesoID {
		t.Fatalf("proceso en aviso = %q, se esperaba %q (la corrida de referencia del titular)", procesoAviso, orden.ProcesoID)
	}

	emision, err := s.De(ctx, aplicacion.RefOrdenDePago, orden.ID)
	if err != nil {
		t.Fatalf("asientos de la orden: %v", err)
	}
	emitidas := 0
	for _, a := range emision {
		if a.Hecho != aplicacion.HechoLiquidacionEmitida {
			continue
		}
		emitidas++
		var payload aplicacion.AsientoOrden
		if err := json.Unmarshal(a.Payload, &payload); err != nil {
			t.Fatalf("payload de la emision: %v", err)
		}
		if payload.Acuse != acuse {
			t.Fatalf("acuse del asiento = %q, el del aviso = %q: el libro tiene que citar el aviso real", payload.Acuse, acuse)
		}
	}
	if emitidas != 1 {
		t.Fatalf("%d asientos de emision de %s; se esperaba uno", emitidas, orden.ID)
	}

	lote, err := s.De(ctx, aplicacion.RefLiquidacionLote, orden.Periodo+"-"+orden.Circuito)
	if err != nil {
		t.Fatalf("asiento del lote: %v", err)
	}
	if len(lote) != 1 {
		t.Fatalf("%d asientos de lote; el lote se asienta una vez", len(lote))
	}
}

// sembrarCorridaEnVerificacion deja una corrida nacional en verificacion con
// las dos firmas de la revision 1 -- lista para salir -- y su resultado: una
// sola linea de Ana por importe. Cada corrida con su propia bolsa (ADR 0019).
func sembrarCorridaEnVerificacion(t *testing.T, pool *pgxpool.Pool, procesoID, bolsaID, periodo, importe string) {
	sembrarCorridaEnVerificacionTitular(t, pool, procesoID, bolsaID, periodo, importe, titularAna)
}

func sembrarCorridaEnVerificacionTitular(t *testing.T, pool *pgxpool.Pool, procesoID, bolsaID, periodo, importe, titularID string) {
	t.Helper()
	ejecutar := ejecutorDePruebas(t, pool)
	sembrarBolsa(t, pool, bolsaID, periodo, "nacional")
	ejecutar(`INSERT INTO procesos (id, circuito, etapa, periodo, bolsa_id, snapshot_id, reglamento)
	          VALUES ($1, 'nacional', 'verificacion', $2, $3, 'snap-1', 'RD-IX')`, procesoID, periodo, bolsaID)
	ejecutar(`INSERT INTO firmas (proceso_id, rol, revision, actor_id)
	          VALUES ($1, 'distribucion', 1, $2), ($1, 'contabilidad', 1, $3)`,
		procesoID, usuarioDistribucion, usuarioContabilidad)
	sembrarResultado(t, pool, procesoID, importe, "0", "0", "0")
	ejecutar(`INSERT INTO resultados_obra (proceso_id, obra_id, puntos, importe, retenida)
	          VALUES ($1, $2, 10, $3, FALSE)`, procesoID, obraCompleta, pgDec(importe))
	ejecutar(`INSERT INTO resultados_titular (proceso_id, obra_id, titular_id, ipi, porcentaje, importe)
	          VALUES ($1, $2, $3, 'IPI-00000001', 100, $4)`, procesoID, obraCompleta, titularID, pgDec(importe))
}

// procesosConLiquidacion es el flujo de aprobaciones con la liquidacion
// cableada como en cmd/api, sobre el repositorio de liquidacion dado.
func procesosConLiquidacion(s *Store, ordenes aplicacion.RepositorioLiquidacion) aplicacion.Procesos {
	return aplicacion.Procesos{
		Repo: s, Unidad: s, Bitacora: s, Reloj: reloj.Sistema{},
		Liquidacion: aplicacion.Liquidaciones{
			Ordenes: ordenes, Reloj: reloj.Sistema{}, Notificador: s.AvisoPortal(), Bitacora: s, Unidad: s,
		},
	}
}

func etapaEnBase(t *testing.T, pool *pgxpool.Pool, procesoID string) (reparto.Etapa, int) {
	t.Helper()
	var etapa string
	var revision int
	if err := pool.QueryRow(t.Context(),
		`SELECT etapa, revision FROM procesos WHERE id = $1`, procesoID).Scan(&etapa, &revision); err != nil {
		t.Fatalf("leer la etapa de %s: %v", procesoID, err)
	}
	return reparto.Etapa(etapa), revision
}

// TestLaLiquidacionDelPeriodoEsperaALaUltimaCorrida es el ADR 0024 de punta a
// punta: dos bolsas nacionales del mismo periodo.
//
//  1. A entra a liquidacion_final y no emite: B sigue en verificacion.
//  2. A no puede salir hacia pago_registro -- 409, y la transaccion se
//     revierte de verdad: A sigue en liquidacion_final y no hay asiento --.
//  3. B entra y emite UNA orden de Ana por las dos corridas.
//  4. A ya sale, y la guarda no emite nada nuevo.
func TestLaLiquidacionDelPeriodoEsperaALaUltimaCorrida(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	sembrarCorridaEnVerificacion(t, pool, "prc-a", "bolsa-a", "2026-01", "30000")
	sembrarCorridaEnVerificacion(t, pool, "prc-b", "bolsa-b", "2026-01", "20000")
	uc := procesosConLiquidacion(s, s)

	if _, err := uc.AvanzarEtapa(ctx, "prc-a", usuarioDistribucion); err != nil {
		t.Fatalf("A entra a liquidacion_final: %v", err)
	}
	if etapa, rev := etapaEnBase(t, pool, "prc-a"); etapa != reparto.EtapaLiquidacionFinal || rev != 2 {
		t.Fatalf("A = %s/%d, se esperaba liquidacion_final/2", etapa, rev)
	}
	if ordenes, _ := s.Listar(ctx); len(ordenes) != 0 {
		t.Fatalf("%d ordenes con B todavia en verificacion", len(ordenes))
	}

	antes, err := s.De(ctx, aplicacion.RefProceso, "prc-a")
	if err != nil {
		t.Fatalf("asientos de A: %v", err)
	}
	_, err = uc.AvanzarEtapa(ctx, "prc-a", usuarioDistribucion)
	if !errors.Is(err, aplicacion.ErrLiquidacionEnEspera) {
		t.Fatalf("A hacia pago_registro: se esperaba ErrLiquidacionEnEspera, se obtuvo %v", err)
	}
	if etapa, _ := etapaEnBase(t, pool, "prc-a"); etapa != reparto.EtapaLiquidacionFinal {
		t.Fatalf("A = %s; la transicion rechazada no puede moverla", etapa)
	}
	despues, err := s.De(ctx, aplicacion.RefProceso, "prc-a")
	if err != nil {
		t.Fatalf("asientos de A: %v", err)
	}
	if len(despues) != len(antes) {
		t.Fatalf("asientos de A: %d -> %d; una transicion revertida no deja asiento", len(antes), len(despues))
	}

	if _, err := uc.AvanzarEtapa(ctx, "prc-b", usuarioDistribucion); err != nil {
		t.Fatalf("B entra a liquidacion_final: %v", err)
	}
	comprobarLiquidacionEmitida(t, s, pool, "prc-b", titularAna, pgDec("50000"))
	ordenes, err := s.Listar(ctx)
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if !slices.Equal(ordenes[0].Procesos, []string{"prc-a", "prc-b"}) || ordenes[0].ProcesoID != "prc-a" {
		t.Fatalf("orden = %+v, se esperaban las dos corridas con prc-a de referencia", ordenes[0])
	}

	p, err := uc.AvanzarEtapa(ctx, "prc-a", usuarioDistribucion)
	if err != nil {
		t.Fatalf("A hacia pago_registro con el periodo liquidado: %v", err)
	}
	if p.Etapa != reparto.EtapaPagoRegistro {
		t.Fatalf("A = %s, se esperaba pago_registro", p.Etapa)
	}
	comprobarLiquidacionEmitida(t, s, pool, "prc-a", titularAna, pgDec("50000"))
}

// TestLaCorridaQueLlegaTardeAUnPeriodoLiquidadoNoAvanza: el periodo se liquido
// con A; una bolsa registrada despues abre C. C no entra a
// liquidacion_final -- su dinero no esta en ninguna orden -- y el 409 lo dice.
func TestLaCorridaQueLlegaTardeAUnPeriodoLiquidadoNoAvanza(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	sembrarCorridaEnVerificacion(t, pool, "prc-a", "bolsa-a", "2026-01", "30000")
	uc := procesosConLiquidacion(s, s)
	if _, err := uc.AvanzarEtapa(ctx, "prc-a", usuarioDistribucion); err != nil {
		t.Fatalf("A entra a liquidacion_final: %v", err)
	}

	sembrarCorridaEnVerificacion(t, pool, "prc-c", "bolsa-c", "2026-01", "10000")
	_, err := uc.AvanzarEtapa(ctx, "prc-c", usuarioDistribucion)
	if !errors.Is(err, aplicacion.ErrPeriodoYaLiquidado) {
		t.Fatalf("se esperaba ErrPeriodoYaLiquidado, se obtuvo %v", err)
	}
	if etapa, rev := etapaEnBase(t, pool, "prc-c"); etapa != reparto.EtapaVerificacion || rev != 1 {
		t.Fatalf("C = %s/%d; tiene que seguir en verificacion/1", etapa, rev)
	}
	comprobarLiquidacionEmitida(t, s, pool, "prc-a", titularAna, pgDec("30000"))
}

// TestDosCorridasQueEntranALaVezEmitenUnaSolaLiquidacion es la carrera que la
// espera tiene que resistir: las dos ultimas corridas del periodo entran a
// liquidacion_final a la vez. Cada una ya escribio su etapa y ninguna ha
// confirmado, asi que sin serializacion las dos verian a la otra en
// verificacion, las dos esperarian, y el periodo no se liquidaria nunca.
//
// La cita antes del cerrojo del periodo pone a las dos en el tramo critico a
// la vez. La primera en tomar el cerrojo ve a la otra sin confirmar y espera;
// la segunda lo toma despues del commit de la primera, la ve en
// liquidacion_final y emite por las dos.
func TestDosCorridasQueEntranALaVezEmitenUnaSolaLiquidacion(t *testing.T) {
	// Dos transacciones a la vez necesitan dos conexiones: el pool de
	// testhelp deja una, y la segunda esperaria la conexion y no el cerrojo.
	cfg, err := pgxpool.ParseConfig(testhelp.DSN(t))
	if err != nil {
		t.Fatalf("configurar el pool concurrente: %v", err)
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("abrir el pool concurrente: %v", err)
	}
	t.Cleanup(pool.Close)
	s := sembrarEn(t, pool)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	sembrarCorridaEnVerificacion(t, pool, "prc-a", "bolsa-a", "2026-01", "30000")
	sembrarCorridaEnVerificacion(t, pool, "prc-b", "bolsa-b", "2026-01", "20000")

	uc := procesosConLiquidacion(s, repoConCita{Store: s, cita: citaDe(2)})

	var grupo sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"prc-a", "prc-b"} {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			_, errs[i] = uc.AvanzarEtapa(ctx, id, usuarioDistribucion)
		}()
	}
	grupo.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("avance concurrente %d: %v", i, err)
		}
	}
	for _, id := range []string{"prc-a", "prc-b"} {
		if etapa, _ := etapaEnBase(t, pool, id); etapa != reparto.EtapaLiquidacionFinal {
			t.Fatalf("%s = %s, se esperaba liquidacion_final", id, etapa)
		}
	}
	comprobarLiquidacionEmitida(t, s, pool, "prc-a", titularAna, pgDec("50000"))
}

// TestAvisoPortalSeNiegaFueraDeUnaUnidad: un aviso confirmado por su cuenta
// sobreviviria a una orden revertida.
func TestAvisoPortalSeNiegaFueraDeUnaUnidad(t *testing.T) {
	s, pool := sembrar(t)
	sembrarFirmantes(t, pool)
	sembrarBolsa(t, pool, bolsaNac, "2026", "nacional")
	sembrarProcesoListo(t, pool, procesoNac, bolsaNac, "2026", "nacional")

	_, err := s.AvisoPortal().Notificar(t.Context(), aplicacion.Aviso{
		TitularID: titularAna, ProcesoID: procesoNac, Asunto: "a", Cuerpo: "c",
	})
	if !errors.Is(err, errAvisoFueraDeUnidad) {
		t.Fatalf("se esperaba errAvisoFueraDeUnidad, se obtuvo %v", err)
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM notificaciones`).Scan(&n); err != nil {
		t.Fatalf("contar notificaciones: %v", err)
	}
	if n != 0 {
		t.Fatalf("%d notificaciones escritas fuera de una unidad", n)
	}

	// Dentro de una unidad escribe, y el acuse es la huella del contenido:
	// el mismo aviso da el mismo acuse.
	var acuse string
	if err := s.EnUnidad(t.Context(), func(ctx context.Context) error {
		var err error
		acuse, err = s.AvisoPortal().Notificar(ctx, aplicacion.Aviso{
			TitularID: titularAna, ProcesoID: procesoNac, Asunto: "a", Cuerpo: "c",
		})
		return err
	}); err != nil {
		t.Fatalf("avisar dentro de una unidad: %v", err)
	}
	var guardado string
	if err := pool.QueryRow(t.Context(),
		`SELECT acuse FROM notificaciones WHERE titular_id = $1 AND proceso_id = $2 AND via = 'portal'`,
		titularAna, procesoNac).Scan(&guardado); err != nil {
		t.Fatalf("leer el aviso: %v", err)
	}
	if guardado != acuse || len(acuse) != len("portal:")+64 {
		t.Fatalf("acuse devuelto %q, guardado %q", acuse, guardado)
	}
}

// TestCorridasDePeriodoDevuelveTodasConSusFirmas es el contrato del adaptador:
// todas las corridas del periodo y circuito, en cualquier etapa, ordenadas por
// id, con su bolsa y TODAS sus firmas. Filtrar es del caso de uso.
func TestCorridasDePeriodoDevuelveTodasConSusFirmas(t *testing.T) {
	s, pool := sembrar(t)
	sembrarFirmantes(t, pool)
	sembrarCorridaEnVerificacion(t, pool, "prc-b", "bolsa-b", "2026-01", "10")
	sembrarBolsa(t, pool, "bolsa-a", "2026-01", "nacional")
	sembrarProcesoListo(t, pool, "prc-a", "bolsa-a", "2026-01", "nacional")
	sembrarBolsa(t, pool, "bolsa-feb", "2026-02", "nacional")
	sembrarProcesoListo(t, pool, "prc-feb", "bolsa-feb", "2026-02", "nacional")
	sembrarBolsa(t, pool, "bolsa-int", "2026-01", "internacional")
	sembrarProcesoListo(t, pool, "prc-int", "bolsa-int", "2026-01", "internacional")

	metas, err := s.CorridasDePeriodo(t.Context(), "2026-01", reparto.Nacional)
	if err != nil {
		t.Fatalf("CorridasDePeriodo: %v", err)
	}
	if len(metas) != 2 || metas[0].ID != "prc-a" || metas[1].ID != "prc-b" {
		t.Fatalf("metas = %+v, se esperaban prc-a y prc-b en ese orden", metas)
	}
	a, b := metas[0], metas[1]
	if a.Etapa != reparto.EtapaLiquidacionFinal || a.Revision != 2 || a.BolsaID != "bolsa-a" || len(a.Firmas) != 2 {
		t.Fatalf("prc-a = %+v", a)
	}
	if !a.CerroLaVerificacion() {
		t.Fatal("prc-a tiene las dos firmas de la revision 1 y esta en la 2: cerro la verificacion")
	}
	if b.Etapa != reparto.EtapaVerificacion || b.Revision != 1 || len(b.Firmas) != 2 || b.CerroLaVerificacion() {
		t.Fatalf("prc-b = %+v; firmada pero todavia en la compuerta", b)
	}

	vacio, err := s.CorridasDePeriodo(t.Context(), "2030-01", reparto.Nacional)
	if err != nil || vacio == nil || len(vacio) != 0 {
		t.Fatalf("periodo sin corridas = %v, %v; se esperaba una lista vacia", vacio, err)
	}
}

// TestLaLiquidacionEmiteAvisosConLaCorridaQueAportaSoloAlTitular comprueba el
// escenario de #196: prc-a aporta solo a Ana y prc-b aporta solo a Beto.
// Cuando ambas corridas se liquidan, la fila de `notificaciones` de Beto debe
// apuntar a prc-b (y no a prc-a, que es la primera lexicografica del periodo pero
// no le aporta nada).
func TestLaLiquidacionEmiteAvisosConLaCorridaQueAportaSoloAlTitular(t *testing.T) {
	s, pool := sembrar(t)
	ctx := t.Context()
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	sembrarCorridaEnVerificacionTitular(t, pool, "prc-a", "bolsa-a", "2026-01", "30000", titularAna)
	sembrarCorridaEnVerificacionTitular(t, pool, "prc-b", "bolsa-b", "2026-01", "20000", titularBeto)
	uc := procesosConLiquidacion(s, s)

	if _, err := uc.AvanzarEtapa(ctx, "prc-a", usuarioDistribucion); err != nil {
		t.Fatalf("A entra a liquidacion_final: %v", err)
	}
	if _, err := uc.AvanzarEtapa(ctx, "prc-b", usuarioDistribucion); err != nil {
		t.Fatalf("B entra a liquidacion_final: %v", err)
	}

	// Ana solo estuvo en prc-a: su orden y su aviso deben ser prc-a
	var avisoAnaProceso string
	if err := pool.QueryRow(ctx,
		`SELECT proceso_id FROM notificaciones WHERE titular_id = $1 AND via = 'portal'`, titularAna).Scan(&avisoAnaProceso); err != nil {
		t.Fatalf("aviso Ana: %v", err)
	}
	if avisoAnaProceso != "prc-a" {
		t.Fatalf("aviso Ana apunto a %q, se esperaba prc-a", avisoAnaProceso)
	}

	// Beto solo estuvo en prc-b: su orden y su aviso deben ser prc-b
	var avisoBetoProceso string
	if err := pool.QueryRow(ctx,
		`SELECT proceso_id FROM notificaciones WHERE titular_id = $1 AND via = 'portal'`, titularBeto).Scan(&avisoBetoProceso); err != nil {
		t.Fatalf("aviso Beto: %v", err)
	}
	if avisoBetoProceso != "prc-b" {
		t.Fatalf("aviso Beto apunto a %q, se esperaba prc-b (la corrida que le aporto)", avisoBetoProceso)
	}
}

type authPrueba struct {
	usuarios map[string]aplicacion.Usuario
}

func (a authPrueba) IniciarSesion(context.Context, string, string) (aplicacion.Sesion, error) {
	return aplicacion.Sesion{}, nil
}
func (a authPrueba) ResolverSesion(_ context.Context, token string) (aplicacion.Usuario, error) {
	u, hay := a.usuarios[token]
	if !hay {
		return aplicacion.Usuario{}, aplicacion.ErrCredenciales
	}
	return u, nil
}
func (a authPrueba) CerrarSesion(context.Context, string) error { return nil }

// TestAvanzarEtapaDisparaLiquidacionPuntaAPuntaHTTP comprueba por HTTP de punta a
// punta: avanzar etapas de corridas hasta liquidacion_final, y consultar
// GET /liquidaciones (administrador) y GET /mis-liquidaciones (titular) sobre el
// servidor HTTP real cableado contra Postgres real.
func TestAvanzarEtapaDisparaLiquidacionPuntaAPuntaHTTP(t *testing.T) {
	s, pool := sembrar(t)
	sembrarFirmantes(t, pool)
	sembrarSMMLV(t, pool)
	sembrarCorridaEnVerificacion(t, pool, "prc-a", "bolsa-a", "2026-01", "30000")
	sembrarCorridaEnVerificacion(t, pool, "prc-b", "bolsa-b", "2026-01", "20000")

	liq := aplicacion.Liquidaciones{
		Ordenes: s, Reloj: reloj.Sistema{}, Notificador: s.AvisoPortal(), Bitacora: s, Unidad: s,
	}
	proc := procesosConLiquidacion(s, s)

	auth := authPrueba{
		usuarios: map[string]aplicacion.Usuario{
			"tok-admin": {ID: usuarioAdmin, Rol: aplicacion.RolAdministrador},
			"tok-ana":   {ID: usuarioTitular, Rol: aplicacion.RolTitular, TitularID: titularAna},
		},
	}

	api := httpapi.Nueva(httpapi.Casos{
		Auth:     auth,
		Procesos: proc,
		Ordenes:  liq,
	}, httpapi.Opciones{})
	router := api.Router()

	// 1. POST /procesos/prc-a/avanzar como admin
	reqA := httptest.NewRequest(http.MethodPost, "/procesos/prc-a/avanzar", nil)
	reqA.Header.Set("Authorization", "Bearer tok-admin")
	recA := httptest.NewRecorder()
	router.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("POST /procesos/prc-a/avanzar = %d; cuerpo: %s", recA.Code, recA.Body)
	}

	// 2. POST /procesos/prc-b/avanzar como admin -> entra a liquidacion_final y emite el periodo
	reqB := httptest.NewRequest(http.MethodPost, "/procesos/prc-b/avanzar", nil)
	reqB.Header.Set("Authorization", "Bearer tok-admin")
	recB := httptest.NewRecorder()
	router.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("POST /procesos/prc-b/avanzar = %d; cuerpo: %s", recB.Code, recB.Body)
	}

	// 3. GET /liquidaciones como admin
	reqLiq := httptest.NewRequest(http.MethodGet, "/liquidaciones", nil)
	reqLiq.Header.Set("Authorization", "Bearer tok-admin")
	recLiq := httptest.NewRecorder()
	router.ServeHTTP(recLiq, reqLiq)
	if recLiq.Code != http.StatusOK {
		t.Fatalf("GET /liquidaciones = %d; cuerpo: %s", recLiq.Code, recLiq.Body)
	}

	var listadoAdmin struct {
		Liquidaciones []struct {
			ID        string   `json:"id"`
			TitularID string   `json:"titular_id"`
			Neto      string   `json:"neto"`
			Procesos  []string `json:"procesos"`
		} `json:"liquidaciones"`
	}
	if err := json.Unmarshal(recLiq.Body.Bytes(), &listadoAdmin); err != nil {
		t.Fatalf("deserializar GET /liquidaciones: %v", err)
	}
	if len(listadoAdmin.Liquidaciones) != 1 {
		t.Fatalf("GET /liquidaciones: se esperaba 1 orden, llegaron %d", len(listadoAdmin.Liquidaciones))
	}
	ord := listadoAdmin.Liquidaciones[0]
	if ord.TitularID != titularAna || ord.Neto != "50000.00" {
		t.Fatalf("GET /liquidaciones = %+v, se esperaba Ana con 50000.00", ord)
	}

	// 4. GET /mis-liquidaciones como Ana
	reqMis := httptest.NewRequest(http.MethodGet, "/mis-liquidaciones", nil)
	reqMis.Header.Set("Authorization", "Bearer tok-ana")
	recMis := httptest.NewRecorder()
	router.ServeHTTP(recMis, reqMis)
	if recMis.Code != http.StatusOK {
		t.Fatalf("GET /mis-liquidaciones = %d; cuerpo: %s", recMis.Code, recMis.Body)
	}

	var listadoTitular struct {
		Liquidaciones []struct {
			ID        string `json:"id"`
			TitularID string `json:"titular_id"`
			Neto      string `json:"neto"`
		} `json:"liquidaciones"`
	}
	if err := json.Unmarshal(recMis.Body.Bytes(), &listadoTitular); err != nil {
		t.Fatalf("deserializar GET /mis-liquidaciones: %v", err)
	}
	if len(listadoTitular.Liquidaciones) != 1 || listadoTitular.Liquidaciones[0].ID != ord.ID {
		t.Fatalf("GET /mis-liquidaciones = %+v, se esperaba la orden %s", listadoTitular, ord.ID)
	}
}
