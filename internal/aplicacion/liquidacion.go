package aplicacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/liquidacion"
	"github.com/rosvend/intela/internal/dominio/reparto"
)

const claveSMMLV = "smmlv"

// RefOrdenDePago es el tipo de referencia de los asientos de liquidacion: es
// la mitad con la que [BitacoraAuditoria.De] recupera la historia de una orden
// -- emision, silencio, arrastre -- en el orden en que ocurrio.
const RefOrdenDePago = "orden_pago"

// RefLiquidacionLote es la referencia del asiento de lote: el residuo de
// prorrateo es del (periodo, circuito), no de cada orden. Un asiento por
// orden con el mismo residuo haria que quien sume el libro cuente N veces
// el mismo centavo (ADR 0005).
const RefLiquidacionLote = "liquidacion_lote"

// Hechos que la liquidacion asienta en la bitacora (ADR 0006).
//
// Constantes y no literales en la llamada por lo mismo que en el catalogo: el
// hecho es la clave por la que se consulta el libro, y una errata en el
// literal no rompe nada hoy, deja un asiento que ninguna consulta encuentra
// dentro de diez anos.
const (
	// HechoLiquidacionEmitida es la orden de pago recien enviada al titular.
	// Su payload lleva el acuse de la notificacion: es lo que prueba desde
	// cuando corre el plazo de R-10.
	HechoLiquidacionEmitida = "liquidacion.emitida"
	// HechoLiquidacionResiduoProrrateo asienta UNA vez por lote el centavaje
	// de [liquidacion.Prorratear] que no quedo en ninguna orden. RefID =
	// periodo-circuito.
	HechoLiquidacionResiduoProrrateo = "liquidacion.residuo_prorrateo"
	// HechoLiquidacionAceptadaPorSilencio es R-10 consumado: 15 dias
	// calendario sin respuesta con el neto sobre el umbral.
	HechoLiquidacionAceptadaPorSilencio = "liquidacion.aceptada_por_silencio"
	// HechoLiquidacionDiferida es R-11: 15 dias sin respuesta y el neto no
	// llega al 2% de un SMMLV, asi que el monto espera al periodo siguiente.
	HechoLiquidacionDiferida = "liquidacion.diferida"
	// HechoLiquidacionAcumulada cierra una diferida cuyo neto ya se incorporo
	// a una orden posterior, para que no se arrastre dos veces.
	HechoLiquidacionAcumulada = "liquidacion.acumulada"
)

// actorSistema es el actor de los asientos que NO nacen de una decision de una
// persona.
//
// Generar una liquidacion y las transiciones de R-10 y R-11 las produce el
// sistema: el silencio del titular no es una accion que nadie firme, y el
// plazo lo consuma el calendario. El ADR 0006 pide el actor "en ese ultimo
// caso", el de la decision manual, y `asientos.actor_id` es nullable
// precisamente para esto (el adaptador lo escribe con NULLIF sobre la cadena
// vacia).
//
// Tiene que ser la cadena VACIA y no un "sistema" literal: `asientos.actor_id`
// referencia `usuarios(id)`, asi que un id inventado no pasa la clave foranea.
// Por eso estos asientos no pasan por [exigirActor], que es la guarda de los
// casos de uso que SI reciben un actor de la sesion.
const actorSistema = ""

// OrdenVista es la orden mas lo que solo se sabe en esta capa: si se
// puede pagar (R-12) y el dia civil con el que se evaluo el plazo.
type OrdenVista struct {
	Orden   liquidacion.OrdenDePago
	Pagable bool
}

// Liquidaciones genera ordenes de pago a partir de las lineas de titular
// de una corrida, y las sirve al admin y al titular.
//
// El reloj entra aqui, no en dominio: EvaluarPlazo recibe un YYYY-MM-DD.
//
// # Por que sostiene cinco puertos y no uno
//
// Igual que [Catalogo], y por la misma razon del ADR 0003: la trazabilidad
// entra por [BitacoraAuditoria] y no por el contrato del repositorio, asi que
// el limite de transaccion hay que declararlo, y eso es [UnidadDeTrabajo].
// Emitir una orden son cuatro escrituras que son UN hecho -- la orden, el
// cierre de las diferidas que absorbe, el asiento de cada una y la
// notificacion que arranca el plazo -- y ninguna de ellas tiene sentido sin
// las otras.
//
// [Notificador] esta aqui y no en el adaptador porque R-10 cuenta 15 dias
// "desde el envio": sin un envio que haya ocurrido de verdad, `EnviadaDia` es
// una fecha que nadie puede oponer al titular.
type Liquidaciones struct {
	Ordenes     RepositorioLiquidacion
	Reloj       Reloj
	Notificador Notificador
	Bitacora    BitacoraAuditoria
	Unidad      UnidadDeTrabajo
}

