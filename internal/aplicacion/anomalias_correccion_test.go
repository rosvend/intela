package aplicacion

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Pruebas del cierre con correccion de una critica (#164). El periodo es el de
// [servicioSembrado]: u-dup2 repite el registro de u-dup1, r1 comparte bytes con
// r3 (otro periodo, otra fuente) y u-sintipo es una fila identificada sin tipo.

// evaluado devuelve el servicio sembrado con el periodo ya evaluado.
func evaluado(t *testing.T) (Anomalias, *entregasFalsas, *alertasFalsas, *bitacoraFalsa, *unidadFalsa) {
	t.Helper()
	svc, entregas, alertas, bitacora, unidad := servicioSembrado()
	if _, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1"); err != nil {
		t.Fatalf("Evaluar: %v", err)
	}
	return svc, entregas, alertas, bitacora, unidad
}

func correccionesDe(t *testing.T, svc Anomalias) *correccionesFalsas {
	t.Helper()
	c, ok := svc.Correcciones.(*correccionesFalsas)
	if !ok {
		t.Fatalf("Correcciones no es el doble: %T", svc.Correcciones)
	}
	return c
}

// asientosDesde devuelve los asientos escritos a partir del indice n.
func asientosDesde(b *bitacoraFalsa, n int) []Asiento {
	return append([]Asiento(nil), b.asientos[n:]...)
}

func payloadDeAsiento(t *testing.T, a Asiento) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(a.Payload, &p); err != nil {
		t.Fatalf("payload de %s: %v", a.Hecho, err)
	}
	return p
}

func usoFalso(t *testing.T, e *entregasFalsas, id string) UsoPersistido {
	t.Helper()
	for _, u := range e.usos {
		if u.ID == id {
			return u
		}
	}
	t.Fatalf("no hay uso %q", id)
	return UsoPersistido{}
}

func alertaGuardada(t *testing.T, alertas *alertasFalsas, id string) Alerta {
	t.Helper()
	for _, a := range alertas.filas {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("no hay alerta %q", id)
	return Alerta{}
}

// ---------------------------------------------------------------------------
// Una critica no se cierra sin tocar el dato

func TestResolverUnaCriticaSinAccionNoLaCierra(t *testing.T) {
	for _, tipo := range []string{anomalias.TipoDuplicadoRegistro, anomalias.TipoDuplicadoArchivo, anomalias.TipoTipoObraSinMapear} {
		t.Run(tipo, func(t *testing.T) {
			svc, _, alertas, bitacora, _ := evaluado(t)
			id := alertaDeTipo(t, alertas, tipo).ID
			antes := len(bitacora.asientos)

			_, err := svc.Resolver(t.Context(), id, "usr-2", soloNota("ya lo mire"))
			if !errors.Is(err, anomalias.ErrAccionInvalida) {
				t.Fatalf("cerrar %s sin accion dio %v, se esperaba ErrAccionInvalida", tipo, err)
			}
			if alertaGuardada(t, alertas, id).Resuelta {
				t.Fatal("la critica quedo cerrada sin corregir el dato")
			}
			if len(bitacora.asientos) != antes {
				t.Fatalf("se asentaron %d hechos por un cierre rechazado", len(bitacora.asientos)-antes)
			}
			if len(alertas.bloqueos) != 1 {
				t.Fatalf("cerrojos = %v: un cierre rechazado no toma el del periodo", alertas.bloqueos)
			}
		})
	}
}

func TestResolverUnaNoCriticaConAccionSeRechaza(t *testing.T) {
	svc, _, alertas, _, _ := evaluado(t)
	id := alertaDeTipo(t, alertas, anomalias.TipoONI).ID

	_, err := svc.Resolver(t.Context(), id, "usr-2", conAccion("fuera",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso}))
	if !errors.Is(err, anomalias.ErrAccionInvalida) {
		t.Fatalf("una ONI con accion dio %v, se esperaba ErrAccionInvalida", err)
	}
	if alertaGuardada(t, alertas, id).Resuelta {
		t.Fatal("la ONI quedo cerrada")
	}
}

