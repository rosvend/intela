package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rosvend/intela/internal/dominio/identificacion"
)

// Hechos de la resolucion manual de un caso ONI (#175, D8). El front de
// auditoria agrupa el prefijo `identificacion.` bajo "Identificacion".
const (
	HechoIdentificacionAsignada   = "identificacion.asignada"
	HechoIdentificacionDescartada = "identificacion.descartada"
)

// RefUso es el tipo de referencia de un asiento que apunta a un uso. El hermano
// de [RefObra]: asignar referencia la OBRA -- para que GET /auditoria/obra/{id}
// devuelva la resolucion -- y descartar referencia el USO, porque no hay obra.
const RefUso = "uso"

// RepositorioResolucionIdentificacion es lo justo para resolver un caso. Va
// declarado junto a quien lo consume, como [LecturaDeEntregas]: el adaptador no
// tiene por que ofrecer mas de lo que este caso de uso necesita.
//
// *postgres.Store lo satisface entero.
type RepositorioResolucionIdentificacion interface {
	// PeriodoDeUso da el periodo del reporte de la fila. ErrNoEncontrado si no hay fila.
	PeriodoDeUso(ctx context.Context, usoID string) (string, error)

	// BloquearPeriodoDeUsos toma el cerrojo de periodo y exige una unidad abierta.
	BloquearPeriodoDeUsos(ctx context.Context, periodo string) error

	// CasoParaResolver lee el caso bloqueando la fila (FOR UPDATE) y trae sus candidatos.
	CasoParaResolver(ctx context.Context, usoID string) (CasoParaResolver, error)

	// TituloDeObra da el titulo de catalogo. ErrNoEncontrado si esa obra no esta.
	TituloDeObra(ctx context.Context, obraID string) (string, error)

	Alias(ctx context.Context, fuente, tipo, valor string) (string, error)
	GuardarAlias(ctx context.Context, fuente, tipo, valor, obraID, quien string) error

	GuardarResolucionManual(ctx context.Context, r ResolucionManual) error
	CasoIdentificacionPorID(ctx context.Context, usoID string) (CasoIdentificacion, error)
}

// CasoParaResolver es la fila cruda que se va a resolver: el uso, el periodo de
// su reporte y lo que el motor propuso.
type CasoParaResolver struct {
	Uso        UsoPersistido
	Periodo    string
	Candidatos []identificacion.Candidato
}

// ResolucionManual es la escritura: que quedo y quien lo decidio.
type ResolucionManual struct {
	UsoID     string
	Resultado identificacion.Resultado
	ActorID   string
	Cuando    time.Time
	Nota      string
}

// SolicitudResolucion es lo que llega del cuerpo de la peticion. El actor NO
// viaja aqui: sale de la sesion (ADR 0006).
type SolicitudResolucion struct {
	UsoID    string
	Decision string
	ObraID   string
	Nota     string

	// Sello es la propuesta que la bandeja mostro para este caso. Si viene,
	// la aceptacion se mide contra esa propuesta aunque el historial haya
	// cambiado desde el listado. Vacio: no hubo propuesta verificable y se
	// rankea con el historial de este momento.
	Sello string
}

// ResolucionIdentificacion resuelve un caso de la cola manual: le asigna una
// obra o lo descarta (#175).
//
// # Que NO hace
//
// No escribe titulares, porcentajes ni importes: este camino no mueve dinero
// (R-02, R-03, ADR 0007), solo dice a que obra pertenece un uso -- o que no
// pertenece a ninguna --. La ponderacion no viaja en la respuesta: un caso no
// lleva rating, vistas ni taquilla.
//
// No toca `alertas` (D2). La alerta `oni` de #37 se autocierra en la siguiente
// evaluacion, porque su detector filtra por `escalon = 'oni'`.
//
// No corrige una decision manual: reasignar y deshacer un descarte quedan fuera
// de alcance (D4).
//
// # El cerrojo de periodo
//
// La resolucion se serializa con el cerrojo de periodo de #171 -- la misma
// clave que usan la evaluacion de anomalias, la ingesta y la valorizacion --
// para que no se cuele entre la compuerta y el calculo (D3). Por eso se puede
// resolver aunque el periodo este en reparto o ya distribuido.
type ResolucionIdentificacion struct {
	Repo     RepositorioResolucionIdentificacion
	Bitacora BitacoraAuditoria
	Unidad   UnidadDeTrabajo
	Reloj    Reloj

	// Ejemplos y Rankeador guardan la decision como ejemplo etiquetado y
	// anotan si coincidio con la sugerencia (#53). La sugerencia no se aplica:
	// lo que se escribe es lo que pidio la persona.
	Ejemplos  RepositorioEjemplosResolucion
	Rankeador PuertoRankeadorDeResoluciones

	// ClaveSello abre el sello que puso CasosIdentificacion en la propuesta
	// mostrada. Tiene que ser la misma clave; si no, el sello no verifica.
	ClaveSello []byte
}