// GenerarLiquidacion emite las ordenes de pago de un periodo y circuito.
//
// # Quien la invoca en produccion
//
// Este PR (#36) NO expone un POST HTTP: el contrato de la issue son las
// lecturas (`GET /liquidaciones`, `GET /mis-liquidaciones`). El motor queda
// cableado en `cmd/api` y `cmd/lambda` listo para que el orquestador de
// corridas (#34 / ProcesoDeReparto) lo dispare al cerrar la compuerta del
// RD 13.5. Sin ese disparador, `ordenes_pago` solo se poblaria desde tests o
// una llamada directa al caso de uso — es deliberado, no un olvido.
//
// # Precondicion del disparador (#34 / #159)
//
// Hoy `cmd/api` y `cmd/lambda` cablean `notificaciones.Bitacora`, que solo
// registra en el log y inventa un acuse. El PR que conecte este caso de uso
// tiene que traer un adaptador que entregue de verdad (correo, SMS, portal)
// o bloquear la emision: si no, el plazo de R-10 corre sin que el titular
// haya recibido nada.
//
// # Una orden por (titular, periodo, circuito), no por corrida
//
// Es el ADR 0019 (ver tambien la nota de [liquidacion.OrdenDePago]): un
// periodo se cierra con tantas corridas como bolsas tenga el circuito, y lo
// que R-11 mide contra el 2% de un SMMLV es lo que el titular cobra POR EL
// PERIODO. Emitir una orden por corrida diferiria como menor cuantia saldos
// que juntos si pasan el umbral, y le mandaria al titular tres avisos con tres
// plazos distintos por el mismo periodo.
//
// Asi que procesoID es el DISPARADOR, no el alcance: de el se sacan periodo y
// circuito, y se agregan las lineas de TODAS las corridas de ese periodo y
// circuito que ya pasaron la compuerta del RD 13.5.
//
// # Decision abierta: las corridas que cierran tarde
//
// Una corrida del mismo periodo y circuito que pase la compuerta DESPUES de
// que las ordenes ya se emitieron NO se incorpora a ellas, y hoy no emite
// ordenes propias: la guarda de idempotencia ve que ya hay ordenes para ese
// (periodo, circuito) y las devuelve tal cual. Es deliberado y es lo
// conservador: doblar el bruto de una orden ya enviada reabriria un plazo de
// R-10 que puede estar corriendo o ya vencido, y emitir una segunda orden del
// mismo periodo choca con el UNIQUE (titular_id, periodo, circuito) que
// sostiene todo lo de arriba.
//
// Lo que queda sin resolver es como se paga ese dinero, y no se resuelve aqui
// porque la respuesta es normativa y no tecnica: puede ser una corrida de
// ajuste del periodo siguiente (el camino de R-11, que ya existe) o una
// reapertura del periodo, y eso lo decide el Consejo Directivo. Mientras no
// este decidido, el orden de operaciones es el control: no se liquida un
// periodo hasta que sus corridas estan firmadas.
//
// # Idempotente
//
// Volver a llamarla sobre el mismo periodo y circuito NO regenera: devuelve lo
// que hay, con el plazo reevaluado. Es lo que hace que un reintento -- de la
// cola, de un operador, de dos peticiones a la vez -- no duplique dinero, y lo
// que impide el defecto peor de todos: que una orden ya diferida se incorpore
// a si misma como arrastre y aparezca acumulada de su propio neto.
func (l Liquidaciones) GenerarLiquidacion(ctx context.Context, procesoID string) ([]OrdenVista, error) {
	if err := l.cableadoParaEmitir(); err != nil {
		return nil, err
	}

	meta, err := l.Ordenes.MetaDeProceso(ctx, procesoID)
	if err != nil {
		return nil, fmt.Errorf("meta del proceso %s: %w", procesoID, err)
	}
	if err := meta.ExigirListoParaLiquidar(); err != nil {
		return nil, err
	}

	ahora := l.Reloj.Ahora().UTC()

	// El SMMLV se resuelve ANTES de tocar nada. No se usa para emitir -- el
	// umbral decide el silencio, no la emision -- pero sin el no se puede
	// servir lo emitido, y fallar despues de haber insertado y notificado
	// dejaria al titular con un aviso de una orden que la respuesta no puede
	// mostrar (ADR 0004: se falla, no se inventa un valor).
	if _, err := l.umbralVigente(ctx, ahora); err != nil {
		return nil, err
	}

	var emitidas []liquidacion.OrdenDePago
	err = l.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		// Lo PRIMERO, y dentro de la unidad: todo lo que sigue es un
		// leer-modificar-escribir sobre el mismo (periodo, circuito), y hasta
		// que la primera orden exista no hay ninguna fila que sirva de cerrojo.
		if err := l.Ordenes.BloquearPeriodo(ctx, meta.Periodo, meta.Circuito); err != nil {
			return fmt.Errorf("bloquear %s: %w", meta.donde(), err)
		}

		existentes, err := l.Ordenes.DePeriodoCircuito(ctx, meta.Periodo, meta.Circuito)
		if err != nil {
			return fmt.Errorf("ordenes de %s: %w", meta.donde(), err)
		}
		if len(existentes) > 0 {
			emitidas = existentes
			return nil
		}

		procesos, err := l.Ordenes.ProcesosListos(ctx, meta.Periodo, meta.Circuito)
		if err != nil {
			return fmt.Errorf("corridas listas de %s: %w", meta.donde(), err)
		}
		// El disparador acaba de pasar la compuerta, asi que tiene que estar en
		// la lista. Si no esta, el adaptador y el gate no estan mirando lo
		// mismo, y emitir con un alcance que no incluye la corrida que se pidio
		// liquidar seria peor que no emitir.
		if !slices.Contains(procesos, procesoID) {
			return fmt.Errorf(
				"%w: %s paso la compuerta pero no aparece entre las corridas listas de %s",
				ErrCorridaNoCuadra, procesoID, meta.donde())
		}

		ag, err := l.agregar(ctx, procesos)
		if err != nil {
			return err
		}
		if err := l.emitir(ctx, meta, ag, ahora); err != nil {
			return err
		}

		// Se relee en vez de devolver lo que se construyo: EmitirOrdenes no
		// pisa lo que ya existiera bajo el mismo id, asi que lo unico que de
		// verdad describe el estado del periodo es la base.
		emitidas, err = l.Ordenes.DePeriodoCircuito(ctx, meta.Periodo, meta.Circuito)
		return err
	})
	if err != nil {
		return nil, err
	}
	return l.conPlazoYDocumentos(ctx, emitidas)
}

