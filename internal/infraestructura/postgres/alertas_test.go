package postgres

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Fixture del #37: un periodo con una anomalia de CADA tipo, sembrado sobre
// las obras de sembrar() -- que ya trae las cuatro combinaciones de
// declaracion que R-04 distingue (semilla_test.go).
//
// Se apoya en el esquema real y no en un doble a proposito (ADR 0010): la
// idempotencia de la evaluacion la decide el UNIQUE de la migracion 00017, y
// un mock no tiene ese UNIQUE.
const (
	periodoAlertas = "2025-01"

	repA = "rep-a" // caracol, 2025-01, huella aa
	repB = "rep-b" // caracol, 2025-01, huella bb
	repC = "rep-c" // netflix, 2024-12, huella aa -> colisiona con repA
)

// insertarUsoDeAlertas siembra una fila canonica con control sobre el escalon,
// la obra y los campos que forman la clave logica de registro.
//
// Por SQL directo y no por GuardarUsos: hace falta poner `escalon` y `obra_id`
// a mano -- GuardarUsos siembra todo como pendiente -- y el CHECK
// `uso_resuelto_tiene_obra` (migracion 00007) exige que oni y obra_id cuadren
// con el escalon, asi que la coherencia se deriva aqui en vez de dejarla al
// caso de cada llamada.
func insertarUsoDeAlertas(
	t *testing.T, pool *pgxpool.Pool,
	id, reporteID, escalon, obraID, tipoObra, idsFuente, fecha, hora string,
) {
	t.Helper()

	var obra *string
	if obraID != "" {
		obra = &obraID
	}
	// La bandera la fija el CHECK, no el escalon: una fila sin obra lleva
	// oni=TRUE salvo que este excluida. O sea que `u-pendiente` -- que NADIE
	// ha intentado identificar todavia -- se siembra con oni=TRUE, que es
	// exactamente la trampa por la que el detector filtra por escalon y no por
	// la bandera. Si el esquema no obligara a esto, la prueba no cubriria el
	// caso que importa.
	oni := obraID == "" && escalon != identificacion.EscalonExcluido

	_, err := pool.Exec(t.Context(),
		`INSERT INTO usos (id, reporte_id, fuente, titulo, ids_fuente, obra_id, escalon, oni,
		                    modalidad, tipo_obra, fecha, hora, emisiones)
		 VALUES ($1, $2, 'caracol', 'Titulo de prueba', $3, $4, $5, $6, 'tv', $7, $8, $9, 1)`,
		id, reporteID, idsFuente, obra, escalon, oni, tipoObra, fecha, hora)
	if err != nil {
		t.Fatalf("insertar uso %q: %v", id, err)
	}
}