// Resolver aplica la decision sobre el caso y devuelve el caso ya resuelto.
//
// El orden importa: las validaciones baratas -- actor, nota y forma del pedido
// -- van ANTES de abrir la unidad, para no tomar el cerrojo de un periodo por
// una peticion que se va a rechazar igual.
func (r ResolucionIdentificacion) Resolver(ctx context.Context, s SolicitudResolucion,
	actorID, actorNombre string) (CasoIdentificacion, error) {

	usoID := strings.TrimSpace(s.UsoID)
	if err := exigirActor(actorID, fmt.Sprintf("resolver el caso %q", usoID)); err != nil {
		return CasoIdentificacion{}, err
	}
	nota, err := identificacion.NormalizarNota(s.Nota)
	if err != nil {
		return CasoIdentificacion{}, fmt.Errorf("resolver el caso %q: %w", usoID, err)
	}
	decision := identificacion.Decision(strings.TrimSpace(s.Decision))
	obraID := strings.TrimSpace(s.ObraID)
	if err := identificacion.ValidarDecision(decision, obraID); err != nil {
		return CasoIdentificacion{}, fmt.Errorf("resolver el caso %q: %w", usoID, err)
	}
	if usoID == "" {
		return CasoIdentificacion{}, fmt.Errorf("resolver un caso: %w", ErrNoEncontrado)
	}
	if err := r.cableado(); err != nil {
		return CasoIdentificacion{}, err
	}

	// El sello se abre antes de la unidad: es un dato del cuerpo, y un sello
	// que no verifica no tiene por que tomar el cerrojo del periodo.
	var mostrada *identificacion.Sugerencia
	if sello := strings.TrimSpace(s.Sello); sello != "" {
		vista, err := abrirPropuesta(r.ClaveSello, usoID, sello)
		if err != nil {
			return CasoIdentificacion{}, fmt.Errorf("resolver el caso %q: %w", usoID, ErrPropuestaInvalida)
		}
		mostrada = &vista
	}

	var resuelto CasoIdentificacion
	err = r.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		caso, err := r.resolverEnUnidad(ctx, usoID, decision, obraID, nota, actorID, actorNombre, mostrada)
		if err != nil {
			return err
		}
		resuelto = caso
		return nil
	})
	if err != nil {
		return CasoIdentificacion{}, err
	}
	return resuelto, nil
}

