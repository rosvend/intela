package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rosvend/intela/internal/dominio/anomalias"
	"github.com/rosvend/intela/internal/dominio/recaudo"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Hechos que la deteccion de anomalias asienta en la bitacora (ADR 0006).
//
// Constantes y no literales por lo mismo que las del catalogo: el hecho es la
// clave por la que se consulta el libro -- hay un indice `asientos_hecho` --,
// y una errata no rompe nada hoy, deja un asiento que nadie encuentra dentro
// de diez anos.
const (
	// HechoAnomaliasEvaluadas es una pasada completa sobre un periodo.
	HechoAnomaliasEvaluadas = "anomalias.evaluadas"
	// HechoAlertaResuelta es la decision humana sobre una alerta.
	HechoAlertaResuelta = "alerta.resuelta"
	// HechoAlertaAutocerrada: una reevaluacion ya no detecto la anomalia y el sistema la cerro.
	HechoAlertaAutocerrada = "alerta.autocerrada"
	// HechoAlertaReabierta: una reevaluacion volvio a detectar una alerta autocerrada y el sistema la reabrio.
	HechoAlertaReabierta = "alerta.reabierta"
)

// notaAutocierre es la nota que deja el sistema al autocerrar una alerta.
const notaAutocierre = "cerrada por el sistema: la reevaluacion del periodo ya no la detecta"

// RefPeriodo y RefAlerta son los tipos de referencia de esos dos asientos.
//
// RefPeriodo es propio y no reutiliza [RefObra]: una evaluacion no es un hecho
// DE una obra, es un hecho del periodo, y mezclarlos haria que el historial de
// una obra trajera pasadas enteras que no la nombran.
const (
	RefPeriodo = "periodo"
	RefAlerta  = "alerta"
)

// LecturaDeEntregas es lo que la evaluacion necesita de la ingesta: las filas
// canonicas de un periodo y el acuse de cada entrega recibida.
//
// Se declara aqui, junto a quien la consume, y solo con los dos metodos de
// LECTURA, por la misma razon que [LectorDeDeclaraciones] existe aparte de
// [GestionDeclaraciones]: [RepositorioIngesta] tiene cuatro metodos de
// escritura -- GuardarEntrega entre ellos, que QUEMA la huella de un archivo --
// y una pasada de deteccion no tiene nada que hacer con ninguno. Quitarselos al
// puerto convierte "no deberia escribir" en "no puede".
type LecturaDeEntregas interface {
	UsosDePeriodo(ctx context.Context, periodo string) ([]UsoPersistido, error)

	// EntregasRecibidas devuelve TODAS las entregas conocidas, no solo las de
	// un periodo. La evaluacion las pide asi a proposito: ver
	// [Anomalias.Evaluar].
	//
	// Y devuelve [EntregaRecibida] y no [CargaReporte] por coste: de cada
	// entrega aqui hacen falta cuatro campos, y `ListarCargas` calcula ademas
	// dos COUNT correlacionados por fila que esta lectura tira -- 130,8 ms
	// contra 4,8 ms sobre 5.001 reportes, en cada pasada.
	EntregasRecibidas(ctx context.Context) ([]EntregaRecibida, error)
}

// LectorDeCoautores entrega los coautores registrados de un punado de obras en
// UNA consulta.
//
// Existe aparte de [CatalogoObras] -- que ya sabe leer una obra entera -- por
// la forma de la pregunta, no por gusto: aqui hacen falta los coautores de las
// N obras que el periodo pondera, y `PorID` una por una son N viajes contra la
// base. Es exactamente la misma razon por la que
// [GestionDeclaraciones.VigentesDeObras] existe aparte de `VigenteEn`.
//
// Una obra sin entrada en el mapa es una obra sin coautores registrados, que
// el catalogo no permite crear ([repertorio.NuevaObra] exige al menos uno)
// pero que una fila escrita por SQL directo si podria dejar.
type LectorDeCoautores interface {
	CoautoresDeObras(ctx context.Context, obraIDs []string) (map[string][]repertorio.Coautor, error)
}

// Anomalias es el paso de revision que corre sobre un periodo ARMADO, antes de
// que se reparta (OE-5 / KR-4).
//
// # Que hace y que no
//
// Reune el periodo, se lo pasa al dominio ([anomalias.Detectar], que es puro),
// y persiste lo que sale. No decide reglas: los seis criterios viven en
// `internal/dominio/anomalias` y `R-04` lo decide
// [repertorio.Declaracion.Completa]. No toca dinero y no reparte.
//
// Es ademas la [CompuertaAnomalias] que [Procesos] consulta antes de calcular (ver [Anomalias.Bloqueantes]).
type Anomalias struct {
	Entregas LecturaDeEntregas
	// Declaraciones es el MISMO puerto estrecho que usa el catalogo: la
	// version abierta de un conjunto de obras, sin poder escribir ninguna.
	Declaraciones LectorDeDeclaraciones
	Coautores     LectorDeCoautores
	Alertas       RepositorioAlertas
	// Correcciones es lo que escribe sobre el dato al cerrar una critica (#164).
	Correcciones CorreccionDeDatos
	Bitacora     BitacoraAuditoria
	Unidad       UnidadDeTrabajo
	Reloj        Reloj
}

