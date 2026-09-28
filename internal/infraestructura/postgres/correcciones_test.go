package postgres

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rosvend/intela/internal/aplicacion"
	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
	"github.com/rosvend/intela/internal/dominio/repertorio"
	"github.com/rosvend/intela/internal/infraestructura/reloj"
)

// Pruebas del cierre con correccion de una critica contra el esquema real
// (#164, migracion 00024). Los CHECK de `usos` y `alertas` y el filtro
// `obra_id IS NOT NULL` del reparto son lo que se prueba: un doble no los tiene.

// criticaAbierta devuelve la unica alerta abierta de ese tipo en el periodo.
func criticaAbierta(t *testing.T, svc aplicacion.Anomalias, periodo, tipo string) aplicacion.Alerta {
	t.Helper()
	sinResolver := false
	alertas, err := svc.Listar(t.Context(), aplicacion.FiltroAlertas{Periodo: periodo, Tipo: tipo, Resueltas: &sinResolver})
	if err != nil {
		t.Fatalf("listar %s: %v", tipo, err)
	}
	if len(alertas) != 1 {
		t.Fatalf("hay %d alertas %s abiertas en %s, se esperaba una", len(alertas), tipo, periodo)
	}
	return alertas[0]
}

func lineaDeObra(t *testing.T, r reparto.Resultado, obraID string) reparto.LineaObra {
	t.Helper()
	for _, o := range r.Obras {
		if o.ObraID == obraID {
			return o
		}
	}
	t.Fatalf("la corrida no pondera la obra %q: %+v", obraID, r.Obras)
	return reparto.LineaObra{}
}