// resolverEnUnidad es el cuerpo de la unidad de trabajo. Todos los puertos se
// llaman con el ctx que recibe fn -- no con el de fuera --: uno llamado con el
// ctx exterior escribiria fuera de la transaccion y se confirmaria aparte.
func (r ResolucionIdentificacion) resolverEnUnidad(ctx context.Context, usoID string,
	decision identificacion.Decision, obraID, nota, actorID, actorNombre string,
	mostrada *identificacion.Sugerencia) (CasoIdentificacion, error) {

	// El cerrojo va antes de bloquear la fila, el mismo orden que la
	// valorizacion y la ingesta: siempre la misma clave, siempre el mismo
	// orden, o dos caminos se bloquean mutuamente.
	periodo, err := r.Repo.PeriodoDeUso(ctx, usoID)
	if err != nil {
		// Sin envolver: quien llama distingue ErrNoEncontrado con errors.Is.
		return CasoIdentificacion{}, err
	}
	if err := r.Repo.BloquearPeriodoDeUsos(ctx, periodo); err != nil {
		return CasoIdentificacion{}, err
	}

	// El reloj se lee UNA vez, con el cerrojo tomado: es el mismo instante para
	// `resuelto_en` y para el asiento, y no puede caer entre dos lecturas.
	ahora := r.Reloj.Ahora()

	caso, err := r.Repo.CasoParaResolver(ctx, usoID)
	if err != nil {
		return CasoIdentificacion{}, err
	}

	res, err := identificacion.ResolverCaso(caso.Uso.Escalon, decision, obraID, caso.Candidatos)
	if err != nil {
		return CasoIdentificacion{}, err
	}

	// La obra se comprueba contra el catalogo antes de escribir: un obra_id que
	// no existe es un dato malo del cuerpo (400), no un fallo del servidor.
	// Ademas da el titulo que el asiento guarda tal como estaba al decidir.
	titulo := ""
	if decision == identificacion.DecisionAsignar {
		titulo, err = r.Repo.TituloDeObra(ctx, obraID)
		if errors.Is(err, ErrNoEncontrado) {
			return CasoIdentificacion{}, fmt.Errorf("resolver el caso %q: %w: %q", usoID, ErrObraInexistente, obraID)
		}
		if err != nil {
			return CasoIdentificacion{}, err
		}
	}

	alias, err := r.aprenderAlias(ctx, caso.Uso, decision, obraID, actorID)
	if err != nil {
		return CasoIdentificacion{}, err
	}

	// Si la persona trae el sello de la bandeja, la aceptacion se mide contra
	// ESA propuesta. Recalcular aqui leeria el historial de este momento, que
	// puede haber cambiado desde el listado, y el ejemplo y el asiento
	// dirian que rechazo una obra que nunca vio. Sin sello no hay propuesta
	// que conservar: se rankea ahora, antes de guardar el ejemplo, para que
	// este caso no se cuente a si mismo. Ni el sello ni el rankeo cambian
	// `res`: eso es lo que pidio la persona (ADR 0007).
	clave := identificacion.ClaveDeTitulo(caso.Uso.Titulo, caso.Uso.TituloOrig)
	var sug identificacion.Sugerencia
	if mostrada != nil {
		sug = *mostrada
	} else {
		sug, err = r.sugerir(ctx, clave, caso.Candidatos)
		if err != nil {
			return CasoIdentificacion{}, err
		}
	}
	aceptada := sug.AceptadaPor(decision, obraID)

	if err := r.Repo.GuardarResolucionManual(ctx, ResolucionManual{
		UsoID: usoID, Resultado: res, ActorID: actorID, Cuando: ahora, Nota: nota,
	}); err != nil {
		return CasoIdentificacion{}, err
	}

	if err := r.Ejemplos.GuardarEjemplo(ctx, EjemploResolucion{
		UsoID: usoID, Clave: clave, Fuente: caso.Uso.Fuente,
		Candidatos: caso.Candidatos, Decision: decision, ObraElegida: obraID,
		SugerenciaDecision: sug.Decision, SugerenciaObraID: sug.ObraID,
		Confianza: sug.Confianza, Motivo: sug.Motivo, Orden: sug.Orden,
		Aceptada: aceptada, ActorID: actorID,
	}); err != nil {
		return CasoIdentificacion{}, err
	}

	// El asiento es parte de la definicion de hecho (ADR 0006): si falla, sube
	// el error y la unidad revierte la fila, el alias y el ejemplo con el.
	payload, err := asientoDeResolucion(asientoResolucion{
		usoID: usoID, decision: decision, obraID: obraID,
		candidatos: caso.Candidatos, resultado: res,
		titulo: titulo, uso: caso.Uso, periodo: caso.Periodo,
		nota: nota, actorNombre: actorNombre, alias: alias,
		sugerencia: sug, aceptada: aceptada,
	})
	if err != nil {
		return CasoIdentificacion{}, err
	}

	refTipo, refID := RefObra, obraID
	if decision == identificacion.DecisionDescartar {
		refTipo, refID = RefUso, usoID
	}
	if err := r.Bitacora.Asentar(ctx, Asiento{
		Hecho:   hechoDe(decision),
		RefTipo: refTipo,
		RefID:   refID,
		ActorID: actorID,
		Payload: payload,
		Cuando:  ahora,
	}); err != nil {
		return CasoIdentificacion{}, fmt.Errorf("asentar la resolucion del caso %q: %w", usoID, err)
	}

	// Se relee dentro de la unidad para que la respuesta sea atomica con la
	// escritura: lo que se devuelve es lo que quedo guardado.
	leido, err := r.Repo.CasoIdentificacionPorID(ctx, usoID)
	if err != nil {
		return CasoIdentificacion{}, err
	}
	completo, err := completarCaso(leido)
	if err != nil {
		return CasoIdentificacion{}, err
	}
	ya := aceptada
	completo.Sugerencia = sugerenciaVisible(sug, completo.Candidatos, &ya)
	return completo, nil
}