// ResumenEvaluacion es lo que devuelve una pasada.
//
// Detectadas y Nuevas son dos cifras distintas y las dos hacen falta:
// Detectadas es cuantas anomalias tiene el periodo AHORA, Nuevas cuantas de
// esas no estaban antes de esta pasada. Con una sola, correr la evaluacion dos
// veces seguidas daria "0" la segunda y se leeria como "el periodo esta
// limpio" cuando lo que pasa es que ya estaban todas escritas.
type ResumenEvaluacion struct {
	Periodo    string
	Detectadas int
	Nuevas     int

	// PorTipo cuenta lo detectado en ESTA pasada, por tipo, con los seis tipos
	// siempre presentes -- un cero explicito y no una clave ausente, para que
	// el tablero pueda pintar las seis tarjetas sin inventarse ninguna.
	PorTipo map[string]int

	// CriticasAbiertas son las alertas sin resolver del periodo cuyo tipo
	// bloquea la distribucion. Es lo que lee la compuerta ([Anomalias.Bloqueantes]).
	CriticasAbiertas int

	// CriticasAceptadas son las criticas del periodo que una persona cerro
	// aceptandolas tal cual, sin corregir el dato (#164). No bloquean; se
	// cuentan aparte para que "cero abiertas" no se lea como "cero anomalias".
	CriticasAceptadas int

	// Autocerradas son las abiertas que esta pasada ya no detecto y el sistema cerro.
	Autocerradas int

	// UsosSinCotejar son las filas del periodo a las que no se les pudo
	// componer clave de registro, asi que el detector de duplicados no las
	// comparo con ninguna otra ([anomalias.SinClaveDeRegistro]).
	//
	// No es un conteo de anomalias: es el TAMANO DEL PUNTO CIEGO. Sin esta
	// cifra, "cero duplicados" y "no se miro" se leen igual en el tablero, y
	// la segunda lectura es la que deja pasar una emision contada dos veces.
	// Es la misma disciplina que `aplicacion.Reparto.UsosSinCanal`.
	UsosSinCotejar int
}