// La forma del pedido se valida antes de abrir la unidad: un cuerpo que se va
// a rechazar igual no toma el cerrojo del periodo.
func TestResolverRechazaUnPedidoMalFormadoAntesDeAbrirLaUnidad(t *testing.T) {
	svc, _, alertas, _, unidad := evaluado(t)
	id := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro).ID
	entradas := unidad.entradas

	for _, p := range []anomalias.PedidoDeCorreccion{
		{Accion: "borrar"},
		{Accion: anomalias.AccionExcluirUso, TipoObra: "serie"},
		{Accion: anomalias.AccionAsignarTipoObra, TipoObra: "documental"},
	} {
		if _, err := svc.Resolver(t.Context(), id, "usr-2", conAccion("nota", p)); !errors.Is(err, anomalias.ErrAccionInvalida) {
			t.Fatalf("%+v dio %v, se esperaba ErrAccionInvalida", p, err)
		}
	}
	if unidad.entradas != entradas {
		t.Fatalf("se abrieron %d unidades por pedidos mal formados", unidad.entradas-entradas)
	}
}

// ---------------------------------------------------------------------------
// excluir_uso

func TestResolverExcluyeLaCopiaDuplicadaYAsientaLosDosHechos(t *testing.T) {
	svc, entregas, alertas, bitacora, _ := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro)
	if alerta.RefID != "u-dup2" {
		t.Fatalf("la alerta cae sobre %q, se esperaba u-dup2", alerta.RefID)
	}
	antes := len(bitacora.asientos)

	resuelta, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("reenvio de la misma parrilla",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso}))
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	// La alerta guarda que se hizo, sobre que y con que rol.
	if resuelta.Accion != anomalias.AccionExcluirUso || resuelta.AccionObjetivo != "u-dup2" ||
		resuelta.ResueltaRol != string(RolDistribucion) || resuelta.ResueltaPor != "usr-2" {
		t.Fatalf("alerta cerrada = %+v", resuelta)
	}

	// El dato cambio: la fila objetivo salio, condicionada a lo que se valido.
	c := correccionesDe(t, svc)
	if len(c.usosExcluidos) != 1 {
		t.Fatalf("exclusiones = %+v, se esperaba una", c.usosExcluidos)
	}
	ex := c.usosExcluidos[0]
	if ex.UsoID != "u-dup2" || ex.EscalonPrevio != identificacion.EscalonAlias || ex.ObraPrevia != "o-ok" ||
		ex.ActorID != "usr-2" || ex.Nota != "reenvio de la misma parrilla" || !ex.Cuando.Equal(instanteAnomalias) {
		t.Fatalf("exclusion = %+v", ex)
	}
	if !strings.Contains(ex.Evidencia, alerta.ID) || !strings.Contains(ex.Evidencia, "o-ok") {
		t.Fatalf("la evidencia no dice por que alerta salio ni que obra tenia: %q", ex.Evidencia)
	}
	if u := usoFalso(t, entregas, "u-dup2"); u.Escalon != identificacion.EscalonDuplicado {
		t.Fatalf("u-dup2 quedo en %q", u.Escalon)
	}

	// Dos asientos: el cierre sobre la alerta y la correccion sobre la fila.
	nuevos := asientosDesde(bitacora, antes)
	if len(nuevos) != 2 {
		t.Fatalf("asientos = %+v, se esperaban dos", nuevos)
	}
	cierre, correccion := nuevos[0], nuevos[1]
	if cierre.Hecho != HechoAlertaResuelta || cierre.RefID != alerta.ID {
		t.Fatalf("primer asiento = %+v", cierre)
	}
	if p := payloadDeAsiento(t, cierre); p["accion"] != anomalias.AccionExcluirUso || p["accion_objetivo"] != "u-dup2" ||
		p["actor_rol"] != string(RolDistribucion) {
		t.Fatalf("payload del cierre = %v", p)
	}
	if correccion.Hecho != HechoUsoExcluidoPorDuplicado || correccion.RefTipo != RefUso || correccion.RefID != "u-dup2" ||
		correccion.ActorID != "usr-2" || !correccion.Cuando.Equal(instanteAnomalias) {
		t.Fatalf("asiento de la correccion = %+v", correccion)
	}
	p := payloadDeAsiento(t, correccion)
	if p["alerta_id"] != alerta.ID || p["escalon_anterior"] != identificacion.EscalonAlias || p["obra_anterior"] != "o-ok" ||
		p["uso_de_la_alerta"] != "u-dup2" || p["clave_registro"] == "" {
		t.Fatalf("payload de la correccion = %v", p)
	}

	// La correccion corre con el cerrojo del periodo: dos tomas, la de Evaluar y la del cierre.
	if !slices.Equal(alertas.bloqueos, []string{periodoDePrueba, periodoDePrueba}) {
		t.Fatalf("cerrojos = %v", alertas.bloqueos)
	}

	// La siguiente pasada ya no ve el duplicado: el dato esta corregido.
	resumen, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1")
	if err != nil {
		t.Fatalf("reevaluar: %v", err)
	}
	if n := resumen.PorTipo[anomalias.TipoDuplicadoRegistro]; n != 0 {
		t.Fatalf("la reevaluacion sigue viendo %d duplicados de registro", n)
	}
}