// sugerir rankea el caso contra el historial ya guardado. Una clave vacia no
// consulta: dos filas sin titulo no son el mismo caso, y traerlas todas no
// aportaria nada.
func (r ResolucionIdentificacion) sugerir(ctx context.Context, clave string, candidatos []identificacion.Candidato) (identificacion.Sugerencia, error) {
	var claves []string
	if clave != "" {
		claves = []string{clave}
	}
	historia, err := r.Ejemplos.HistoriaPorClaves(ctx, claves)
	if err != nil {
		return identificacion.Sugerencia{}, err
	}
	return r.Rankeador.Rankear(identificacion.PedidoTriage{
		Clave: clave, Candidatos: candidatos, Historia: historia,
	}), nil
}

// aprenderAlias resuelve el escalon 1 de aqui en adelante (ADR 0007: resolver
// una vez, reutilizar siempre). Devuelve nil -- y el asiento lo dice -- cuando
// no hay par local que aprender.
//
// El conflicto (D6) se rechaza ANTES de escribir nada: el par ya apunta a OTRA
// obra. Es un caso real -- dos emisiones del mismo programa, se resuelve una y
// la otra sigue pendiente --, y pisar el alias desharia en silencio una
// decision anterior. Si ya apunta a la MISMA obra no es error: no se aprende
// nada nuevo.
func (r ResolucionIdentificacion) aprenderAlias(ctx context.Context, u UsoPersistido,
	decision identificacion.Decision, obraID, actorID string) (*aliasAprendido, error) {

	if decision != identificacion.DecisionAsignar {
		return nil, nil
	}
	e := entradaDesdeUso(u)
	if e.TipoID == "" || e.ValorID == "" {
		return nil, nil
	}

	aprendido := &aliasAprendido{Fuente: e.Fuente, TipoID: e.TipoID, Valor: e.ValorID}

	actual, err := r.Repo.Alias(ctx, e.Fuente, e.TipoID, e.ValorID)
	switch {
	case err == nil && actual == obraID:
		return aprendido, nil
	case err == nil:
		return nil, fmt.Errorf("resolver el uso %q: %w: %s %s=%s ya apunta a %q",
			u.ID, ErrAliasEnConflicto, e.Fuente, e.TipoID, e.ValorID, actual)
	case !errors.Is(err, ErrNoEncontrado):
		// Un fallo de base no es "no hay alias": tragarlo reclasificaria la
		// fila en silencio (D8 de la cascada).
		return nil, err
	}

	if err := r.Repo.GuardarAlias(ctx, e.Fuente, e.TipoID, e.ValorID, obraID, actorID); err != nil {
		return nil, err
	}
	// Relectura: GuardarAlias es ON CONFLICT DO NOTHING, asi que si otra
	// peticion gano la carrera el alias apunta a su obra y no a la nuestra.
	final, err := r.Repo.Alias(ctx, e.Fuente, e.TipoID, e.ValorID)
	if err != nil {
		return nil, err
	}
	if final != obraID {
		return nil, fmt.Errorf("resolver el uso %q: %w: %s %s=%s ya apunta a %q",
			u.ID, ErrAliasEnConflicto, e.Fuente, e.TipoID, e.ValorID, final)
	}
	aprendido.Aprendido = true
	return aprendido, nil
}

// aliasAprendido es el alias en el payload del asiento: dice cual se aprendio y
// si de verdad se escribio o ya estaba.
type aliasAprendido struct {
	Fuente    string
	TipoID    string
	Valor     string
	Aprendido bool
}

// asientoResolucion son los datos crudos del asiento, para no pasar ocho
// parametros sueltos por la funcion que arma el payload.
type asientoResolucion struct {
	usoID       string
	decision    identificacion.Decision
	obraID      string
	candidatos  []identificacion.Candidato
	resultado   identificacion.Resultado
	titulo      string
	uso         UsoPersistido
	periodo     string
	nota        string
	actorNombre string
	alias       *aliasAprendido
	sugerencia  identificacion.Sugerencia
	aceptada    bool
}