// agregado es el insumo de TODAS las corridas listas de un periodo y circuito,
// sumado. Es lo unico que ve el prorrateo: una orden agregada no tiene una
// corrida a la que pertenecer.
type agregado struct {
	// Procesos son las corridas que contribuyeron, ordenadas. La primera es la
	// de referencia.
	Procesos []string

	Bruto   decimal.Decimal
	Admin   decimal.Decimal
	Social  decimal.Decimal
	Reserva decimal.Decimal

	// PorTitular es el neto de cada titular, ya sumado sobre obras y corridas.
	PorTitular map[string]decimal.Decimal

	// Distribuido es la suma de las lineas. Puede ser MENOR que el neto de la
	// corrida -- eso es el retenido -- pero nunca mayor; ver
	// [ErrCorridaNoCuadra].
	Distribuido decimal.Decimal
}

// Neto es el denominador del prorrateo: lo que las corridas dejaron para
// repartir. Ver [liquidacion.Prorratear] para por que no es Distribuido.
func (a agregado) Neto() decimal.Decimal {
	return liquidacion.NetoDeCorrida(a.Bruto, a.Admin, a.Social, a.Reserva)
}

func (l Liquidaciones) agregar(ctx context.Context, procesos []string) (agregado, error) {
	ag := agregado{
		Procesos:    procesos,
		Bruto:       decimal.Zero,
		Admin:       decimal.Zero,
		Social:      decimal.Zero,
		Reserva:     decimal.Zero,
		PorTitular:  map[string]decimal.Decimal{},
		Distribuido: decimal.Zero,
	}
	for _, procesoID := range procesos {
		insumo, err := l.Ordenes.InsumoDeProceso(ctx, procesoID)
		if err != nil {
			return agregado{}, fmt.Errorf("insumo del proceso %s: %w", procesoID, err)
		}
		ag.Bruto = ag.Bruto.Add(insumo.Bruto)
		ag.Admin = ag.Admin.Add(insumo.Admin)
		ag.Social = ag.Social.Add(insumo.Social)
		ag.Reserva = ag.Reserva.Add(insumo.Reserva)
		for _, linea := range insumo.Titulares {
			ag.PorTitular[linea.TitularID] = ag.PorTitular[linea.TitularID].Add(linea.Importe)
			ag.Distribuido = ag.Distribuido.Add(linea.Importe)
		}
	}

	neto := ag.Neto()
	if neto.IsNegative() {
		return agregado{}, fmt.Errorf(
			"%w: las corridas %v deducen %s de un bruto de %s",
			ErrCorridaNoCuadra, procesos,
			ag.Admin.Add(ag.Social).Add(ag.Reserva), ag.Bruto)
	}
	if ag.Distribuido.GreaterThan(neto) {
		return agregado{}, fmt.Errorf(
			"%w: las corridas %v reparten %s sobre un neto de %s",
			ErrCorridaNoCuadra, procesos, ag.Distribuido, neto)
	}
	return ag, nil
}