// La alerta cae sobre la copia que llego segunda, pero la que no manda puede
// ser la primera.
func TestResolverExcluyeLaOtraCopia(t *testing.T) {
	svc, entregas, alertas, _, _ := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro)

	resuelta, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("la segunda entrega es la corregida",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso, UsoID: "u-dup1"}))
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if resuelta.AccionObjetivo != "u-dup1" {
		t.Fatalf("objetivo = %q, se esperaba u-dup1", resuelta.AccionObjetivo)
	}
	if usoFalso(t, entregas, "u-dup1").Escalon != identificacion.EscalonDuplicado ||
		usoFalso(t, entregas, "u-dup2").Escalon != identificacion.EscalonAlias {
		t.Fatal("se excluyo la copia equivocada")
	}
}

func TestResolverNoExcluyeUnUsoQueNoRepiteElRegistro(t *testing.T) {
	svc, entregas, alertas, bitacora, _ := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro)
	antes := len(bitacora.asientos)

	_, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("nota",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso, UsoID: "u-sintipo"}))
	if !errors.Is(err, anomalias.ErrAccionInvalida) {
		t.Fatalf("dio %v, se esperaba ErrAccionInvalida", err)
	}
	if usoFalso(t, entregas, "u-sintipo").Escalon != identificacion.EscalonAlias {
		t.Fatal("se excluyo una fila que no repite el registro")
	}
	if len(bitacora.asientos) != antes {
		t.Fatal("un cierre rechazado dejo asientos")
	}
}

// Si la escritura condicional no encuentra la fila como se valido -- la
// cascada la movio entretanto --, el error sale tal cual y la unidad no se
// confirma: el cierre de la alerta se revierte con ella.
func TestUnaCorreccionQueYaNoAplicaNoConfirmaLaUnidad(t *testing.T) {
	svc, _, alertas, bitacora, unidad := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro)
	correccionesDe(t, svc).err = anomalias.ErrAccionNoAplica
	unidad.confirmo = false
	antes := len(bitacora.asientos)

	_, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("nota",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso}))
	if !errors.Is(err, anomalias.ErrAccionNoAplica) {
		t.Fatalf("dio %v, se esperaba ErrAccionNoAplica", err)
	}
	if unidad.confirmo {
		t.Fatal("la unidad se confirmo con la correccion fallida")
	}
	if len(bitacora.asientos) != antes {
		t.Fatal("se asento una correccion que no se aplico")
	}
}

// ---------------------------------------------------------------------------
// excluir_entrega