// TestUnDuplicadoResueltoConExclusionPonderaLaFilaUnaVez es el criterio de
// aceptacion de #164: la misma emision de obra-z llega en dos entregas de
// Caracol; cerrar el duplicado excluyendo una copia hace que la corrida la
// pondere UNA vez.
//
// obra-y tiene una sola emision con las mismas cifras (10 emisiones, 48 min,
// rating 9, serie), asi que la comprobacion no depende de la formula: si la
// copia excluida siguiera ponderando, obra-z sacaria el doble de puntos que
// obra-y y dos tercios de la bolsa en vez de la mitad.
func TestUnDuplicadoResueltoConExclusionPonderaLaFilaUnaVez(t *testing.T) {
	s, pool := sembrarProcesoNacionalListoParaValorizar(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO reportes (id, fuente, periodo, sha256, clave_objeto, nbytes)
		 VALUES ('rep-caracol-enero-bis', 'caracol', '2026-01', repeat('c', 64), 'reportes/c.csv', 64)`); err != nil {
		t.Fatalf("sembrar la segunda entrega: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO obras (id, titulo, genero, anio, tipo) VALUES ('obra-z', 'Obra Z', 'Drama', 2021, 'serie')`); err != nil {
		t.Fatalf("sembrar obra-z: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO titulares (id, nombre, ipi, clase) VALUES ('titular-z', 'Titular Z', 'IPI-Z', 'socio')`); err != nil {
		t.Fatalf("sembrar titular-z: %v", err)
	}
	decl, err := repertorio.NuevaDeclaracion("obra-z", []repertorio.Parte{
		{TitularID: "titular-z", IPI: "IPI-Z", Porcentaje: dec("100")},
	})
	if err != nil {
		t.Fatalf("construir declaracion: %v", err)
	}
	if _, _, err := s.Guardar(ctx, decl, time.Now(), "actor-declaracion"); err != nil {
		t.Fatalf("guardar declaracion: %v", err)
	}

	// La misma emision (ID_Ficha, fecha y hora) en las dos entregas.
	copia := func(id, reporteID string) aplicacion.UsoPersistido {
		u := usoPendiente(id, reporteID, "Obra Z")
		u.TipoObra, u.Emisiones, u.DuracionMin, u.Rating = "serie", 10, dec("48"), dec("9")
		u.Escalon, u.ONI, u.ObraID, u.Evidencia = "alias", false, "obra-z", "alias caracol/id="+id
		u.IDsFuente, u.Fecha, u.Hora = "id_ficha=9", "2026-01-05", "20:00:00"
		return u
	}
	if err := s.GuardarUsos(ctx, []aplicacion.UsoPersistido{
		copia("uso-z1", reporteEnero), copia("uso-z2", "rep-caracol-enero-bis"),
	}); err != nil {
		t.Fatalf("sembrar las dos copias: %v", err)
	}

	svc := servicioDeAnomalias(s, time.Now())
	uc := aplicacion.Procesos{
		Repo: s, Parametros: s, Bolsas: s, Declaraciones: s, Usos: s, Resultados: s, Unidad: s,
		Anomalias: svc, Bitacora: s, Reloj: reloj.Sistema{}, Origen: s,
	}
	if _, err := uc.IniciarProceso(ctx, "proc-y", "2026-01", reparto.Nacional, "bolsa-1", "actor-dist"); err != nil {
		t.Fatalf("iniciar proceso: %v", err)
	}
	if _, err := uc.AvanzarEtapa(ctx, "proc-y", "actor-dist"); err != nil {
		t.Fatalf("avanzar a deducciones: %v", err)
	}
	if _, err := uc.AvanzarEtapa(ctx, "proc-y", "actor-dist"); !errors.Is(err, aplicacion.ErrAnomaliasCriticasAbiertas) {
		t.Fatalf("con el duplicado abierto: err = %v, se esperaba ErrAnomaliasCriticasAbiertas", err)
	}

	dup := criticaAbierta(t, svc, "2026-01", anomalias.TipoDuplicadoRegistro)

	// Cerrarla solo con la nota -- lo que hacia la bandeja antes de #164 -- ya no se puede.
	if _, err := svc.Resolver(ctx, dup.ID, "actor-dist", aplicacion.SolicitudCierreAlerta{
		Nota: "revisado", ActorRol: string(aplicacion.RolDistribucion),
	}); !errors.Is(err, anomalias.ErrAccionInvalida) {
		t.Fatalf("cerrar sin accion dio %v, se esperaba ErrAccionInvalida", err)
	}

	resuelta, err := svc.Resolver(ctx, dup.ID, "actor-dist", aplicacion.SolicitudCierreAlerta{
		Nota:       "la entrega bis es reenvio de la misma parrilla",
		ActorRol:   string(aplicacion.RolDistribucion),
		Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso},
	})
	if err != nil {
		t.Fatalf("resolver con exclusion: %v", err)
	}
	if resuelta.Accion != anomalias.AccionExcluirUso || resuelta.AccionObjetivo != dup.RefID ||
		resuelta.ResueltaRol != string(aplicacion.RolDistribucion) {
		t.Fatalf("alerta cerrada = %+v", resuelta)
	}

	// La fila quedo fuera, firmada y con la obra que tenia en la evidencia.
	var escalon, evidencia, resueltoPor, nota string
	var oni, conObra bool
	if err := pool.QueryRow(ctx,
		`SELECT escalon, oni, obra_id IS NOT NULL, evidencia, COALESCE(resuelto_por, ''), nota_resolucion
		   FROM usos WHERE id = $1`, dup.RefID).Scan(&escalon, &oni, &conObra, &evidencia, &resueltoPor, &nota); err != nil {
		t.Fatalf("leer la copia excluida: %v", err)
	}
	if escalon != identificacion.EscalonDuplicado || oni || conObra || resueltoPor != "actor-dist" || nota == "" ||
		!strings.Contains(evidencia, "obra-z") || !strings.Contains(evidencia, dup.ID) {
		t.Fatalf("copia excluida: escalon=%q oni=%v con_obra=%v por=%q nota=%q evidencia=%q",
			escalon, oni, conObra, resueltoPor, nota, evidencia)
	}
	var asientos int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM asientos WHERE hecho = $1 AND ref_tipo = 'uso' AND ref_id = $2 AND actor_id = 'actor-dist'`,
		aplicacion.HechoUsoExcluidoPorDuplicado, dup.RefID).Scan(&asientos); err != nil {
		t.Fatalf("contar asientos de la correccion: %v", err)
	}
	if asientos != 1 {
		t.Fatalf("asientos %s sobre %s = %d, se esperaba uno", aplicacion.HechoUsoExcluidoPorDuplicado, dup.RefID, asientos)
	}

	// El motor recibe una sola copia, y la excluida se cuenta como duplicada, no como pendiente u ONI.
	usos, resumen, err := s.UsosDeCanal(ctx, "2026-01", "caracol", 2026)
	if err != nil {
		t.Fatalf("UsosDeCanal: %v", err)
	}
	copias := 0
	for _, u := range usos {
		if u.Uso.ObraID == "obra-z" {
			copias++
		}
	}
	if copias != 1 || resumen != (aplicacion.ResumenUsosDeCanal{Duplicados: 1}) {
		t.Fatalf("copias de obra-z en el motor = %d, resumen = %+v; se esperaba una copia y un duplicado", copias, resumen)
	}

	p, err := uc.AvanzarEtapa(ctx, "proc-y", "actor-dist")
	if err != nil {
		t.Fatalf("con el duplicado corregido deberia avanzar: %v", err)
	}
	if p.Etapa != reparto.EtapaImporteObra {
		t.Fatalf("etapa = %q, se esperaba importe_obra", p.Etapa)
	}

	resultado, err := s.ResultadoPorProceso(ctx, "proc-y")
	if err != nil {
		t.Fatalf("leer el resultado: %v", err)
	}
	y, z := lineaDeObra(t, resultado, "obra-y"), lineaDeObra(t, resultado, "obra-z")
	if y.Puntos.IsZero() || !z.Puntos.Equal(y.Puntos) {
		t.Fatalf("puntos obra-z = %s, obra-y = %s: la emision duplicada se pondero mas de una vez", z.Puntos, y.Puntos)
	}
	if !z.Importe.Equal(y.Importe) {
		t.Fatalf("importe obra-z = %s, obra-y = %s", z.Importe, y.Importe)
	}

	// La transicion dice con cuantas criticas aceptadas sin corregir se paso: ninguna.
	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM asientos WHERE hecho = $1 AND ref_id = 'proc-y' ORDER BY cuando DESC, id DESC LIMIT 1`,
		aplicacion.HechoProcesoEtapaAvanzada).Scan(&payload); err != nil {
		t.Fatalf("leer el asiento de la transicion: %v", err)
	}
	var transicion struct {
		Aceptadas *int `json:"criticas_aceptadas_tal_cual"`
	}
	if err := json.Unmarshal(payload, &transicion); err != nil {
		t.Fatalf("payload de la transicion: %v", err)
	}
	if transicion.Aceptadas == nil || *transicion.Aceptadas != 0 {
		t.Fatalf("criticas_aceptadas_tal_cual = %v, se esperaba un cero explicito", transicion.Aceptadas)
	}
}