// Evaluar corre los seis detectores sobre un periodo y persiste lo que
// encuentra.
//
// Es IDEMPOTENTE: dos pasadas seguidas sobre el mismo dato dejan las mismas
// filas, porque la clave natural de `alertas` es la identidad del hallazgo
// (ver [RepositorioAlertas]). Se puede correr al cerrar la ingesta, otra vez
// despues de que un autor declare, y otra antes de la compuerta.
//
// # Por que las entregas se piden TODAS y los usos solo los del periodo
//
// Porque son dos preguntas distintas. Los usos que ponderan este periodo son
// los de este periodo, y punto. Las entregas no: el UNIQUE (sha256, fuente) de
// `reportes` no lleva el periodo, asi que los mismos bytes pueden estar en un
// mes anterior bajo otra fuente, y esa colision es justamente la que el UNIQUE
// deja pasar. Pedir solo las del periodo dejaria ciega la mitad del detector.
// La alerta se levanta igualmente sobre la entrega DE ESTE periodo; la de
// fuera solo se nombra.
//
// # Por que el asiento entra en la misma unidad que las alertas
//
// ADR 0006: un caso de uso cuyo asiento fallo no esta hecho. Si las alertas se
// escribieran y el asiento no, el tablero diria que hubo una pasada que la
// bitacora no registra, y la pregunta "quien vio esto y cuando" -- la que hace
// una auditoria de `RD 16` -- se quedaria sin respuesta.
//
// El actor puede venir vacio: una pasada la puede disparar el pipeline sin que
// haya nadie delante, y el ADR 0006 pide la firma para la DECISION MANUAL. Por
// eso aqui no hay exigirActor y en [Anomalias.Resolver] si.
func (a Anomalias) Evaluar(ctx context.Context, periodo, actorID string) (ResumenEvaluacion, error) {
	periodo, err := recaudo.ValidarPeriodo(periodo)
	if err != nil {
		return ResumenEvaluacion{}, err
	}
	if err := a.cableado(); err != nil {
		return ResumenEvaluacion{}, err
	}

	var resumen ResumenEvaluacion
	err = a.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		// Cerrojo por periodo ANTES de leer: la foto y el autocierre salen de la misma pasada serializada (ADR 0021).
		if err := a.Alertas.BloquearAlertasDePeriodo(ctx, periodo); err != nil {
			return fmt.Errorf("serializar la evaluacion de %q: %w", periodo, err)
		}
		// El reloj se lee con el cerrojo tomado: leido antes, la pasada que espero asentaria una reapertura
		// con un instante anterior al autocierre de la que gano, y la bitacora terminaria en autocerrada.
		ahora := a.Reloj.Ahora()
		armado, err := a.armarPeriodo(ctx, periodo)
		if err != nil {
			return err
		}

		hallazgos := anomalias.Detectar(armado)
		alertas := make([]Alerta, 0, len(hallazgos))
		porTipo := conteoVacioPorTipo()
		for _, h := range hallazgos {
			porTipo[h.Tipo]++
			alertas = append(alertas, Alerta{
				Periodo:    periodo,
				Tipo:       h.Tipo,
				RefTipo:    h.RefTipo,
				RefID:      h.RefID,
				RefTitular: h.RefTitular,
				Detalle:    h.Detalle,
				Detectada:  ahora,
			})
		}

		resumen = ResumenEvaluacion{
			Periodo:        periodo,
			Detectadas:     len(hallazgos),
			PorTipo:        porTipo,
			UsosSinCotejar: anomalias.SinClaveDeRegistro(armado.Usos),
		}

		nuevas, reabiertas, err := a.Alertas.GuardarAlertas(ctx, alertas)
		if err != nil {
			return fmt.Errorf("guardar las alertas de %q: %w", periodo, err)
		}
		resumen.Nuevas = nuevas
		for _, r := range reabiertas {
			if err := a.asentarDecisionDeSistema(ctx, HechoAlertaReabierta, r, ahora); err != nil {
				return err
			}
		}

		cerradas, err := a.Alertas.AutocerrarAlertas(ctx, periodo, alertas, notaAutocierre, ahora)
		if err != nil {
			return fmt.Errorf("autocerrar las alertas rancias de %q: %w", periodo, err)
		}
		resumen.Autocerradas = len(cerradas)
		for _, c := range cerradas {
			if err := a.asentarDecisionDeSistema(ctx, HechoAlertaAutocerrada, c, ahora); err != nil {
				return err
			}
		}

		payload, err := json.Marshal(struct {
			Periodo        string         `json:"periodo"`
			Detectadas     int            `json:"detectadas"`
			Nuevas         int            `json:"nuevas"`
			Autocerradas   int            `json:"autocerradas"`
			PorTipo        map[string]int `json:"por_tipo"`
			Usos           int            `json:"usos_evaluados"`
			Obras          int            `json:"obras_evaluadas"`
			Entregas       int            `json:"entregas_cotejadas"`
			UsosSinCotejar int            `json:"usos_sin_cotejar"`
		}{
			Periodo: periodo, Detectadas: resumen.Detectadas, Nuevas: nuevas, Autocerradas: resumen.Autocerradas, PorTipo: porTipo,
			Usos: len(armado.Usos), Obras: len(armado.Obras), Entregas: len(armado.Entregas),
			// El tamano del punto ciego queda en el asiento y no solo en la
			// respuesta: quien audite la pasada dentro de diez anos tiene que
			// poder saber sobre cuantas filas NO se miro (`RD 16`).
			UsosSinCotejar: resumen.UsosSinCotejar,
		})
		if err != nil {
			return fmt.Errorf("serializar el asiento de la evaluacion de %q: %w", periodo, err)
		}
		if err := a.Bitacora.Asentar(ctx, Asiento{
			Hecho:   HechoAnomaliasEvaluadas,
			RefTipo: RefPeriodo,
			RefID:   periodo,
			ActorID: actorID,
			Payload: payload,
			Cuando:  ahora,
		}); err != nil {
			return fmt.Errorf("asentar la evaluacion de %q: %w", periodo, err)
		}
		// Se cuenta dentro del cerrojo: otra pasada no puede cambiar la bandeja entre guardar y contar.
		resumen.CriticasAbiertas, err = a.CriticasAbiertas(ctx, periodo)
		if err != nil {
			return err
		}
		resumen.CriticasAceptadas, err = a.Alertas.ContarAlertasConAccion(ctx, periodo, anomalias.AccionAceptarTalCual)
		if err != nil {
			return fmt.Errorf("contar criticas aceptadas tal cual de %q: %w", periodo, err)
		}
		return nil
	})
	if err != nil {
		return ResumenEvaluacion{}, err
	}
	return resumen, nil
}