// sembrarPeriodoConAnomalias deja un periodo con las seis anomalias y devuelve
// el Store y el pool.
func sembrarPeriodoConAnomalias(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	s, pool := sembrar(t)
	ejecutar := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatalf("sembrar el periodo (%s): %v", sql, err)
		}
	}

	// repA y repC comparten los mismos bytes SIN compartir fuente ni periodo:
	// es exactamente lo que el UNIQUE (sha256, fuente) de `reportes` deja
	// pasar, y por eso se puede sembrar.
	ejecutar(`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes) VALUES
	            ($1, 'caracol', $4, repeat('a', 64), 'reportes/a.csv', 100),
	            ($2, 'caracol', $4, repeat('b', 64), 'reportes/b.csv', 100),
	            ($3, 'netflix', '2024-12', repeat('a', 64), 'reportes/c.csv', 100)`,
		repA, repB, repC, periodoAlertas)

	// 1. ONI.
	insertarUsoDeAlertas(t, pool, "u-oni", repA, identificacion.EscalonONI, "", "serie", "id_ficha=1", "2025-01-01", "10:00:00")
	// Un excluido y un pendiente que NO pueden salir como ONI (R-27 y la
	// bandera oni=TRUE por defecto).
	insertarUsoDeAlertas(t, pool, "u-excluido", repA, identificacion.EscalonExcluido, "", "serie", "id_ficha=2", "2025-01-01", "11:00:00")
	insertarUsoDeAlertas(t, pool, "u-pendiente", repA, identificacion.EscalonPendiente, "", "serie", "id_ficha=3", "2025-01-01", "12:00:00")

	// 3. Duplicado de registro: misma clave logica en dos entregas distintas.
	insertarUsoDeAlertas(t, pool, "u-dup1", repA, identificacion.EscalonAlias, obraCompleta, "serie", "id_ficha=7", "2025-01-02", "20:00:00")
	insertarUsoDeAlertas(t, pool, "u-dup2", repB, identificacion.EscalonAlias, obraCompleta, "serie", "id_ficha=7", "2025-01-02", "20:00:00")
	// Y la emision legitima del MISMO programa a otra hora, que no lo es.
	insertarUsoDeAlertas(t, pool, "u-otraemision", repA, identificacion.EscalonAlias, obraCompleta, "serie", "id_ficha=7", "2025-01-02", "22:00:00")

	// 6. tipo_obra sin mapear, sobre una obra identificada.
	insertarUsoDeAlertas(t, pool, "u-sintipo", repA, identificacion.EscalonAlias, obraCompleta, "", "id_ficha=8", "2025-01-03", "21:00:00")

	// 4 y 5 entran por sus obras: obraIncompleta declara 60 y su coautor de
	// catalogo (IPI-00000002) no tiene parte.
	insertarUsoDeAlertas(t, pool, "u-inc", repA, identificacion.EscalonAlias, obraIncompleta, "cinematografica", "id_ficha=9", "2025-01-04", "19:00:00")

	return s, pool
}

// servicioDeAnomalias cablea el caso de uso contra el Store real, con reloj
// fijo: el ADR 0002 mete el instante por el puerto justamente para que la
// prueba pueda fijarlo.
func servicioDeAnomalias(s *Store, instante time.Time) aplicacion.Anomalias {
	return aplicacion.Anomalias{
		Entregas:      s,
		Declaraciones: s,
		Coautores:     s,
		Alertas:       s,
		Bitacora:      s,
		Unidad:        s,
		Reloj:         reloj.Fijo{Instante: instante},
	}
}

var instanteAlertas = time.Date(2026, 5, 2, 8, 30, 0, 0, time.UTC)

func refsDeAlertas(alertas []aplicacion.Alerta) []string {
	out := make([]string, 0, len(alertas))
	for _, a := range alertas {
		ref := a.Tipo + "|" + a.RefTipo + ":" + a.RefID
		if a.RefTitular != "" {
			ref += "#" + a.RefTitular
		}
		out = append(out, ref)
	}
	slices.Sort(out)
	return out
}