// emitir arma y persiste una orden por titular, cierra las diferidas que
// absorbe y asienta las dos cosas. Corre DENTRO de la unidad de
// [Liquidaciones.GenerarLiquidacion] y con su cerrojo ya tomado.
func (l Liquidaciones) emitir(
	ctx context.Context, meta MetaProceso, ag agregado, ahora time.Time,
) error {
	netoProc := ag.Neto()
	enviadaDia := diaCivil(ahora)

	titulares := make([]string, 0, len(ag.PorTitular))
	for titularID := range ag.PorTitular {
		titulares = append(titulares, titularID)
	}
	slices.Sort(titulares)

	deduccionesPorTitular, residuoProrrateo := liquidacion.Prorratear(
		ag.PorTitular, ag.Admin, ag.Social, ag.Reserva, netoProc,
	)

	nuevas := make([]liquidacion.OrdenDePago, 0, len(titulares))
	acuses := make(map[string]string, len(titulares))
	acumuladas := make([]liquidacion.OrdenDePago, 0)

	for _, titularID := range titulares {
		neto := ag.PorTitular[titularID]
		deducciones := deduccionesPorTitular[titularID]
		bruto := neto
		for _, d := range deducciones {
			bruto = bruto.Add(d.Monto)
		}

		id := idOrden(meta.Periodo, meta.Circuito, titularID)

		o, err := liquidacion.NuevaOrden(liquidacion.DatosOrden{
			ID:          id,
			ProcesoID:   ag.Procesos[0],
			Procesos:    ag.Procesos,
			TitularID:   titularID,
			Periodo:     meta.Periodo,
			Circuito:    string(meta.Circuito),
			EnviadaDia:  enviadaDia,
			Bruto:       bruto,
			Deducciones: deducciones,
		})
		if err != nil {
			return fmt.Errorf("armar la orden de %s: %w", titularID, err)
		}

		// Con la fila bloqueada: ver [RepositorioLiquidacion.DiferidasDeTitular].
		// Solo mismo circuito y periodos anteriores (R-11, ADR 0019).
		diferidas, err := l.Ordenes.DiferidasDeTitular(ctx, titularID, meta.Circuito, meta.Periodo)
		if err != nil {
			return fmt.Errorf("diferidas de %s: %w", titularID, err)
		}
		for _, prev := range diferidas {
			// Una orden no se arrastra a si misma. No puede pasar con la guarda
			// de idempotencia por delante -- si existiera ya, no estariamos
			// emitiendo -- y se comprueba igual porque el id es estable y el
			// coste de equivocarse es una orden acumulada de su propio neto.
			if prev.ID == o.ID {
				continue
			}
			o = o.IncorporarArrastre(prev)
			acumuladas = append(acumuladas, prev.MarcarAcumulada())
		}

		// Se notifica DESPUES de armar la orden completa (arrastres incluidos)
		// y el acuse queda en el asiento. R-10 cuenta 15 dias "desde el envio"
		// sobre la liquidacion que se le mostro al titular (RD 13.2): bruto y
		// neto del aviso tienen que coincidir con la orden persistida.
		//
		// Que la notificacion sea un efecto que no se revierte -- si la unidad
		// falla despues, el aviso ya salio -- es el mismo reparto que la boveda
		// de la ingesta: de un envio no se hace rollback. Notificar despues de
		// NuevaOrden / DiferidasDeTitular reduce avisos huerfanos si esas
		// lecturas fallan. El contrario, una orden sin aviso, es el que corre
		// un plazo contra alguien que no sabe nada.
		acuse, err := l.Notificador.Notificar(ctx, titularID,
			asuntoLiquidacion(meta), cuerpoLiquidacion(meta, o, enviadaDia))
		if err != nil {
			return fmt.Errorf("notificar la liquidacion %s: %w", id, err)
		}
		acuses[id] = acuse

		nuevas = append(nuevas, o)
	}

	if err := l.Ordenes.EmitirOrdenes(ctx, nuevas); err != nil {
		return fmt.Errorf("emitir las ordenes de %s: %w", meta.donde(), err)
	}
	for _, o := range nuevas {
		if err := l.asentar(ctx, HechoLiquidacionEmitida, o, "", acuses[o.ID]); err != nil {
			return err
		}
	}
	// Residuo del lote UNA sola vez (ref periodo+circuito), aunque no haya
	// titulares con neto: si todo quedo retenido, el centavaje no puede
	// desaparecer sin rastro (ADR 0005).
	if err := l.asentarResiduoLote(ctx, meta, residuoProrrateo); err != nil {
		return err
	}

	if len(acumuladas) == 0 {
		return nil
	}
	aplicadas, err := l.Ordenes.TransicionarOrdenes(ctx, acumuladas, liquidacion.EstadoDiferida)
	if err != nil {
		return fmt.Errorf("cerrar las diferidas arrastradas a %s: %w", meta.donde(), err)
	}
	for _, o := range aplicadas {
		if err := l.asentar(ctx, HechoLiquidacionAcumulada, o, liquidacion.EstadoDiferida, ""); err != nil {
			return err
		}
	}
	return nil
}