// asentarDecisionDeSistema deja el asiento de una alerta que el sistema autocerro o reabrio (actor de sistema, ADR 0006).
func (a Anomalias) asentarDecisionDeSistema(ctx context.Context, hecho string, c Alerta, ahora time.Time) error {
	payload, err := json.Marshal(struct {
		Tipo       string `json:"tipo"`
		Periodo    string `json:"periodo"`
		RefTipo    string `json:"ref_tipo"`
		RefID      string `json:"ref_id"`
		RefTitular string `json:"ref_titular,omitempty"`
		Detalle    string `json:"detalle"`
		Nota       string `json:"nota,omitempty"`
	}{
		Tipo: c.Tipo, Periodo: c.Periodo, RefTipo: c.RefTipo, RefID: c.RefID,
		RefTitular: c.RefTitular, Detalle: c.Detalle, Nota: c.Nota,
	})
	if err != nil {
		return fmt.Errorf("serializar %s de la alerta %q: %w", hecho, c.ID, err)
	}
	if err := a.Bitacora.Asentar(ctx, Asiento{
		Hecho:   hecho,
		RefTipo: RefAlerta,
		RefID:   c.ID,
		ActorID: actorSistema,
		Payload: payload,
		Cuando:  ahora,
	}); err != nil {
		return fmt.Errorf("asentar %s de la alerta %q: %w", hecho, c.ID, err)
	}
	return nil
}

// armarPeriodo reune lo que los detectores necesitan. Es la unica parte de
// este fichero que hace E/S: el dominio recibe el resultado ya armado.
func (a Anomalias) armarPeriodo(ctx context.Context, periodo string) (anomalias.Periodo, error) {
	usos, err := a.Entregas.UsosDePeriodo(ctx, periodo)
	if err != nil {
		return anomalias.Periodo{}, fmt.Errorf("usos del periodo %q: %w", periodo, err)
	}

	// TODAS las entregas conocidas. Ver el comentario de [Anomalias.Evaluar]
	// sobre por que no se filtra por periodo.
	cargas, err := a.Entregas.EntregasRecibidas(ctx)
	if err != nil {
		return anomalias.Periodo{}, fmt.Errorf("entregas recibidas: %w", err)
	}

	armado := anomalias.Periodo{
		Periodo:  periodo,
		Usos:     make([]anomalias.Uso, 0, len(usos)),
		Entregas: make([]anomalias.Entrega, 0, len(cargas)),
	}
	for _, u := range usos {
		armado.Usos = append(armado.Usos, usoDeDominio(u))
	}
	for _, c := range cargas {
		armado.Entregas = append(armado.Entregas, entregaDeDominio(c))
	}

	armado.Obras, err = a.obrasDelPeriodo(ctx, armado.Usos)
	if err != nil {
		return anomalias.Periodo{}, err
	}
	return armado, nil
}

// usoDeDominio proyecta una fila persistida en lo que miran los detectores.
// Una sola traduccion para la evaluacion y para la correccion (#164): la
// exclusion se valida contra la MISMA clave de registro que levanto la alerta.
func usoDeDominio(u UsoPersistido) anomalias.Uso {
	return anomalias.Uso{
		ID:        u.ID,
		ReporteID: u.ReporteID,
		Fuente:    u.Fuente,
		Titulo:    u.Titulo,
		Escalon:   u.Escalon,
		ObraID:    u.ObraID,
		TipoObra:  u.TipoObra,
		// El dominio de anomalias no puede importar `reparto` (ADR 0003),
		// asi que la modalidad cruza la frontera como string. La necesita
		// para no avisar de `tipo_obra` en una corrida que no lo lee.
		Modalidad: string(u.Modalidad),
		// La clave logica se deriva AQUI y no en el dominio: el
		// vocabulario de ids_fuente es del ADR 0018 y vive en
		// idsfuente.go, que el dominio no puede importar (ADR 0002).
		ClaveRegistro: ClaveDeRegistro(u.Fuente, u.IDsFuente, u.Fecha, u.Hora),
	}
}

// entregaDeDominio proyecta el acuse de una entrega en lo que mira el detector de huella.
func entregaDeDominio(c EntregaRecibida) anomalias.Entrega {
	return anomalias.Entrega{
		ID:       c.ID,
		Fuente:   c.Fuente,
		Periodo:  c.Periodo,
		SHA256:   c.SHA256,
		Excluida: c.Excluida,
	}
}