// El criterio de aceptacion entero, contra Postgres: sembrar un periodo con
// una de cada anomalia, evaluar, y comprobar que quedan las filas de alerta
// con las referencias correctas.
func TestEvaluarAnomaliasSobrePostgres(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	resumen, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}

	quiero := []string{
		// repA (2025-01, caracol) colisiona con repC (2024-12, netflix): la
		// alerta va sobre la entrega DEL periodo evaluado.
		"duplicado_archivo|reporte:" + repA,
		// La segunda fila con la misma clave logica, no la primera.
		"duplicado_registro|uso:u-dup2",
		"oni|uso:u-oni",
		// Las tres obras que el periodo pondera y no tienen declaracion
		// completa. obraCompleta no aparece.
		"reserva_declaracion_incompleta|obra:" + obraIncompleta,
		"tipo_obra_sin_mapear|uso:u-sintipo",
		// El coautor de catalogo de obraIncompleta que no tiene parte. La
		// segunda coordenada lleva su espacio de nombres: este caso referencia
		// un IPI y el otro caso del mismo detector un titulares.id, y sin
		// discriminarlos la clave natural los colapsa.
		"titular_sin_porcentaje|obra:" + obraIncompleta + "#" + anomalias.PrefijoIPI + "IPI-00000002",
	}
	if got := refsDeAlertas(alertas); !slices.Equal(got, quiero) {
		t.Fatalf("alertas = %v,\nse esperaba %v", got, quiero)
	}
	if resumen.Detectadas != len(quiero) || resumen.Nuevas != len(quiero) {
		t.Fatalf("resumen = %d detectadas / %d nuevas, se esperaban %d",
			resumen.Detectadas, resumen.Nuevas, len(quiero))
	}

	// Cada fila lleva su id de la base, su instante del Reloj y su detalle.
	for _, a := range alertas {
		if a.ID == "" {
			t.Fatalf("la alerta %q no trae id", a.Tipo)
		}
		if !a.Detectada.Equal(instanteAlertas) {
			t.Fatalf("la alerta %q se detecto en %v, el reloj marca %v", a.Tipo, a.Detectada, instanteAlertas)
		}
		if strings.TrimSpace(a.Detalle) == "" {
			t.Fatalf("la alerta %q no trae detalle", a.Tipo)
		}
		if a.Resuelta || a.ResueltaPor != "" || a.ResueltaEn != nil {
			t.Fatalf("la alerta %q nace resuelta: %+v", a.Tipo, a)
		}
	}
}

// La idempotencia hay que PROVOCARLA: testhelp toma la plantilla con las
// tablas vacias y restaura antes de cada prueba, asi que el harness nunca
// ejercita filas preexistentes por su cuenta. Sin esta llamada doble
// explicita, el verde no prueba nada.
func TestEvaluarAnomaliasDosVecesNoDuplicaFilas(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	primera, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	// La segunda pasada va con OTRO instante: si la clave natural incluyera
	// `detectada`, esto escribiria una segunda tanda entera.
	svcTarde := servicioDeAnomalias(s, instanteAlertas.Add(24*time.Hour))
	segunda, err := svcTarde.Evaluar(t.Context(), periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}

	var filas int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM alertas`).Scan(&filas); err != nil {
		t.Fatalf("contar alertas: %v", err)
	}
	if filas != primera.Nuevas {
		t.Fatalf("hay %d filas tras dos pasadas, se esperaban %d", filas, primera.Nuevas)
	}
	if segunda.Nuevas != 0 {
		t.Fatalf("la segunda pasada escribio %d filas nuevas", segunda.Nuevas)
	}
	if segunda.Detectadas != primera.Detectadas {
		t.Fatalf("la segunda pasada detecto %d y la primera %d", segunda.Detectadas, primera.Detectadas)
	}

	// Y `detectada` sigue siendo la de la PRIMERA deteccion: DO NOTHING, no
	// DO UPDATE. Cuando se vio por primera vez es el dato que quiere quien
	// persigue la alerta.
	var detectada time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT detectada FROM alertas ORDER BY id LIMIT 1`).Scan(&detectada); err != nil {
		t.Fatalf("leer detectada: %v", err)
	}
	if !detectada.Equal(instanteAlertas) {
		t.Fatalf("detectada = %v, se esperaba la primera pasada (%v)", detectada, instanteAlertas)
	}
}

// Una alerta que una persona ya cerro no se reabre al volver a detectarla. La
// resolucion de #39 actua sobre el REGISTRO OFENSOR; si la anomalia sigue
// ahi, reabrirla borraria la decision de quien la cerro sin que nadie lo
// pidiera. Es limitacion conocida, declarada en el ADR 0021.
func TestEvaluarAnomaliasNoReabreUnaAlertaResuelta(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	id := alertas[0].ID
	if _, err := svc.Resolver(t.Context(), id, usuarioAdmin, "revisada"); err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}

	tras, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar tras la segunda pasada: %v", err)
	}
	for _, a := range tras {
		if a.ID != id {
			continue
		}
		if !a.Resuelta || a.ResueltaPor != usuarioAdmin || a.Nota != "revisada" {
			t.Fatalf("la alerta resuelta volvio a abrirse: %+v", a)
		}
		return
	}
	t.Fatalf("la alerta %q desaparecio tras la segunda pasada", id)
}