// Listar es el listado de administracion. Un titular no lo ve: el suyo
// sale por DeTitular.
func (l Liquidaciones) Listar(ctx context.Context, actor Usuario) ([]OrdenVista, error) {
	if !esStaff(actor.Rol) {
		return nil, ErrNoAutorizado
	}
	ordenes, err := l.Ordenes.Listar(ctx)
	if err != nil {
		return nil, fmt.Errorf("listar liquidaciones: %w", err)
	}
	return l.conPlazoYDocumentos(ctx, ordenes)
}

// DeTitular es GET /mis-liquidaciones. El titular solo ve las suyas; el
// id sale de la sesion, no de la URL, para que no se consulten las de otro.
func (l Liquidaciones) DeTitular(ctx context.Context, actor Usuario) ([]OrdenVista, error) {
	if actor.Rol != RolTitular || actor.TitularID == "" {
		return nil, ErrNoAutorizado
	}
	ordenes, err := l.Ordenes.DeTitular(ctx, actor.TitularID)
	if err != nil {
		return nil, fmt.Errorf("liquidaciones de %s: %w", actor.TitularID, err)
	}
	return l.conPlazoYDocumentos(ctx, ordenes)
}

// conPlazoYDocumentos evalua R-10 y R-11 contra el dia de hoy y persiste las
// transiciones que salgan, cada una con su asiento y en la misma transaccion
// (ADR 0006).
//
// La transicion es CONDICIONAL -- solo se aplica si la orden sigue en
// `enviada` -- y lo que se devuelve refleja lo que de verdad quedo escrito, no
// lo que este proceso calculo: dos lecturas concurrentes del dia 15 evaluan lo
// mismo y solo una escribe, y la que llega tarde no puede informar un estado
// que ella no consiguio poner.
func (l Liquidaciones) conPlazoYDocumentos(ctx context.Context, ordenes []liquidacion.OrdenDePago) ([]OrdenVista, error) {
	if len(ordenes) == 0 {
		return []OrdenVista{}, nil
	}
	ahora := l.Reloj.Ahora().UTC()
	umbral, err := l.umbralVigente(ctx, ahora)
	if err != nil {
		return nil, err
	}
	hoy := diaCivil(ahora)

	cambiadas := make([]liquidacion.OrdenDePago, 0)
	for _, o := range ordenes {
		if nueva := o.EvaluarPlazo(hoy, umbral); nueva.Estado != o.Estado {
			cambiadas = append(cambiadas, nueva)
		}
	}
	if len(cambiadas) == 0 {
		return l.conDocumentos(ctx, ordenes)
	}

	if err := l.cableadoParaEscribir(); err != nil {
		return nil, err
	}
	var aplicadas []liquidacion.OrdenDePago
	err = l.Unidad.EnUnidad(ctx, func(ctx context.Context) error {
		var err error
		aplicadas, err = l.Ordenes.TransicionarOrdenes(ctx, cambiadas, liquidacion.EstadoEnviada)
		if err != nil {
			return fmt.Errorf("persistir transicion de silencio: %w", err)
		}
		for _, o := range aplicadas {
			if err := l.asentar(ctx, hechoDeSilencio(o.Estado), o, liquidacion.EstadoEnviada, ""); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	nuevoEstado := make(map[string]liquidacion.Estado, len(aplicadas))
	for _, o := range aplicadas {
		nuevoEstado[o.ID] = o.Estado
	}
	for i := range ordenes {
		if estado, cambio := nuevoEstado[ordenes[i].ID]; cambio {
			ordenes[i].Estado = estado
		}
	}
	return l.conDocumentos(ctx, ordenes)
}

func (l Liquidaciones) conDocumentos(ctx context.Context, ordenes []liquidacion.OrdenDePago) ([]OrdenVista, error) {
	docs, err := l.Ordenes.Documentos(ctx)
	if err != nil {
		return nil, fmt.Errorf("documentos de titulares: %w", err)
	}
	if docs == nil {
		docs = map[string]liquidacion.Documentos{}
	}
	vistas := make([]OrdenVista, 0, len(ordenes))
	for _, o := range ordenes {
		vistas = append(vistas, OrdenVista{
			Orden:   o,
			Pagable: o.EsPagable(docs[o.TitularID]),
		})
	}
	return vistas, nil
}

// umbralVigente resuelve el 2% de un SMMLV en una fecha. Un SMMLV ausente o no
// positivo es [ErrParametroAusente] y no un umbral de cero: con umbral cero
// TODA orden pasaria de enviada a aceptada_por_silencio a los 15 dias, y R-11
// dejaria de existir sin que nada lo dijera (ADR 0004).
func (l Liquidaciones) umbralVigente(ctx context.Context, ahora time.Time) (decimal.Decimal, error) {
	smmlv, err := l.Ordenes.SMMLVVigente(ctx, ahora)
	if err != nil {
		return decimal.Zero, fmt.Errorf("smmlv: %w", err)
	}
	if smmlv.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("%w: %s", ErrParametroAusente, claveSMMLV)
	}
	return liquidacion.UmbralMenorCuantia(smmlv), nil
}

// asentar escribe el asiento de un hecho de liquidacion. El error NO se
// descarta en ningun camino: el ADR 0006 declara el asiento "parte de la
// definicion de hecho de cada caso de uso", asi que devolverlo es lo que
// revierte la unidad y con ella la orden que el asiento explicaba.
//
// Sin [exigirActor]: estos hechos los produce el sistema, no una persona. Ver
// [actorSistema].
func (l Liquidaciones) asentar(
	ctx context.Context, hecho string, o liquidacion.OrdenDePago,
	anterior liquidacion.Estado, acuse string,
) error {
	payload, err := json.Marshal(asientoDeOrden(o, anterior, acuse))
	if err != nil {
		return fmt.Errorf("serializar el asiento de la orden %q: %w", o.ID, err)
	}
	if err := l.Bitacora.Asentar(ctx, Asiento{
		Hecho:   hecho,
		RefTipo: RefOrdenDePago,
		RefID:   o.ID,
		ActorID: actorSistema,
		Payload: payload,
		Cuando:  l.Reloj.Ahora(),
	}); err != nil {
		return fmt.Errorf("asentar %q sobre la orden %q: %w", hecho, o.ID, err)
	}
	return nil
}

// asentarResiduoLote escribe UNA vez el residuo de [liquidacion.Prorratear]
// del (periodo, circuito). No va en cada asiento de orden: con N titulares
// el mismo centavo se contaria N veces.
func (l Liquidaciones) asentarResiduoLote(
	ctx context.Context, meta MetaProceso, r liquidacion.ResiduoProrrateo,
) error {
	payload, err := json.Marshal(residuoAsentadoDe(r))
	if err != nil {
		return fmt.Errorf("serializar el residuo de %s/%s: %w", meta.Periodo, meta.Circuito, err)
	}
	refID := meta.Periodo + "-" + string(meta.Circuito)
	if err := l.Bitacora.Asentar(ctx, Asiento{
		Hecho:   HechoLiquidacionResiduoProrrateo,
		RefTipo: RefLiquidacionLote,
		RefID:   refID,
		ActorID: actorSistema,
		Payload: payload,
		Cuando:  l.Reloj.Ahora(),
	}); err != nil {
		return fmt.Errorf("asentar residuo de prorrateo de %s: %w", refID, err)
	}
	return nil
}

// cableadoParaEscribir comprueba las cuatro dependencias de cualquier camino
// que escriba. Es la misma guarda que [Catalogo.enUnidad] y existe por lo
// mismo: un servicio cableado a medias tiene que fallar con un mensaje que
// nombre la dependencia que falta, y no con un nil pointer dereference dentro
// de una transaccion ya abierta.
func (l Liquidaciones) cableadoParaEscribir() error {
	switch {
	case l.Ordenes == nil:
		return errors.New("liquidaciones mal cableadas: falta RepositorioLiquidacion")
	case l.Reloj == nil:
		return errors.New("liquidaciones mal cableadas: falta Reloj")
	case l.Unidad == nil:
		return errors.New("liquidaciones mal cableadas: falta UnidadDeTrabajo")
	case l.Bitacora == nil:
		return errors.New("liquidaciones mal cableadas: falta BitacoraAuditoria")
	}
	return nil
}

// cableadoParaEmitir anade el Notificador, que solo hace falta al emitir: una
// transicion de silencio no avisa a nadie, la produce el calendario.
func (l Liquidaciones) cableadoParaEmitir() error {
	if err := l.cableadoParaEscribir(); err != nil {
		return err
	}
	if l.Notificador == nil {
		return errors.New("liquidaciones mal cableadas: falta Notificador")
	}
	return nil
}

// ExigirListoParaLiquidar es la compuerta del RD 13.5 sobre una corrida: etapa
// `liquidacion_final` y las firmas de distribucion y contabilidad SOBRE LA
// REVISION VIGENTE.
//
// Las dos condiciones se comprueban aqui y no en el adaptador porque son la
// regla, no una consulta: dejarlas en el SQL las volveria improbables sin una
// base de datos, y son justo lo que impide que una llamada a
// [Liquidaciones.GenerarLiquidacion] pague una corrida a medio verificar.
//
// El error nombra la etapa que se encontro y los roles que faltan. Quien lo
// recibe es distribucion, y "el proceso no esta listo" a secas no le dice si
// tiene que avanzar la etapa o pedir una firma.
func (m MetaProceso) ExigirListoParaLiquidar() error {
	if m.Etapa != reparto.EtapaLiquidacionFinal {
		return fmt.Errorf("%w: %s esta en etapa %q y liquidar exige %q",
			ErrProcesoNoListo, m.ID, m.Etapa, reparto.EtapaLiquidacionFinal)
	}
	faltan := m.rolesSinFirma()
	if len(faltan) > 0 {
		return fmt.Errorf("%w: a %s le faltan las firmas de %s sobre la revision %d",
			ErrProcesoNoListo, m.ID, strings.Join(faltan, " y "), m.Revision)
	}
	return nil
}

// rolesSinFirma son los roles de la compuerta que no han firmado la revision
// vigente, en orden fijo. Vacio -- y no nil -- no se distingue aqui porque
// quien llama solo mira la longitud.
func (m MetaProceso) rolesSinFirma() []string {
	firmados := make(map[string]bool, len(m.Firmas))
	for _, f := range m.Firmas {
		if f.SobreRev == m.Revision {
			firmados[f.Rol] = true
		}
	}
	faltan := make([]string, 0, 2)
	for _, rol := range []string{string(RolDistribucion), string(RolContabilidad)} {
		if !firmados[rol] {
			faltan = append(faltan, rol)
		}
	}
	return faltan
}

// donde nombra el par (periodo, circuito) para los mensajes de error: es el
// alcance real de una generacion, y decir solo el proceso disparador mandaria
// a mirar la corrida equivocada.
func (m MetaProceso) donde() string {
	return m.Periodo + "/" + string(m.Circuito)
}

func esStaff(r Rol) bool {
	switch r {
	case RolAdministrador, RolDistribucion, RolContabilidad, RolAuditor:
		return true
	default:
		return false
	}
}

// idOrden es la clave de la orden hecha identificador: (periodo, circuito,
// titular), los mismos tres campos del UNIQUE de `ordenes_pago`.
//
// NO lleva el proceso, y es lo que hace la generacion idempotente entre
// corridas: dos corridas del mismo periodo y circuito producen el MISMO id
// para el mismo titular, asi que la segunda choca con la primera en vez de
// abrir una orden paralela. Con el proceso dentro, cada corrida abriria la
// suya y el UNIQUE lo rechazaria con un error de esquema en vez de con la
// idempotencia que se busca.
func idOrden(periodo string, circuito reparto.Circuito, titularID string) string {
	return "liq-" + periodo + "-" + string(circuito) + "-" + titularID
}

func hechoDeSilencio(estado liquidacion.Estado) string {
	if estado == liquidacion.EstadoDiferida {
		return HechoLiquidacionDiferida
	}
	return HechoLiquidacionAceptadaPorSilencio
}

// asuntoLiquidacion y cuerpoLiquidacion arman el aviso de R-10. El texto vive
// en esta capa y no en el adaptador porque el plazo que anuncia es una regla:
// quince dias CALENDARIO desde el envio, no habiles (R-22 usa habiles y no se
// unifican).
//
// Recibe la orden YA completa (arrastres incluidos): el silencio corre sobre
// la cifra que se le comunico al titular, no sobre el neto de la corrida
// antes del arrastre (RD 13.2, ADR 0006).
func asuntoLiquidacion(m MetaProceso) string {
	return fmt.Sprintf("Liquidacion del periodo %s (%s)", m.Periodo, m.Circuito)
}

func cuerpoLiquidacion(m MetaProceso, o liquidacion.OrdenDePago, enviadaDia string) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"Liquidacion del periodo %s, circuito %s, enviada el %s.\n"+
			"Bruto %s, neto %s.\n",
		m.Periodo, m.Circuito, enviadaDia,
		o.Bruto.StringFixed(2), o.Neto.StringFixed(2))
	if len(o.Arrastres) > 0 {
		fmt.Fprintf(&b, "Incluye arrastre de %s.\n", strings.Join(o.Arrastres, ", "))
	}
	fmt.Fprintf(&b,
		"Sin respuesta en %d dias calendario se entiende aceptada (R-10, RD 13.2).",
		liquidacion.PlazoAceptacionDias)
	return b.String()
}