// obrasDelPeriodo reune las obras que el periodo pondera, con sus coautores y
// su declaracion vigente.
//
// El conjunto sale de los usos y NO del catalogo entero: la pregunta de este
// paso es que impide repartir ESTE periodo, y una obra que nadie emitio no lo
// impide. Evaluar el catalogo completo llenaria el tablero de obras retenidas
// que no tienen dinero esperando.
func (a Anomalias) obrasDelPeriodo(ctx context.Context, usos []anomalias.Uso) ([]anomalias.Obra, error) {
	ids := make([]string, 0, len(usos))
	for _, u := range usos {
		if u.ObraID == "" {
			continue
		}
		ids = append(ids, u.ObraID)
	}
	// Ordenar y deduplicar: los usos vienen ordenados por id de uso, no por
	// obra, y el mismo id repetido pediria la misma obra N veces. El orden
	// final es el de la obra, que es lo que hace estable el recorrido del
	// dominio (ADR 0005).
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil, nil
	}

	coautores, err := a.Coautores.CoautoresDeObras(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("coautores de las obras del periodo: %w", err)
	}
	vigentes, err := a.Declaraciones.VigentesDeObras(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("declaraciones vigentes de las obras del periodo: %w", err)
	}

	obras := make([]anomalias.Obra, 0, len(ids))
	for _, id := range ids {
		o := anomalias.Obra{ID: id}
		for _, c := range coautores[id] {
			o.CoautoresIPI = append(o.CoautoresIPI, c.IPI)
		}
		// Los coautores llegan en el orden del adaptador; se ordenan aqui
		// porque el dominio los recorre y el mensaje de la alerta nombra un
		// IPI concreto.
		//
		// Y se COMPACTAN, igual que `ids` arriba y por una razon mas concreta:
		// la clave primaria de `obra_coautores` es (obra_id, ipi, ROL), y
		// `normalizarCoautores` solo prohibe repetir el PAR (IPI, rol). Una
		// guionista que ademas es adaptadora de la misma obra son dos filas
		// legitimas del catalogo con el mismo IPI (`RD 7.3`). Aqui el rol no se
		// mira: lo que el dominio pregunta es a QUIEN le falta declarar, y a esa
		// persona le falta UNA vez.
		//
		// Sin compactar, esa obra levantaba DOS hallazgos identicos. El segundo
		// no llega a la tabla -- GuardarAlertas deduplica el lote por la clave
		// natural (periodo, tipo, ref_tipo, ref_id, ref_titular) -- pero SI
		// cuenta en Detectadas y en PorTipo, asi que el resumen decia 2 donde la
		// bandeja tiene 1. Y ese resumen se serializa en el asiento de la
		// bitacora, que es append-only y no se corrige nunca (ADR 0006).
		slices.Sort(o.CoautoresIPI)
		o.CoautoresIPI = slices.Compact(o.CoautoresIPI)

		// Ausencia en el mapa es "esta obra no tiene ninguna declaracion", que
		// NO es lo mismo que una version abierta sin partes: lo dice el
		// contrato de VigentesDeObras y es la distincion que decide si el
		// detector de titulares tiene a quien nombrar.
		if v, hay := vigentes[id]; hay {
			o.Declarada = true
			o.Declaracion = v.Declaracion
		}
		obras = append(obras, o)
	}
	return obras, nil
}

// Listar sirve la bandeja de un periodo.
//
// Un periodo mal formado se RECHAZA con el validador del dominio en vez de
// ignorarse, por lo mismo que [Recaudo.Listar]: ignorado devolveria las
// alertas de TODOS los periodos y quien pregunta por `2025-13` lo leeria como
// "ese mes no tuvo anomalias" en vez de "ese mes no existe".
//
// Un tipo desconocido tambien se rechaza: devolver la lista vacia haria pasar
// una errata de grafia -- `oni_` en vez de `oni` -- por un periodo limpio.
func (a Anomalias) Listar(ctx context.Context, f FiltroAlertas) ([]Alerta, error) {
	if strings.TrimSpace(f.Periodo) != "" {
		periodo, err := recaudo.ValidarPeriodo(f.Periodo)
		if err != nil {
			return nil, err
		}
		f.Periodo = periodo
	} else {
		f.Periodo = ""
	}

	f.Tipo = strings.TrimSpace(f.Tipo)
	if f.Tipo != "" && !anomalias.EsTipo(f.Tipo) {
		return nil, fmt.Errorf("%w: tipo de anomalia %q, se esperaba uno de %v",
			ErrFiltroInvalido, f.Tipo, anomalias.Tipos())
	}

	// El defecto lo pone el nucleo, igual que en BuscarObras: una llamada
	// interna que no diga nada de paginacion no tiene por que traerse la tabla
	// entera. Los valores ilegales los rechaza el adaptador HTTP con 400.
	f.Paginacion = f.ConDefecto()

	alertas, err := a.Alertas.ListarAlertas(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("listar alertas: %w", err)
	}
	for i := range alertas {
		alertas[i].Critica = anomalias.EsCritica(alertas[i].Tipo)
	}
	return alertas, nil
}