// ADR 0006: el asiento es parte de la definicion de hecho. La pasada y su
// asiento entran en la misma transaccion, y el asiento nombra el periodo.
func TestEvaluarAnomaliasAsientaLaPasada(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	asientos, err := s.De(t.Context(), aplicacion.RefPeriodo, periodoAlertas)
	if err != nil {
		t.Fatalf("De: %v", err)
	}
	if len(asientos) != 1 {
		t.Fatalf("se esperaba un asiento, hubo %d", len(asientos))
	}
	a := asientos[0]
	if a.Hecho != aplicacion.HechoAnomaliasEvaluadas || a.ActorID != usuarioAdmin {
		t.Fatalf("asiento = %q firmado por %q", a.Hecho, a.ActorID)
	}
	if !strings.Contains(string(a.Payload), `"por_tipo"`) {
		t.Fatalf("el payload no lleva el desglose: %s", a.Payload)
	}
}

// Y la pasada entera se revierte si su asiento falla: ADR 0006, "un caso de
// uso cuyo asiento fallo no esta hecho".
//
// Este es el unico sitio donde eso se puede comprobar de verdad. El doble de
// la capa de aplicacion corre `fn(ctx)` tal cual -- no propaga ninguna
// transaccion en el contexto --, asi que un asiento escrito FUERA de la unidad
// le resulta indistinguible de uno escrito dentro. Aqui no: si no comparten
// transaccion, las alertas quedan escritas y el asiento no.
func TestEvaluarAnomaliasSeRevierteEnteraSiElAsientoFalla(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	// Un actor que no esta en `usuarios` revienta la FK de `asientos`, que es
	// la escritura que va DESPUES de las alertas.
	if _, err := svc.Evaluar(t.Context(), periodoAlertas, "usr-fantasma"); err == nil {
		t.Fatal("Evaluar no fallo con un actor que no existe")
	}

	var filas int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM alertas`).Scan(&filas); err != nil {
		t.Fatalf("contar alertas: %v", err)
	}
	if filas != 0 {
		t.Fatalf("quedaron %d alertas escritas pese a que el asiento fallo", filas)
	}
}

// Y en la otra direccion: si la unidad se revierte, NO sobrevive el asiento.
//
// Es la mitad que la prueba de arriba no puede ver. [Store.EnUnidad] es
// reentrante, asi que abrir una unidad AQUI y hacer que Evaluar corra dentro
// pone las dos escrituras en la misma transaccion; abortar despues tiene que
// llevarse las dos. Un asiento escrito contra el pool -- porque alguien le
// pase un contexto que no lleve la transaccion -- sobrevivira al rollback y
// dejaria la bitacora afirmando una pasada que no ocurrio.
func TestUnaPasadaRevertidaNoDejaAsientoHuerfano(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	abortar := errors.New("abortada a proposito")
	err := s.EnUnidad(t.Context(), func(ctx context.Context) error {
		if _, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin); err != nil {
			return err
		}
		return abortar
	})
	if !errors.Is(err, abortar) {
		t.Fatalf("EnUnidad devolvio %v", err)
	}

	var alertas, asientos int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM alertas`).Scan(&alertas); err != nil {
		t.Fatalf("contar alertas: %v", err)
	}
	if err := pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM asientos WHERE hecho = $1`,
		aplicacion.HechoAnomaliasEvaluadas).Scan(&asientos); err != nil {
		t.Fatalf("contar asientos: %v", err)
	}
	if alertas != 0 {
		t.Fatalf("sobrevivieron %d alertas a la reversion", alertas)
	}
	if asientos != 0 {
		t.Fatalf("sobrevivieron %d asientos a la reversion: el asiento no comparte transaccion", asientos)
	}
}

// Resolver escribe el estado Y su asiento en la MISMA transaccion. Este es el
// unico sitio donde se puede probar que el rollback de verdad ocurre: el doble
// de la capa de aplicacion no tiene nada que revertir.
func TestResolverAlertaYSuAsientoSonUnaSolaTransaccion(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	id := alertas[0].ID

	// Un actor que no esta en `usuarios` revienta la FK de `asientos`, que es
	// la SEGUNDA escritura: si las dos no compartieran transaccion, la alerta
	// quedaria resuelta y sin asiento.
	if _, err := svc.Resolver(t.Context(), id, "usr-fantasma", "nota de prueba"); err == nil {
		t.Fatal("Resolver no fallo con un actor que no existe")
	}

	var resuelta bool
	if err := pool.QueryRow(t.Context(),
		`SELECT resuelta FROM alertas WHERE id = $1::uuid`, id).Scan(&resuelta); err != nil {
		t.Fatalf("releer la alerta: %v", err)
	}
	if resuelta {
		t.Fatal("la alerta quedo resuelta pese a que el asiento fallo")
	}
}

func TestResolverAlertaFirmaYDistingueLaSegundaVez(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)

	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodoAlertas})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	id := alertas[0].ID

	resuelta, err := svc.Resolver(t.Context(), id, usuarioAdmin, "asignada a mano")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if !resuelta.Resuelta || resuelta.ResueltaPor != usuarioAdmin || resuelta.Nota != "asignada a mano" {
		t.Fatalf("resuelta = %+v", resuelta)
	}
	if resuelta.ResueltaEn == nil || !resuelta.ResueltaEn.Equal(instanteAlertas) {
		t.Fatalf("resuelta_en = %v, el reloj marca %v", resuelta.ResueltaEn, instanteAlertas)
	}

	// Dos personas mirando el mismo tablero es el caso normal. Quien llega
	// segundo tiene que saber que la firma escrita no es la suya.
	if _, err := svc.Resolver(t.Context(), id, usuarioTitular, "nota de prueba"); !errors.Is(err, aplicacion.ErrAlertaYaResuelta) {
		t.Fatalf("la segunda resolucion dio %v", err)
	}
	// Y un id que no existe no es lo mismo.
	const idInventado = "00000000-0000-0000-0000-000000000000"
	if _, err := svc.Resolver(t.Context(), idInventado, usuarioAdmin, "nota de prueba"); !errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("un id inventado dio %v", err)
	}
}

// Un id que no es un UUID no puede salir como "no encontrado": el error es de
// FORMATO y el 404 de arriba se reserva para el id bien formado que no existe.
func TestResolverAlertaConUnIDQueNoEsUUID(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)

	_, err := s.ResolverAlerta(t.Context(), "no-soy-un-uuid", usuarioAdmin, "", instanteAlertas)
	if err == nil {
		t.Fatal("ResolverAlerta acepto un id que no es UUID")
	}
	if errors.Is(err, aplicacion.ErrNoEncontrado) {
		t.Fatalf("un id mal formado salio como no encontrado: %v", err)
	}
}

func TestListarAlertasFiltra(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	soloONI, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
		Periodo: periodoAlertas, Tipo: anomalias.TipoONI,
	})
	if err != nil {
		t.Fatalf("ListarAlertas por tipo: %v", err)
	}
	if len(soloONI) != 1 || soloONI[0].RefID != "u-oni" {
		t.Fatalf("filtro por tipo = %v", refsDeAlertas(soloONI))
	}

	// Otro periodo no tiene alertas, y eso es la lista vacia, no un 404.
	otro, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{Periodo: "2024-12"})
	if err != nil {
		t.Fatalf("ListarAlertas de otro periodo: %v", err)
	}
	if len(otro) != 0 {
		t.Fatalf("el periodo sin evaluar devolvio %d alertas", len(otro))
	}

	// El filtro de resueltas es un PUNTERO: sin el no filtra, y con el
	// distingue las dos mitades.
	abiertas := false
	sinResolver, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
		Periodo: periodoAlertas, Resueltas: &abiertas,
	})
	if err != nil {
		t.Fatalf("ListarAlertas sin resolver: %v", err)
	}
	if len(sinResolver) != 6 {
		t.Fatalf("sin resolver hay %d alertas, se esperaban 6", len(sinResolver))
	}

	if _, err := svc.Resolver(t.Context(), sinResolver[0].ID, usuarioAdmin, "nota de prueba"); err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	cerradas := true
	resueltas, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
		Periodo: periodoAlertas, Resueltas: &cerradas,
	})
	if err != nil {
		t.Fatalf("ListarAlertas resueltas: %v", err)
	}
	if len(resueltas) != 1 {
		t.Fatalf("hay %d resueltas, se esperaba 1", len(resueltas))
	}
}

// El predicado que consumira la compuerta de #34: cuantas alertas cuyo tipo
// BLOQUEA siguen abiertas en el periodo.
func TestCriticasAbiertasSobrePostgres(t *testing.T) {
	s, _ := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	// De las seis: duplicado de archivo, duplicado de registro y tipo_obra
	// sin mapear. ONI y las dos de declaracion son estados validos del modelo.
	n, err := svc.CriticasAbiertas(t.Context(), periodoAlertas)
	if err != nil {
		t.Fatalf("CriticasAbiertas: %v", err)
	}
	if n != 3 {
		t.Fatalf("CriticasAbiertas = %d, se esperaban 3", n)
	}

	// Una lista de tipos vacia cuenta TODAS, que es el contrato del puerto.
	todas, err := s.ContarAlertasSinResolver(t.Context(), periodoAlertas, nil)
	if err != nil {
		t.Fatalf("ContarAlertasSinResolver: %v", err)
	}
	if todas != 6 {
		t.Fatalf("sin filtro de tipos = %d, se esperaban 6", todas)
	}
}

// ---------------------------------------------------------------------------
// Invariantes del esquema (migracion 00017)

// El CHECK que impide marcar una alerta como resuelta sin decir quien y
// cuando. Es la mitad del ADR 0006 que la base puede sostener por si sola.
func TestElEsquemaExigeFirmaEnUnaAlertaResuelta(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	_, err := pool.Exec(t.Context(), `UPDATE alertas SET resuelta = TRUE`)
	if err == nil {
		t.Fatal("el esquema dejo marcar una alerta como resuelta sin firma")
	}
	if !strings.Contains(err.Error(), "alerta_resuelta_tiene_firma") {
		t.Fatalf("fallo por otra restriccion: %v", err)
	}
}

// Resolver sin nota tampoco cabe en la base (revision de #158, punto 4).
func TestElEsquemaExigeNotaEnUnaAlertaResuelta(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	_, err := pool.Exec(t.Context(),
		`UPDATE alertas SET resuelta = TRUE, resuelta_por = $1, resuelta_en = now(), nota = '  '`, usuarioAdmin)
	if err == nil {
		t.Fatal("el esquema dejo resolver una alerta sin nota")
	}
	if !strings.Contains(err.Error(), "alerta_resuelta_tiene_nota") {
		t.Fatalf("fallo por otra restriccion: %v", err)
	}
}

// ref_titular pertenece al detector de titulares y a ningun otro. Sin este
// CHECK seria una convencion, y una convencion no impide que otro detector la
// use para otra cosa y rompa la clave natural.
func TestElEsquemaReservaRefTitularAlDetectorDeTitulares(t *testing.T) {
	_, pool := sembrarPeriodoConAnomalias(t)

	_, err := pool.Exec(t.Context(),
		`INSERT INTO alertas (periodo, tipo, ref_tipo, ref_id, ref_titular, detalle, detectada)
		 VALUES ($1, 'oni', 'uso', 'u-oni', 'IPI-1', 'x', now())`, periodoAlertas)
	if err == nil {
		t.Fatal("el esquema dejo poner ref_titular en una alerta de ONI")
	}
	if !strings.Contains(err.Error(), "alerta_ref_titular_solo_de_titulares") {
		t.Fatalf("fallo por otra restriccion: %v", err)
	}
}

// La clave natural es (periodo, tipo, ref_tipo, ref_id, ref_titular). Dos
// coautores ausentes de la MISMA obra tienen que caber como dos alertas: sin
// ref_titular en la clave colapsarian en una y uno de los dos nombres se
// perderia.
func TestLaClaveNaturalDejaDosTitularesDeLaMismaObra(t *testing.T) {
	_, pool := sembrarPeriodoConAnomalias(t)

	insertar := func(refTitular string) error {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO alertas (periodo, tipo, ref_tipo, ref_id, ref_titular, detalle, detectada)
			 VALUES ($1, 'titular_sin_porcentaje', 'obra', $2, $3, 'x', now())`,
			periodoAlertas, obraCompleta, refTitular)
		return err
	}
	if err := insertar("IPI-1"); err != nil {
		t.Fatalf("primer titular: %v", err)
	}
	if err := insertar("IPI-2"); err != nil {
		t.Fatalf("segundo titular de la misma obra: %v", err)
	}
	// Y el mismo dos veces sigue siendo uno.
	if err := insertar("IPI-1"); err == nil {
		t.Fatal("el mismo titular entro dos veces en la misma obra y periodo")
	}
}