// payloadResolucion es el JSON del asiento. snake_case y decimales como string,
// igual que [AsientoValorizacion].
type payloadResolucion struct {
	UsoID     string `json:"uso_id"`
	Decision  string `json:"decision"`
	ObraID    string `json:"obra_id,omitempty"`
	Candidata *bool  `json:"candidata,omitempty"`
	// Puntaje solo va cuando la obra elegida estaba entre las propuestas: una
	// obra buscada en el catalogo no tiene puntaje que heredar.
	Puntaje              string `json:"puntaje,omitempty"`
	CandidatosPropuestos int    `json:"candidatos_propuestos,omitempty"`

	// El estado del uso TAL COMO ESTABA al decidir (D5 del comentario de
	// diseno): la fila se sobreescribe, y sin esto no queda en ningun sitio
	// contra que se decidio.
	Titulo            string `json:"titulo"`
	TituloOriginal    string `json:"titulo_original"`
	Fuente            string `json:"fuente"`
	Periodo           string `json:"periodo"`
	ReporteID         string `json:"reporte_id"`
	IDsFuente         string `json:"ids_fuente"`
	EscalonAnterior   string `json:"escalon_anterior"`
	EvidenciaAnterior string `json:"evidencia_anterior"`

	Nota        string         `json:"nota"`
	ActorNombre string         `json:"actor_nombre"`
	Alias       *aliasAsentado `json:"alias"`

	// Lo que se habia sugerido y si la persona lo confirmo (#53). La decision
	// del asiento sigue siendo la suya: estos campos solo dicen si coincidieron.
	SugerenciaDecision string `json:"sugerencia_decision"`
	SugerenciaObraID   string `json:"sugerencia_obra_id,omitempty"`
	SugerenciaAceptada bool   `json:"sugerencia_aceptada"`
}

type aliasAsentado struct {
	Fuente    string `json:"fuente"`
	TipoID    string `json:"tipo_id"`
	Valor     string `json:"valor"`
	Aprendido bool   `json:"aprendido"`
}

func asientoDeResolucion(d asientoResolucion) ([]byte, error) {
	p := payloadResolucion{
		UsoID:              d.usoID,
		Decision:           string(d.decision),
		ObraID:             d.obraID,
		Titulo:             d.uso.Titulo,
		TituloOriginal:     d.uso.TituloOrig,
		Fuente:             d.uso.Fuente,
		Periodo:            d.periodo,
		ReporteID:          d.uso.ReporteID,
		IDsFuente:          d.uso.IDsFuente,
		EscalonAnterior:    d.uso.Escalon,
		EvidenciaAnterior:  d.uso.Evidencia,
		Nota:               d.nota,
		ActorNombre:        d.actorNombre,
		SugerenciaDecision: d.sugerencia.Decision,
		SugerenciaObraID:   d.sugerencia.ObraID,
		SugerenciaAceptada: d.aceptada,
	}

	if d.decision == identificacion.DecisionAsignar {
		candidata := esCandidata(d.obraID, d.candidatos)
		p.Candidata = &candidata
		p.CandidatosPropuestos = len(d.candidatos)
		if candidata {
			p.Puntaje = d.resultado.Puntaje.StringFixed(5)
		}
	}
	if d.alias != nil {
		p.Alias = &aliasAsentado{
			Fuente: d.alias.Fuente, TipoID: d.alias.TipoID,
			Valor: d.alias.Valor, Aprendido: d.alias.Aprendido,
		}
	}

	bruto, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("serializar el asiento de la resolucion del uso %q: %w", d.usoID, err)
	}
	return bruto, nil
}

func esCandidata(obraID string, candidatos []identificacion.Candidato) bool {
	for _, c := range candidatos {
		if c.ObraID == obraID {
			return true
		}
	}
	return false
}

func hechoDe(d identificacion.Decision) string {
	if d == identificacion.DecisionDescartar {
		return HechoIdentificacionDescartada
	}
	return HechoIdentificacionAsignada
}

// cableado comprueba las dependencias antes de abrir una unidad, por lo mismo
// que [Anomalias.cableado]: un servicio a medias paniqueaba DENTRO de la
// transaccion ya abierta y con el mensaje apuntando a la dependencia equivocada.
func (r ResolucionIdentificacion) cableado() error {
	switch {
	case r.Repo == nil:
		return errors.New("resolucion de identificacion mal cableada: falta RepositorioResolucionIdentificacion")
	case r.Bitacora == nil:
		return errors.New("resolucion de identificacion mal cableada: falta BitacoraAuditoria")
	case r.Unidad == nil:
		return errors.New("resolucion de identificacion mal cableada: falta UnidadDeTrabajo")
	case r.Reloj == nil:
		return errors.New("resolucion de identificacion mal cableada: falta Reloj")
	case r.Ejemplos == nil:
		return errors.New("resolucion de identificacion mal cableada: falta RepositorioEjemplosResolucion")
	case r.Rankeador == nil:
		return errors.New("resolucion de identificacion mal cableada: falta PuertoRankeadorDeResoluciones")
	}
	return nil
}