// SolicitudCierreAlerta es lo que llega para cerrar una alerta. El actor NO
// viaja aqui: sale de la sesion (ADR 0006), igual que en
// [SolicitudResolucion].
type SolicitudCierreAlerta struct {
	Nota string
	// ActorRol es el rol de la sesion de quien cierra. Se guarda en la alerta y
	// en el asiento tal como estaba al cerrar: el rol de una cuenta cambia, y la
	// pregunta de la auditoria es con que rol se tomo ESTA decision.
	ActorRol string
	// Correccion es la accion sobre el dato (#164). Obligatoria en las criticas.
	Correccion anomalias.PedidoDeCorreccion
}

// Resolver cierra una alerta a nombre de quien la cierra y, si es critica,
// corrige el dato en la misma unidad (#164).
//
// # El actor, la nota y la forma del pedido se exigen ANTES de escribir nada
//
// Es una decision humana sobre una anomalia que afecta a quien cobra, que es
// exactamente el caso en el que el ADR 0006 pide saber quien la tomo. Un
// actorID vacio dejaria un asiento sin firmar -- valido para la base, invalido
// para el ADR --, y el UPDATE ya estaria hecho cuando eso se notara. La forma
// del pedido ([anomalias.ValidarForma]) tambien va antes: no se toma el
// cerrojo de un periodo por un cuerpo que se va a rechazar igual.
//
// # Una critica no se cierra sin tocar el dato
//
// Hasta #164 cerrar una critica solo dejaba una nota, y la fila duplicada
// seguia ponderando: la compuerta se abria y el doble conteo se pagaba igual.
// Ahora cerrar una critica exige una accion ([anomalias.AccionesDe]): excluir
// la copia que no manda, excluir la entrega repetida, asignar el tipo de obra,
// o -- solo en los duplicados -- aceptarla tal cual, que la compuerta deja
// pasar pero cuenta aparte ([EstadoCompuerta]).
//
// La accion corre con el cerrojo del periodo tomado, el mismo de la
// evaluacion, la ingesta y la valorizacion (#171): no puede colarse entre la
// compuerta y el calculo de una corrida. La foto contra la que se valida (los
// usos del periodo, las entregas) se lee DESPUES del cerrojo.
//
// # El cierre, la correccion y sus asientos van en la misma unidad
//
// Por lo mismo que en [Catalogo.RegistrarObra]: una alerta cerrada cuyo
// asiento fallo no esta cerrada, y una fila excluida sin su asiento es un
// cambio de reparto que nadie puede explicar. Son dos asientos: el cierre
// (`alerta.resuelta`, sobre la alerta) y la correccion (`correccion.*`, sobre
// el registro que cambio), para que el historial de una fila o de una entrega
// cuente por que dejo de ponderar sin pasar por la bandeja.
//
// Una no critica se cierra solo con la nota, como antes: su dato se corrige en
// otro sitio (la declaracion en el catalogo, el ONI en la bandeja de
// identificacion) y la evaluacion la autocierra cuando deja de verla.
func (a Anomalias) Resolver(ctx context.Context, id, actorID string, s SolicitudCierreAlerta) (Alerta, error) {
	id = strings.TrimSpace(id)
	if err := exigirActor(actorID, fmt.Sprintf("resolver la alerta %q", id)); err != nil {
		return Alerta{}, err
	}
	nota := strings.TrimSpace(s.Nota)
	if nota == "" {
		return Alerta{}, fmt.Errorf("resolver la alerta %q: %w", id, ErrNotaObligatoria)
	}
	pedido, err := anomalias.ValidarForma(s.Correccion)
	if err != nil {
		return Alerta{}, fmt.Errorf("resolver la alerta %q: %w", id, err)
	}
	if id == "" {
		return Alerta{}, fmt.Errorf("resolver una alerta: %w", ErrNoEncontrado)
	}
	if err := a.cableadoParaResolver(); err != nil {
		return Alerta{}, err
	}
	rol := strings.TrimSpace(s.ActorRol)

	var resuelta Alerta
	err = a.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		alerta, err := a.Alertas.AlertaPorID(ctx, id)
		if err != nil {
			// Sin envolver: quien llama distingue ErrNoEncontrado con errors.Is.
			return err
		}
		if alerta.Resuelta {
			return fmt.Errorf("resolver la alerta %q: %w", id, ErrAlertaYaResuelta)
		}
		correccion, err := anomalias.CorreccionPara(alerta.Tipo, alerta.RefID, pedido)
		if err != nil {
			return fmt.Errorf("resolver la alerta %q: %w", id, err)
		}
		if err := anomalias.ValidarNota(correccion, nota); err != nil {
			return fmt.Errorf("resolver la alerta %q: %w", id, err)
		}
		if correccion.Accion == anomalias.AccionAceptarTalCual && rol == "" {
			return fmt.Errorf("resolver la alerta %q: aceptarla tal cual exige el rol de quien firma: %w", id, ErrActorAusente)
		}

		if correccion.Accion != "" {
			// El cerrojo antes de leer la foto y antes del reloj, igual que Evaluar:
			// la correccion y su instante quedan dentro de la pasada serializada.
			if err := a.Alertas.BloquearAlertasDePeriodo(ctx, alerta.Periodo); err != nil {
				return fmt.Errorf("serializar la correccion de la alerta %q: %w", id, err)
			}
		}
		ahora := a.Reloj.Ahora()

		// El UPDATE lleva `AND NOT resuelta`: si otra persona la cerro entre la
		// lectura y aqui, sale ErrAlertaYaResuelta y la unidad revierte todo.
		resuelta, err = a.Alertas.ResolverAlerta(ctx, id, CierreDeAlerta{
			ActorID: actorID, ActorRol: rol, Nota: nota,
			Accion: correccion.Accion, AccionObjetivo: correccion.Objetivo, Cuando: ahora,
		})
		if err != nil {
			return err
		}

		corregida, err := a.corregir(ctx, alerta, correccion, actorID, rol, nota, ahora)
		if err != nil {
			return err
		}

		payload, err := json.Marshal(struct {
			Tipo           string `json:"tipo"`
			Periodo        string `json:"periodo"`
			RefTipo        string `json:"ref_tipo"`
			RefID          string `json:"ref_id"`
			RefTitular     string `json:"ref_titular,omitempty"`
			Detalle        string `json:"detalle"`
			Nota           string `json:"nota"`
			ActorRol       string `json:"actor_rol,omitempty"`
			Accion         string `json:"accion,omitempty"`
			AccionObjetivo string `json:"accion_objetivo,omitempty"`
		}{
			Tipo: resuelta.Tipo, Periodo: resuelta.Periodo,
			RefTipo: resuelta.RefTipo, RefID: resuelta.RefID, RefTitular: resuelta.RefTitular,
			Detalle: resuelta.Detalle, Nota: nota, ActorRol: rol,
			Accion: correccion.Accion, AccionObjetivo: correccion.Objetivo,
		})
		if err != nil {
			return fmt.Errorf("serializar el asiento de la alerta %q: %w", id, err)
		}
		if err := a.Bitacora.Asentar(ctx, Asiento{
			Hecho:   HechoAlertaResuelta,
			RefTipo: RefAlerta,
			RefID:   resuelta.ID,
			ActorID: actorID,
			Payload: payload,
			Cuando:  ahora,
		}); err != nil {
			return fmt.Errorf("asentar la resolucion de la alerta %q: %w", id, err)
		}
		if corregida != nil {
			corregida.ActorID, corregida.Cuando = actorID, ahora
			if err := a.Bitacora.Asentar(ctx, *corregida); err != nil {
				return fmt.Errorf("asentar la correccion de la alerta %q: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return Alerta{}, err
	}
	resuelta.Critica = anomalias.EsCritica(resuelta.Tipo)
	return resuelta, nil
}

// EstadoCompuerta es lo que la compuerta de #34 lee de un periodo tras evaluarlo.
//
// Abiertas bloquea. AceptadasTalCual NO bloquea -- una persona firmo que el
// duplicado es un falso positivo -- pero se cuenta aparte y viaja al asiento
// de la transicion: la corrida tiene que poder decir con cuantas criticas
// aceptadas sin corregir se calculo (#164).
type EstadoCompuerta struct {
	Abiertas         int
	AceptadasTalCual int
}

// Bloqueantes implementa [CompuertaAnomalias]: evalua el periodo AHORA y luego cuenta las criticas abiertas y las aceptadas.
func (a Anomalias) Bloqueantes(ctx context.Context, periodo string) (EstadoCompuerta, error) {
	resumen, err := a.Evaluar(ctx, periodo, actorSistema)
	if err != nil {
		return EstadoCompuerta{}, fmt.Errorf("compuerta de anomalias de %q: %w", periodo, err)
	}
	return EstadoCompuerta{Abiertas: resumen.CriticasAbiertas, AceptadasTalCual: resumen.CriticasAceptadas}, nil
}

// CriticasAbiertas cuenta las alertas guardadas sin resolver de tipo critico ([anomalias.EsCritica]); no evalua.
func (a Anomalias) CriticasAbiertas(ctx context.Context, periodo string) (int, error) {
	periodo, err := recaudo.ValidarPeriodo(periodo)
	if err != nil {
		return 0, err
	}
	criticos := make([]string, 0, len(anomalias.Tipos()))
	for _, t := range anomalias.Tipos() {
		if anomalias.EsCritica(t) {
			criticos = append(criticos, t)
		}
	}
	n, err := a.Alertas.ContarAlertasSinResolver(ctx, periodo, criticos)
	if err != nil {
		return 0, fmt.Errorf("contar alertas criticas abiertas de %q: %w", periodo, err)
	}
	return n, nil
}

// ConteoDeTipo es lo abierto de un tipo y si ese tipo bloquea la corrida.
type ConteoDeTipo struct {
	Abiertas int
	Critica  bool
}

// ResumenAlertas es la foto de lectura de un periodo para el tablero y la compuerta:
// cuenta en la base (no sobre una pagina) y no evalua.
type ResumenAlertas struct {
	Periodo           string
	Abiertas          int
	CriticasAbiertas  int
	CriticasAceptadas int
	PorTipo           map[string]ConteoDeTipo
	// UltimaEvaluacion es nil si nadie evaluo el periodo: "sin evaluar" no es "limpio".
	UltimaEvaluacion *time.Time
}

// Resumen cuenta lo abierto del periodo por tipo y dice cuando se evaluo por ultima vez; no evalua ni escribe.
func (a Anomalias) Resumen(ctx context.Context, periodo string) (ResumenAlertas, error) {
	periodo, err := recaudo.ValidarPeriodo(periodo)
	if err != nil {
		return ResumenAlertas{}, err
	}
	if a.Alertas == nil || a.Bitacora == nil {
		return ResumenAlertas{}, errors.New("anomalias mal cableadas: faltan Alertas o Bitacora")
	}
	r := ResumenAlertas{Periodo: periodo, PorTipo: make(map[string]ConteoDeTipo, len(anomalias.Tipos()))}
	for _, t := range anomalias.Tipos() {
		n, err := a.Alertas.ContarAlertasSinResolver(ctx, periodo, []string{t})
		if err != nil {
			return ResumenAlertas{}, fmt.Errorf("contar alertas abiertas de %q tipo %q: %w", periodo, t, err)
		}
		critica := anomalias.EsCritica(t)
		r.PorTipo[t] = ConteoDeTipo{Abiertas: n, Critica: critica}
		r.Abiertas += n
		if critica {
			r.CriticasAbiertas += n
		}
	}
	r.CriticasAceptadas, err = a.Alertas.ContarAlertasConAccion(ctx, periodo, anomalias.AccionAceptarTalCual)
	if err != nil {
		return ResumenAlertas{}, fmt.Errorf("contar criticas aceptadas de %q: %w", periodo, err)
	}
	asientos, err := a.Bitacora.De(ctx, RefPeriodo, periodo)
	if err != nil {
		return ResumenAlertas{}, fmt.Errorf("leer la bitacora de %q: %w", periodo, err)
	}
	for _, as := range asientos {
		if as.Hecho == HechoAnomaliasEvaluadas && (r.UltimaEvaluacion == nil || as.Cuando.After(*r.UltimaEvaluacion)) {
			c := as.Cuando
			r.UltimaEvaluacion = &c
		}
	}
	return r, nil
}

// conteoVacioPorTipo devuelve los seis tipos en cero. Un cero explicito y no
// una clave ausente: el tablero pinta una tarjeta por tipo, y una clave que
// falta se distingue mal de un cero cuando el JSON llega al otro lado.
func conteoVacioPorTipo() map[string]int {
	out := make(map[string]int, len(anomalias.Tipos()))
	for _, t := range anomalias.Tipos() {
		out[t] = 0
	}
	return out
}

// cableadoParaResolver es [Anomalias.cableado] mas lo que solo usa el cierre:
// la foto del periodo para validar una exclusion y el puerto que corrige el dato.
func (a Anomalias) cableadoParaResolver() error {
	if err := a.cableado(); err != nil {
		return err
	}
	switch {
	case a.Entregas == nil:
		return errors.New("anomalias mal cableadas: falta LecturaDeEntregas")
	case a.Correcciones == nil:
		return errors.New("anomalias mal cableadas: falta CorreccionDeDatos")
	}
	return nil
}

// cableado comprueba las dependencias de los caminos de ESCRITURA antes de
// abrir una transaccion, igual que [Catalogo.enUnidad] y por lo mismo: un
// servicio a medias paniqueaba DENTRO de la unidad ya abierta y con un mensaje
// que apuntaba a la dependencia equivocada.
func (a Anomalias) cableado() error {
	switch {
	case a.Alertas == nil:
		return errors.New("anomalias mal cableadas: falta RepositorioAlertas")
	case a.Unidad == nil:
		return errors.New("anomalias mal cableadas: falta UnidadDeTrabajo")
	case a.Bitacora == nil:
		return errors.New("anomalias mal cableadas: falta BitacoraAuditoria")
	case a.Reloj == nil:
		return errors.New("anomalias mal cableadas: falta Reloj")
	}
	return nil
}