// El par (ref_tipo, ref_id) NO es una clave foranea, y es deliberado: la
// referencia apunta a tablas distintas segun el detector, y ademas la
// resolucion de #39 puede llegar a borrar el uso ofensor -- una FK con CASCADE
// se llevaria por delante la alerta, y con ella el rastro de que aquello paso.
func TestLaAlertaSobreviveAlBorradoDelRegistroOfensor(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	if _, err := pool.Exec(t.Context(), `DELETE FROM usos WHERE id = 'u-oni'`); err != nil {
		t.Fatalf("borrar el uso ofensor: %v", err)
	}
	quedan, err := s.ListarAlertas(t.Context(), aplicacion.FiltroAlertas{
		Periodo: periodoAlertas, Tipo: anomalias.TipoONI,
	})
	if err != nil {
		t.Fatalf("ListarAlertas: %v", err)
	}
	if len(quedan) != 1 {
		t.Fatalf("la alerta desaparecio al borrar el uso: quedan %d", len(quedan))
	}
}

// Los coautores de N obras salen en UNA consulta, y una obra sin coautores no
// aparece en el mapa: la ausencia es el dato, igual que en VigentesDeObras.
func TestCoautoresDeObras(t *testing.T) {
	s, pool := sembrar(t)

	porObra, err := s.CoautoresDeObras(t.Context(), []string{obraCompleta, obraIncompleta})
	if err != nil {
		t.Fatalf("CoautoresDeObras: %v", err)
	}
	if len(porObra[obraCompleta]) != 2 {
		t.Fatalf("obraCompleta trae %d coautores", len(porObra[obraCompleta]))
	}
	// Ordenados por (ipi, rol): reproducibilidad (ADR 0005).
	if porObra[obraCompleta][0].IPI != "IPI-00000001" {
		t.Fatalf("coautores sin ordenar: %+v", porObra[obraCompleta])
	}

	if _, err := pool.Exec(t.Context(),
		`DELETE FROM obra_coautores WHERE obra_id = $1`, obraIncompleta); err != nil {
		t.Fatalf("vaciar coautores: %v", err)
	}
	porObra, err = s.CoautoresDeObras(t.Context(), []string{obraIncompleta})
	if err != nil {
		t.Fatalf("CoautoresDeObras tras vaciar: %v", err)
	}
	if _, hay := porObra[obraIncompleta]; hay {
		t.Fatal("una obra sin coautores aparecio en el mapa")
	}

	// Un slice vacio no consulta y devuelve un mapa vacio, no nil.
	vacio, err := s.CoautoresDeObras(t.Context(), nil)
	if err != nil {
		t.Fatalf("CoautoresDeObras(nil): %v", err)
	}
	if vacio == nil {
		t.Fatal("CoautoresDeObras(nil) devolvio nil en vez de un mapa vacio")
	}
}

