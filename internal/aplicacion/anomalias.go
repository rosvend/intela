package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

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
)

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
// No bloquea nada por si sola tampoco: [Anomalias.CriticasAbiertas] expone el
// predicado, y quien lo consume es la compuerta de #34 -- que hoy no existe:
// `/admin/pipeline` es un stub y `cmd/worker` registra el trabajo de reparto
// como pendiente. Inventarle aqui el cableado seria cablear contra una firma
// que todavia no esta escrita.
type Anomalias struct {
	Entregas LecturaDeEntregas
	// Declaraciones es el MISMO puerto estrecho que usa el catalogo: la
	// version abierta de un conjunto de obras, sin poder escribir ninguna.
	Declaraciones LectorDeDeclaraciones
	Coautores     LectorDeCoautores
	Alertas       RepositorioAlertas
	Bitacora      BitacoraAuditoria
	Unidad        UnidadDeTrabajo
	Reloj         Reloj
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
	// bloquea la distribucion. Es el predicado que consumira #34.
	CriticasAbiertas int

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

	armado, err := a.armarPeriodo(ctx, periodo)
	if err != nil {
		return ResumenEvaluacion{}, err
	}

	hallazgos := anomalias.Detectar(armado)
	ahora := a.Reloj.Ahora()
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

	resumen := ResumenEvaluacion{
		Periodo:        periodo,
		Detectadas:     len(hallazgos),
		PorTipo:        porTipo,
		UsosSinCotejar: anomalias.SinClaveDeRegistro(armado.Usos),
	}

	err = a.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		nuevas, err := a.Alertas.GuardarAlertas(ctx, alertas)
		if err != nil {
			return fmt.Errorf("guardar las alertas de %q: %w", periodo, err)
		}
		resumen.Nuevas = nuevas

		payload, err := json.Marshal(struct {
			Periodo        string         `json:"periodo"`
			Detectadas     int            `json:"detectadas"`
			Nuevas         int            `json:"nuevas"`
			PorTipo        map[string]int `json:"por_tipo"`
			Usos           int            `json:"usos_evaluados"`
			Obras          int            `json:"obras_evaluadas"`
			Entregas       int            `json:"entregas_cotejadas"`
			UsosSinCotejar int            `json:"usos_sin_cotejar"`
		}{
			Periodo: periodo, Detectadas: resumen.Detectadas, Nuevas: nuevas, PorTipo: porTipo,
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
		return nil
	})
	if err != nil {
		return ResumenEvaluacion{}, err
	}

	resumen.CriticasAbiertas, err = a.CriticasAbiertas(ctx, periodo)
	if err != nil {
		return ResumenEvaluacion{}, err
	}
	return resumen, nil
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
		armado.Usos = append(armado.Usos, anomalias.Uso{
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
		})
	}
	for _, c := range cargas {
		armado.Entregas = append(armado.Entregas, anomalias.Entrega{
			ID:      c.ID,
			Fuente:  c.Fuente,
			Periodo: c.Periodo,
			SHA256:  c.SHA256,
		})
	}

	armado.Obras, err = a.obrasDelPeriodo(ctx, armado.Usos)
	if err != nil {
		return anomalias.Periodo{}, err
	}
	return armado, nil
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
		// no llega a la tabla -- la clave natural (periodo, tipo, ref_tipo,
		// ref_id, ref_titular) lo absorbe con ON CONFLICT DO NOTHING -- pero SI
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

// Resolver cierra una alerta a nombre de quien la cierra.
//
// # El actor se exige ANTES de escribir nada
//
// Es una decision humana sobre una anomalia que afecta a quien cobra, que es
// exactamente el caso en el que el ADR 0006 pide saber quien la tomo. Un
// actorID vacio dejaria un asiento sin firmar -- valido para la base, invalido
// para el ADR --, y el UPDATE ya estaria hecho cuando eso se notara.
//
// # El UPDATE y su asiento van en la misma unidad
//
// Por lo mismo que en [Catalogo.RegistrarObra]: una alerta cerrada cuyo
// asiento fallo no esta cerrada. Y el asiento guarda el estado ANTERIOR ademas
// del nuevo, porque `alertas` se sobreescribe: si no queda en el payload, no
// queda en ningun sitio.
//
// Resolver NO toca el registro ofensor. Asignar la obra de un ONI o descartar
// una fila es #39, con su propio caso de uso y su propio asiento; esto solo
// dice que alguien se hizo cargo.
func (a Anomalias) Resolver(ctx context.Context, id, actorID, nota string) (Alerta, error) {
	id = strings.TrimSpace(id)
	if err := exigirActor(actorID, fmt.Sprintf("resolver la alerta %q", id)); err != nil {
		return Alerta{}, err
	}
	if id == "" {
		return Alerta{}, fmt.Errorf("resolver una alerta: %w", ErrNoEncontrado)
	}
	if err := a.cableado(); err != nil {
		return Alerta{}, err
	}

	nota = strings.TrimSpace(nota)
	ahora := a.Reloj.Ahora()

	var resuelta Alerta
	err := a.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		var err error
		resuelta, err = a.Alertas.ResolverAlerta(ctx, id, actorID, nota, ahora)
		if err != nil {
			// Sin envolver: quien llama distingue ErrNoEncontrado y
			// ErrAlertaYaResuelta con errors.Is, y el adaptador ya puso su
			// contexto.
			return err
		}
		payload, err := json.Marshal(struct {
			Tipo       string `json:"tipo"`
			Periodo    string `json:"periodo"`
			RefTipo    string `json:"ref_tipo"`
			RefID      string `json:"ref_id"`
			RefTitular string `json:"ref_titular,omitempty"`
			Detalle    string `json:"detalle"`
			Nota       string `json:"nota,omitempty"`
		}{
			Tipo: resuelta.Tipo, Periodo: resuelta.Periodo,
			RefTipo: resuelta.RefTipo, RefID: resuelta.RefID, RefTitular: resuelta.RefTitular,
			Detalle: resuelta.Detalle, Nota: nota,
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
		return nil
	})
	if err != nil {
		return Alerta{}, err
	}
	resuelta.Critica = anomalias.EsCritica(resuelta.Tipo)
	return resuelta, nil
}

// CriticasAbiertas cuenta las alertas sin resolver de un periodo cuyo tipo
// BLOQUEA la distribucion.
//
// Es el predicado de la compuerta de OE-5 -- "resolver antes del reparto" -- y
// esta expuesto aqui, suelto, a proposito: quien lo tiene que consumir es el
// proceso de reparto de #33/#34, que hoy no existe (`/admin/pipeline` es un
// stub y `cmd/worker` registra TrabajoEjecutarReparto como pendiente).
// Cablearlo ahora seria cablearlo contra una firma que nadie ha escrito; lo
// que si se puede dejar hecho es la pregunta, con su criterio en el dominio.
//
// Que tipos bloquean lo decide [anomalias.EsCritica] y no este metodo: la
// lista viaja al adaptador como parametro para que no haya un segundo
// criterio en el SQL.
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