// AsientoOrden es el payload JSONB de los asientos de liquidacion.
//
// Tipo propio y no [liquidacion.OrdenDePago] serializada: los modelos del
// dominio no llevan etiquetas json a proposito, asi que marshalearlos
// escribiria los nombres de los campos de Go en un libro que el ADR 0006
// manda conservar diez anos y que tiene que seguir siendo legible por una
// persona al final de ese plazo.
//
// Los montos van como cadena por la misma razon que en la forma de red: un
// JSON number es IEEE-754 y no puede representar dinero (ADR 0010).
type AsientoOrden struct {
	Periodo  string `json:"periodo"`
	Circuito string `json:"circuito"`

	// Procesos son las corridas que aportaron a la orden. Es lo que permite
	// reconstruir de donde salio un bruto agregado; sin ella, `proceso_id` a
	// secas afirmaria que todo vino de una sola corrida.
	Procesos []string `json:"procesos"`

	TitularID   string              `json:"titular_id"`
	Bruto       string              `json:"bruto"`
	Deducciones []DeduccionAsentada `json:"deducciones"`
	Neto        string              `json:"neto"`
	Estado      string              `json:"estado"`

	// EstadoAnterior va vacio en la emision -- no habia nada antes -- y con el
	// estado que esta transicion sustituyo en las demas.
	EstadoAnterior string `json:"estado_anterior,omitempty"`

	Enviada   string   `json:"enviada"`
	Arrastres []string `json:"arrastres,omitempty"`

	// Acuse es la prueba de la notificacion, y es lo que fecha el arranque del
	// plazo de R-10: `enviada` es un dia civil, y sin acuse no hay nada que
	// respalde que ese dia salio algo.
	Acuse string `json:"acuse,omitempty"`
}