func TestResolverExcluyeLaEntregaEnteraYApagaLasAlertasDeSusFilas(t *testing.T) {
	svc, entregas, alertas, bitacora, _ := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoArchivo)
	if alerta.RefID != "r1" {
		t.Fatalf("la alerta cae sobre %q, se esperaba r1", alerta.RefID)
	}
	antes := len(bitacora.asientos)

	if _, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("netflix reenvio el archivo de caracol",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirEntrega})); err != nil {
		t.Fatalf("Resolver: %v", err)
	}

	for _, u := range entregas.usos {
		if u.ReporteID == "r1" && u.Escalon != identificacion.EscalonDuplicado {
			t.Errorf("la fila %s de r1 quedo en %q", u.ID, u.Escalon)
		}
		if u.ReporteID != "r1" && u.Escalon == identificacion.EscalonDuplicado {
			t.Errorf("la fila %s de otra entrega se excluyo", u.ID)
		}
	}

	nuevos := asientosDesde(bitacora, antes)
	if len(nuevos) != 2 || nuevos[1].Hecho != HechoEntregaExcluidaPorDuplicado ||
		nuevos[1].RefTipo != RefReporte || nuevos[1].RefID != "r1" {
		t.Fatalf("asientos = %+v", nuevos)
	}
	p := payloadDeAsiento(t, nuevos[1])
	// u-oni, u-dup1, u-sintipo y u-inc.
	if p["usos_excluidos"] != float64(4) || p["sha256"] != "aa" {
		t.Fatalf("payload = %v", p)
	}
	obras, _ := p["obras_afectadas"].([]any)
	if len(obras) != 2 || obras[0] != "o-inc" || obras[1] != "o-ok" {
		t.Fatalf("obras_afectadas = %v, se esperaban [o-inc o-ok] ordenadas", p["obras_afectadas"])
	}

	// La siguiente pasada ya no ve la huella duplicada, y las alertas de las
	// filas de r1 (ONI, tipo sin mapear) se autocierran: esas filas ya no ponderan.
	resumen, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1")
	if err != nil {
		t.Fatalf("reevaluar: %v", err)
	}
	for _, tipo := range []string{anomalias.TipoDuplicadoArchivo, anomalias.TipoONI, anomalias.TipoTipoObraSinMapear} {
		if n := resumen.PorTipo[tipo]; n != 0 {
			t.Errorf("la reevaluacion sigue viendo %d %s", n, tipo)
		}
	}
	if resumen.CriticasAbiertas != 0 {
		t.Fatalf("criticas abiertas tras excluir la entrega = %d", resumen.CriticasAbiertas)
	}
}

// La otra pata del par vive en otro periodo: se excluye desde una alerta de
// su periodo, no desde esta.
func TestResolverNoExcluyeUnaEntregaDeOtroPeriodo(t *testing.T) {
	svc, _, alertas, _, _ := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoArchivo)

	_, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("nota",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirEntrega, ReporteID: "r3"}))
	if !errors.Is(err, anomalias.ErrAccionInvalida) {
		t.Fatalf("dio %v, se esperaba ErrAccionInvalida", err)
	}
	if len(correccionesDe(t, svc).entregasExcluidas) != 0 {
		t.Fatal("se excluyo la entrega de otro periodo")
	}
}

// ---------------------------------------------------------------------------
// asignar_tipo_obra

func TestResolverAsignaElTipoDeObra(t *testing.T) {
	svc, entregas, alertas, bitacora, _ := evaluado(t)
	alerta := alertaDeTipo(t, alertas, anomalias.TipoTipoObraSinMapear)
	antes := len(bitacora.asientos)

	resuelta, err := svc.Resolver(t.Context(), alerta.ID, "usr-2", conAccion("serie segun la ficha del catalogo",
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAsignarTipoObra, TipoObra: "Serie"}))
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if resuelta.AccionObjetivo != "serie" {
		t.Fatalf("objetivo = %q, se esperaba el tipo normalizado", resuelta.AccionObjetivo)
	}
	if u := usoFalso(t, entregas, "u-sintipo"); u.TipoObra != "serie" {
		t.Fatalf("u-sintipo quedo con tipo %q", u.TipoObra)
	}
	nuevos := asientosDesde(bitacora, antes)
	if len(nuevos) != 2 || nuevos[1].Hecho != HechoTipoObraAsignado || nuevos[1].RefID != "u-sintipo" {
		t.Fatalf("asientos = %+v", nuevos)
	}
	if p := payloadDeAsiento(t, nuevos[1]); p["obra_id"] != "o-ok" || p["tipo_obra"] != "serie" {
		t.Fatalf("payload = %v", p)
	}

	resumen, err := svc.Evaluar(t.Context(), periodoDePrueba, "usr-1")
	if err != nil {
		t.Fatalf("reevaluar: %v", err)
	}
	if n := resumen.PorTipo[anomalias.TipoTipoObraSinMapear]; n != 0 {
		t.Fatalf("la reevaluacion sigue viendo %d filas sin tipo", n)
	}
}

// ---------------------------------------------------------------------------
// aceptar_tal_cual