// Alertas rancias (revision de #158, punto 7): arreglado el dato, la reevaluacion autocierra la
// alerta con actor de sistema y asiento; si la anomalia vuelve, la alerta se reabre.
func TestReevaluarAutocierraYReabreContraLaBase(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()
	svc := servicioDeAnomalias(s, instanteAlertas)
	primera, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("Evaluar: %v", err)
	}

	// Se corrige el dato: la fila duplicada sale del periodo.
	if _, err := pool.Exec(ctx, `UPDATE usos SET fecha = '2025-01-09' WHERE id = 'u-dup2'`); err != nil {
		t.Fatalf("corregir el uso: %v", err)
	}
	segunda, err := svc.Evaluar(ctx, periodoAlertas, "")
	if err != nil {
		t.Fatalf("reevaluar: %v", err)
	}
	if segunda.Autocerradas != 1 || segunda.CriticasAbiertas != primera.CriticasAbiertas-1 {
		t.Fatalf("autocerradas = %d, criticas %d -> %d", segunda.Autocerradas, primera.CriticasAbiertas, segunda.CriticasAbiertas)
	}
	var (
		autocerrada bool
		porNulo     bool
		nota        string
	)
	if err := pool.QueryRow(ctx,
		`SELECT autocerrada, resuelta_por IS NULL, nota FROM alertas
		  WHERE tipo = 'duplicado_registro' AND ref_id = 'u-dup2'`).Scan(&autocerrada, &porNulo, &nota); err != nil {
		t.Fatalf("leer la alerta: %v", err)
	}
	if !autocerrada || !porNulo || nota == "" {
		t.Fatalf("autocerrada=%v resuelta_por_nulo=%v nota=%q", autocerrada, porNulo, nota)
	}
	var asientos int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM asientos WHERE hecho = $1 AND actor_id IS NULL`, aplicacion.HechoAlertaAutocerrada).Scan(&asientos); err != nil {
		t.Fatalf("contar asientos: %v", err)
	}
	if asientos != 1 {
		t.Fatalf("asientos de autocierre = %d, se esperaba 1 con actor de sistema", asientos)
	}

	// El dato vuelve a estar mal: la alerta se reabre y vuelve a bloquear.
	if _, err := pool.Exec(ctx, `UPDATE usos SET fecha = '2025-01-02' WHERE id = 'u-dup2'`); err != nil {
		t.Fatalf("revertir el uso: %v", err)
	}
	tercera, err := svc.Evaluar(ctx, periodoAlertas, "")
	if err != nil {
		t.Fatalf("tercera pasada: %v", err)
	}
	if tercera.CriticasAbiertas != primera.CriticasAbiertas || tercera.Nuevas != 1 {
		t.Fatalf("criticas = %d nuevas = %d, se esperaban %d y 1", tercera.CriticasAbiertas, tercera.Nuevas, primera.CriticasAbiertas)
	}
}

// Una alerta no puede estar autocerrada Y firmada por una persona.
func TestElEsquemaNoMezclaAutocierreYFirma(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	if _, err := servicioDeAnomalias(s, instanteAlertas).Evaluar(t.Context(), periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	_, err := pool.Exec(t.Context(),
		`UPDATE alertas SET resuelta = TRUE, autocerrada = TRUE, resuelta_por = $1, resuelta_en = now(), nota = 'x'`, usuarioAdmin)
	if err == nil || !strings.Contains(err.Error(), "alerta_resuelta_tiene_firma") {
		t.Fatalf("se esperaba alerta_resuelta_tiene_firma, fue %v", err)
	}
}