// Excluir la entrega entera de un duplicado de huella marca la entrega, saca
// sus filas del reparto (salvo las descartadas, que ya estaban fuera por otra
// decision) y apaga en la siguiente pasada las alertas que esas filas tenian.
func TestExcluirUnaEntregaDuplicadaMarcaLaEntregaYSacaSusFilas(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	dup := criticaAbierta(t, svc, periodoAlertas, anomalias.TipoDuplicadoArchivo)
	if dup.RefID != repA {
		t.Fatalf("la alerta cae sobre %q, se esperaba %q", dup.RefID, repA)
	}

	// repC es la otra pata, pero vive en 2024-12: no se excluye desde una alerta de 2025-01.
	if _, err := svc.Resolver(ctx, dup.ID, usuarioAdmin, aplicacion.SolicitudCierreAlerta{
		Nota: "nota", ActorRol: string(aplicacion.RolAdministrador),
		Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirEntrega, ReporteID: repC},
	}); !errors.Is(err, anomalias.ErrAccionInvalida) {
		t.Fatalf("excluir la entrega de otro periodo dio %v, se esperaba ErrAccionInvalida", err)
	}

	enJuego := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM usos WHERE reporte_id = $1 AND escalon NOT IN ('descartado', 'duplicado')`,
			repA).Scan(&n); err != nil {
			t.Fatalf("contar filas en juego: %v", err)
		}
		return n
	}
	antes := enJuego()
	if antes == 0 {
		t.Fatal("el fixture no deja filas en juego en repA")
	}

	if _, err := svc.Resolver(ctx, dup.ID, usuarioAdmin, aplicacion.SolicitudCierreAlerta{
		Nota: "netflix reenvio el archivo de caracol", ActorRol: string(aplicacion.RolAdministrador),
		Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirEntrega},
	}); err != nil {
		t.Fatalf("excluir la entrega: %v", err)
	}

	var excluidaPor string
	var excluidaEn *time.Time
	if err := pool.QueryRow(ctx, `SELECT COALESCE(excluida_por, ''), excluida_en FROM reportes WHERE id = $1`, repA).
		Scan(&excluidaPor, &excluidaEn); err != nil {
		t.Fatalf("leer la entrega: %v", err)
	}
	if excluidaPor != usuarioAdmin || excluidaEn == nil || !excluidaEn.Equal(instanteAlertas) {
		t.Fatalf("entrega excluida por %q en %v", excluidaPor, excluidaEn)
	}
	if n := enJuego(); n != 0 {
		t.Fatalf("quedan %d filas de repA en juego", n)
	}
	var sinPrevio int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usos WHERE reporte_id = $1 AND escalon = 'duplicado'
		    AND (evidencia NOT LIKE '%antes escalon %' OR resuelto_por IS NULL OR oni OR obra_id IS NOT NULL)`,
		repA).Scan(&sinPrevio); err != nil {
		t.Fatalf("revisar las filas excluidas: %v", err)
	}
	if sinPrevio != 0 {
		t.Fatalf("%d filas excluidas sin firma, sin estado previo en la evidencia o con obra", sinPrevio)
	}

	var payload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM asientos WHERE hecho = $1 AND ref_tipo = 'reporte' AND ref_id = $2`,
		aplicacion.HechoEntregaExcluidaPorDuplicado, repA).Scan(&payload); err != nil {
		t.Fatalf("leer el asiento de la exclusion: %v", err)
	}
	var hecho struct {
		Usos int `json:"usos_excluidos"`
	}
	if err := json.Unmarshal(payload, &hecho); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if hecho.Usos != antes {
		t.Fatalf("usos_excluidos = %d, se esperaban %d", hecho.Usos, antes)
	}

	// Una segunda exclusion de la misma entrega no pasa: la marca es condicional.
	if _, err := s.ExcluirEntregaDuplicada(ctx, aplicacion.ExclusionDeEntrega{
		ReporteID: repA, ActorID: usuarioAdmin, Nota: "otra vez", Evidencia: "duplicado", Cuando: instanteAlertas,
	}); !errors.Is(err, anomalias.ErrAccionNoAplica) {
		t.Fatalf("la segunda exclusion dio %v, se esperaba ErrAccionNoAplica", err)
	}

	resumen, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("reevaluar: %v", err)
	}
	// Las filas de repA traian la ONI, el tipo sin mapear y la primera copia
	// del duplicado de registro: fuera del reparto, ninguna de esas se ve.
	for _, tipo := range []string{anomalias.TipoDuplicadoArchivo, anomalias.TipoONI, anomalias.TipoTipoObraSinMapear, anomalias.TipoDuplicadoRegistro} {
		if n := resumen.PorTipo[tipo]; n != 0 {
			t.Errorf("la reevaluacion sigue viendo %d %s", n, tipo)
		}
	}
}

// Asignar el tipo corrige la fila y la siguiente pasada deja de verla.
func TestAsignarElTipoDeObraCorrigeLaFila(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	sinTipo := criticaAbierta(t, svc, periodoAlertas, anomalias.TipoTipoObraSinMapear)

	if _, err := svc.Resolver(ctx, sinTipo.ID, usuarioAdmin, aplicacion.SolicitudCierreAlerta{
		Nota: "serie segun la ficha", ActorRol: string(aplicacion.RolAdministrador),
		Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAsignarTipoObra, TipoObra: "serie"},
	}); err != nil {
		t.Fatalf("asignar el tipo: %v", err)
	}
	var tipo string
	if err := pool.QueryRow(ctx, `SELECT tipo_obra FROM usos WHERE id = $1`, sinTipo.RefID).Scan(&tipo); err != nil {
		t.Fatalf("leer la fila: %v", err)
	}
	if tipo != "serie" {
		t.Fatalf("tipo_obra = %q", tipo)
	}

	// Una fila que ya tiene tipo no se pisa: la alerta describia otro dato.
	if _, err := s.AsignarTipoObraAUso(ctx, sinTipo.RefID, "telenovela"); !errors.Is(err, anomalias.ErrAccionNoAplica) {
		t.Fatalf("reasignar dio %v, se esperaba ErrAccionNoAplica", err)
	}

	resumen, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin)
	if err != nil {
		t.Fatalf("reevaluar: %v", err)
	}
	if n := resumen.PorTipo[anomalias.TipoTipoObraSinMapear]; n != 0 {
		t.Fatalf("la reevaluacion sigue viendo %d filas sin tipo", n)
	}
}

// Aceptar tal cual no toca el dato y la compuerta la cuenta aparte.
func TestAceptarTalCualLaCompuertaLaCuentaAparte(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	for _, tipo := range []string{anomalias.TipoDuplicadoRegistro, anomalias.TipoDuplicadoArchivo} {
		a := criticaAbierta(t, svc, periodoAlertas, tipo)
		if _, err := svc.Resolver(ctx, a.ID, usuarioAdmin, aplicacion.SolicitudCierreAlerta{
			Nota: "falso positivo (P-21)", ActorRol: string(aplicacion.RolAdministrador),
			Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAceptarTalCual},
		}); err != nil {
			t.Fatalf("aceptar %s: %v", tipo, err)
		}
	}
	a := criticaAbierta(t, svc, periodoAlertas, anomalias.TipoTipoObraSinMapear)
	if _, err := svc.Resolver(ctx, a.ID, usuarioAdmin, aplicacion.SolicitudCierreAlerta{
		Nota: "serie", ActorRol: string(aplicacion.RolAdministrador),
		Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAsignarTipoObra, TipoObra: "serie"},
	}); err != nil {
		t.Fatalf("asignar: %v", err)
	}

	var duplicados int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM usos WHERE escalon = 'duplicado'`).Scan(&duplicados); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if duplicados != 0 {
		t.Fatalf("aceptar tal cual saco %d filas del reparto", duplicados)
	}

	estado, err := svc.Bloqueantes(ctx, periodoAlertas)
	if err != nil {
		t.Fatalf("Bloqueantes: %v", err)
	}
	if estado.Abiertas != 0 || estado.AceptadasTalCual != 2 {
		t.Fatalf("compuerta = %+v, se esperaban 0 abiertas y 2 aceptadas", estado)
	}
}