func TestAceptarTalCualNoTocaElDatoYLaCompuertaLaCuentaAparte(t *testing.T) {
	svc, entregas, alertas, bitacora, _ := evaluado(t)
	antes := len(bitacora.asientos)

	for _, tipo := range []string{anomalias.TipoDuplicadoRegistro, anomalias.TipoDuplicadoArchivo} {
		a := alertaDeTipo(t, alertas, tipo)
		resuelta, err := svc.Resolver(t.Context(), a.ID, "usr-2", conAccion("falso positivo de la clave (P-21)",
			anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAceptarTalCual}))
		if err != nil {
			t.Fatalf("aceptar %s: %v", tipo, err)
		}
		if resuelta.Accion != anomalias.AccionAceptarTalCual || resuelta.AccionObjetivo != "" ||
			resuelta.ResueltaRol != string(RolDistribucion) {
			t.Fatalf("alerta aceptada = %+v", resuelta)
		}
	}
	if _, err := svc.Resolver(t.Context(), alertaDeTipo(t, alertas, anomalias.TipoTipoObraSinMapear).ID, "usr-2",
		conAccion("serie", anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAsignarTipoObra, TipoObra: "serie"})); err != nil {
		t.Fatalf("asignar tipo: %v", err)
	}

	c := correccionesDe(t, svc)
	if len(c.usosExcluidos) != 0 || len(c.entregasExcluidas) != 0 {
		t.Fatalf("aceptar tal cual toco el dato: %+v / %+v", c.usosExcluidos, c.entregasExcluidas)
	}
	if usoFalso(t, entregas, "u-dup2").Escalon != identificacion.EscalonAlias {
		t.Fatal("la copia aceptada dejo de ponderar")
	}
	// Un asiento por aceptacion (no hay correccion que asentar) y dos por la asignacion.
	if n := len(bitacora.asientos) - antes; n != 4 {
		t.Fatalf("asientos nuevos = %d, se esperaban 4", n)
	}

	estado, err := svc.Bloqueantes(t.Context(), periodoDePrueba)
	if err != nil {
		t.Fatalf("Bloqueantes: %v", err)
	}
	if estado.Abiertas != 0 || estado.AceptadasTalCual != 2 {
		t.Fatalf("compuerta = %+v, se esperaban 0 abiertas y 2 aceptadas tal cual", estado)
	}
}

// Aceptar sin corregir es la decision que mas pesa en la auditoria: sin el rol
// de quien firma no se acepta.
func TestAceptarTalCualExigeElRolDeQuienFirma(t *testing.T) {
	svc, _, alertas, _, _ := evaluado(t)
	a := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro)

	_, err := svc.Resolver(t.Context(), a.ID, "usr-2", SolicitudCierreAlerta{
		Nota:       "falso positivo",
		Correccion: anomalias.PedidoDeCorreccion{Accion: anomalias.AccionAceptarTalCual},
	})
	if !errors.Is(err, ErrActorAusente) {
		t.Fatalf("dio %v, se esperaba ErrActorAusente", err)
	}
	if alertaGuardada(t, alertas, a.ID).Resuelta {
		t.Fatal("la critica quedo aceptada sin rol")
	}
}

// ---------------------------------------------------------------------------
// Nota

func TestLaNotaDeUnaCorreccionNoPasaDelTope(t *testing.T) {
	svc, _, alertas, _, _ := evaluado(t)
	a := alertaDeTipo(t, alertas, anomalias.TipoDuplicadoRegistro)

	_, err := svc.Resolver(t.Context(), a.ID, "usr-2", conAccion(strings.Repeat("x", anomalias.MaxNotaCorreccion+1),
		anomalias.PedidoDeCorreccion{Accion: anomalias.AccionExcluirUso}))
	if !errors.Is(err, anomalias.ErrNotaDemasiadoLarga) {
		t.Fatalf("dio %v, se esperaba ErrNotaDemasiadoLarga", err)
	}
	if len(correccionesDe(t, svc).usosExcluidos) != 0 {
		t.Fatal("se excluyo la fila con una nota que la base rechazaria")
	}
}

// Sin el puerto de correcciones, Resolver falla cerrado antes de abrir la
// unidad: cerrar una critica sin poder corregirla es el fallo que #164 cierra.
func TestResolverSinCorreccionesFallaCerrado(t *testing.T) {
	svc, _, alertas, _, unidad := evaluado(t)
	svc.Correcciones = nil
	entradas := unidad.entradas

	if _, err := svc.Resolver(t.Context(), alertaDeTipo(t, alertas, anomalias.TipoONI).ID, "usr-2",
		soloNota("nota")); err == nil || !strings.Contains(err.Error(), "CorreccionDeDatos") {
		t.Fatalf("dio %v, se esperaba el error de cableado", err)
	}
	if unidad.entradas != entradas {
		t.Fatal("abrio la unidad con el servicio a medias")
	}
}