// ResiduoProrrateoAsentado es [liquidacion.ResiduoProrrateo] en el libro:
// montos como cadena (ADR 0010). Vive en el asiento de lote
// ([HechoLiquidacionResiduoProrrateo]), no en cada orden.
type ResiduoProrrateoAsentado struct {
	Admin   string `json:"admin"`
	Social  string `json:"social"`
	Reserva string `json:"reserva"`
}

// DeduccionAsentada es un renglon del desglose en el libro.
type DeduccionAsentada struct {
	Concepto string `json:"concepto"`
	Monto    string `json:"monto"`
}

func residuoAsentadoDe(r liquidacion.ResiduoProrrateo) ResiduoProrrateoAsentado {
	return ResiduoProrrateoAsentado{
		Admin:   r.Admin.StringFixed(2),
		Social:  r.Social.StringFixed(2),
		Reserva: r.Reserva.StringFixed(2),
	}
}

func asientoDeOrden(
	o liquidacion.OrdenDePago, anterior liquidacion.Estado, acuse string,
) AsientoOrden {
	deducciones := make([]DeduccionAsentada, 0, len(o.Deducciones))
	for _, d := range o.Deducciones {
		deducciones = append(deducciones, DeduccionAsentada{
			Concepto: d.Concepto,
			Monto:    d.Monto.StringFixed(2),
		})
	}
	return AsientoOrden{
		Periodo:        o.Periodo,
		Circuito:       o.Circuito,
		Procesos:       o.Procesos,
		TitularID:      o.TitularID,
		Bruto:          o.Bruto.StringFixed(2),
		Deducciones:    deducciones,
		Neto:           o.Neto.StringFixed(2),
		Estado:         string(o.Estado),
		EstadoAnterior: string(anterior),
		Enviada:        o.EnviadaDia,
		Arrastres:      o.Arrastres,
		Acuse:          acuse,
	}
}

// diaCivil deja el helper de formato en un solo sitio: las pruebas del
// plazo mueven el Reloj y comparan contra este mismo recorte.
func diaCivil(instante time.Time) string {
	return instante.UTC().Format("2006-01-02")
}