// La escritura de la exclusion es condicional al estado validado: si la fila
// cambio entretanto, no se toca y sale ErrAccionNoAplica.
func TestExcluirUnUsoQueCambioNoLoToca(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()

	err := s.ExcluirUsoDuplicado(ctx, aplicacion.ExclusionDeUso{
		UsoID: "u-dup2", EscalonPrevio: identificacion.EscalonDifuso, ObraPrevia: obraCompleta,
		ActorID: usuarioAdmin, Nota: "nota", Evidencia: "duplicado", Cuando: instanteAlertas,
	})
	if !errors.Is(err, anomalias.ErrAccionNoAplica) {
		t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
	}
	var escalon string
	if err := pool.QueryRow(ctx, `SELECT escalon FROM usos WHERE id = 'u-dup2'`).Scan(&escalon); err != nil {
		t.Fatalf("leer: %v", err)
	}
	if escalon != identificacion.EscalonAlias {
		t.Fatalf("la fila quedo en %q", escalon)
	}
}

// Los CHECK de 00024 son la ultima defensa si manana se escribe en `alertas`
// o en `usos` por otro camino que el caso de uso.
func TestLosCheckDeLaCorreccionSostienenElCierre(t *testing.T) {
	s, pool := sembrarPeriodoConAnomalias(t)
	ctx := t.Context()
	svc := servicioDeAnomalias(s, instanteAlertas)
	if _, err := svc.Evaluar(ctx, periodoAlertas, usuarioAdmin); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	abierta := criticaAbierta(t, svc, periodoAlertas, anomalias.TipoDuplicadoRegistro)

	rechaza := func(t *testing.T, restriccion, sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		if err == nil {
			t.Fatalf("la base acepto lo que %s prohibe", restriccion)
		}
		if !strings.Contains(err.Error(), restriccion) {
			t.Fatalf("el rechazo no viene de %s: %v", restriccion, err)
		}
	}

	t.Run("una abierta no lleva accion", func(t *testing.T) {
		rechaza(t, "alerta_accion_solo_de_persona",
			`UPDATE alertas SET accion = 'aceptar_tal_cual' WHERE id = $1::uuid`, abierta.ID)
	})
	t.Run("accion fuera del vocabulario", func(t *testing.T) {
		rechaza(t, "alerta_accion_valida",
			`UPDATE alertas SET resuelta = TRUE, resuelta_por = $2, resuelta_en = now(), nota = 'n',
			        accion = 'borrar', accion_objetivo = 'u-dup2'
			  WHERE id = $1::uuid`, abierta.ID, usuarioAdmin)
	})
	t.Run("una correccion nombra su objetivo", func(t *testing.T) {
		rechaza(t, "alerta_accion_tiene_objetivo",
			`UPDATE alertas SET resuelta = TRUE, resuelta_por = $2, resuelta_en = now(), nota = 'n',
			        accion = 'excluir_uso', accion_objetivo = ''
			  WHERE id = $1::uuid`, abierta.ID, usuarioAdmin)
	})
	t.Run("aceptar no nombra objetivo", func(t *testing.T) {
		rechaza(t, "alerta_accion_tiene_objetivo",
			`UPDATE alertas SET resuelta = TRUE, resuelta_por = $2, resuelta_en = now(), nota = 'n',
			        accion = 'aceptar_tal_cual', accion_objetivo = 'u-dup2'
			  WHERE id = $1::uuid`, abierta.ID, usuarioAdmin)
	})
	t.Run("un duplicado va firmado", func(t *testing.T) {
		rechaza(t, "manual_tiene_autor",
			`UPDATE usos SET escalon = 'duplicado', oni = FALSE, obra_id = NULL, nota_resolucion = 'n' WHERE id = 'u-dup2'`)
	})
	t.Run("un duplicado no tiene obra", func(t *testing.T) {
		rechaza(t, "uso_resuelto_tiene_obra",
			`UPDATE usos SET escalon = 'duplicado', oni = FALSE, resuelto_por = $1, resuelto_en = now(),
			        nota_resolucion = 'n' WHERE id = 'u-dup2'`, usuarioAdmin)
	})
	t.Run("una entrega excluida lleva firma e instante", func(t *testing.T) {
		rechaza(t, "reporte_exclusion_firmada",
			`UPDATE reportes SET excluida_por = $2 WHERE id = $1`, repA, usuarioAdmin)
	})
}
