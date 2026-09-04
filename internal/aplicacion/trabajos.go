package aplicacion

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ---------------------------------------------------------------------------
// La clave natural
// ---------------------------------------------------------------------------

// TipoTrabajo es el catalogo de trabajos que la cola sabe transportar.
//
// Es una lista cerrada a proposito. El tipo es la mitad de la clave natural y
// es lo que elige manejador: una cadena libre deja encolar "ejecutar_repato"
// -con la errata- sin que nada proteste hasta que el trabajo caduca sin
// manejador, meses despues, en la unica corrida del ano.
type TipoTrabajo string

const (
	// TrabajoResolverUsos pasa los usos pendientes por la cascada de
	// identificacion (ADR 0007). Lo implementa el issue #37.
	TrabajoResolverUsos TipoTrabajo = "resolver_usos"

	// TrabajoEjecutarReparto abre el proceso de reparto de un periodo.
	//
	// ABRE, no ejecuta: el ADR 0008 es explicito en que el calendario dispara
	// AbrirProcesoDeReparto y a partir de ahi el proceso avanza por accion
	// humana o por trabajos que el propio proceso encola. Lo implementan los
	// issues #33 y #34.
	TrabajoEjecutarReparto TipoTrabajo = "ejecutar_reparto"
)

// tiposConocidos es la lista contra la que valida [ClaveTrabajo.Valida].
var tiposConocidos = map[TipoTrabajo]bool{
	TrabajoResolverUsos:    true,
	TrabajoEjecutarReparto: true,
}

// periodoValido es el mismo patron que el CHECK de las tablas `reportes`,
// `bolsas`, `procesos` y `cola_trabajos`: un ano, o un ano y un mes.
//
// Duplicado a proposito en las dos orillas. La base lo comprueba porque una
// fila mal formada no se puede permitir aunque la escriba otro cliente; el
// nucleo lo comprueba porque rechazar un periodo invalido cuando se encola es
// mucho mas barato que descubrirlo cuando la insercion revienta.
var periodoValido = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2})?$`)

// ClaveTrabajo identifica un trabajo por lo que significa, no por su fila.
//
// # Por que existe
//
// El objetivo 7 / KR-3 pide que el agente despierte por calendario, ejecute el
// pipeline y NUNCA repita trabajo ni pagos. Con una cola de identidad
// autoincremental, encolar dos veces el reparto de 2026 -porque el scheduler
// reintento, porque alguien pulso el boton dos veces, porque la marca de
// disparado no llego a escribirse- crea dos filas y paga el periodo dos veces.
// La clave natural convierte ese duplicado en un no-op de la base de datos.
//
// # Corrida no es Intentos
//
// Son los dos numeros de este paquete que mas facil se confunden:
//
//   - Corrida es cual corrida LOGICA del periodo es. 1 es la original.
//     Mayor que 1 es una corrida de AJUSTE, que es la unica forma legitima de
//     volver sobre un periodo ya repartido: RD 14.5.10 a RD 14.5.12 ajustan
//     con una liquidacion nueva y avalada, no reabriendo la anterior. Es parte
//     de la clave, asi que pedir una corrida de ajuste explicita SI encola;
//     repetir la corrida 1 no.
//   - Trabajo.Intentos cuenta cuantas veces se ha tomado ese mismo trabajo
//     tras fallar. No es parte de la clave y no tiene significado de negocio.
type ClaveTrabajo struct {
	Tipo    TipoTrabajo
	Periodo string
	Corrida int
}

// Valida comprueba la clave antes de que llegue a la base.
func (c ClaveTrabajo) Valida() error {
	if !tiposConocidos[c.Tipo] {
		return fmt.Errorf("tipo de trabajo desconocido %q", c.Tipo)
	}
	if !periodoValido.MatchString(c.Periodo) {
		return fmt.Errorf("periodo %q: se espera AAAA o AAAA-MM", c.Periodo)
	}
	if c.Corrida < 1 {
		return fmt.Errorf("corrida %d: la primera corrida de un periodo es la 1", c.Corrida)
	}
	return nil
}

// EsAjuste dice si esta clave pide una corrida de ajuste y no la original.
//
// Lo consume el manejador de reparto: una corrida de ajuste produce una
// liquidacion de ajuste, nunca un segundo pago del mismo importe.
func (c ClaveTrabajo) EsAjuste() bool { return c.Corrida > 1 }

// String da la forma corta que va a los logs y a los mensajes de error.
func (c ClaveTrabajo) String() string {
	return fmt.Sprintf("%s %s #%d", c.Tipo, c.Periodo, c.Corrida)
}

// ---------------------------------------------------------------------------
// El cierre de un trabajo
// ---------------------------------------------------------------------------

// Cierre describe como termino un trabajo. Hay exactamente tres formas, y se
// construyen con [Hecho], [Reintentar] y [Abandonar].
//
// Es un tipo y no tres metodos del puerto porque la decision -reintentar o
// rendirse- es politica del nucleo, no del adaptador. Un adaptador que
// recibiera "fallo" y decidiera el siguiente instante por su cuenta se
// llevaria la espera exponencial a la capa donde no se puede probar sin base
// de datos.
type Cierre struct {
	// Motivo del fallo. Vacio significa que salio bien.
	Motivo string

	// Volver es cuando el trabajo puede tomarse de nuevo. El instante cero
	// significa que no vuelve: o termino, o se abandona.
	Volver time.Time
}

// Hecho cierra un trabajo que salio bien.
func Hecho() Cierre { return Cierre{} }

// Reintentar devuelve el trabajo a la cola, disponible a partir de `cuando`.
func Reintentar(cuando time.Time, motivo string) Cierre {
	return Cierre{Motivo: motivo, Volver: cuando}
}

// Abandonar da el trabajo por fallido sin mas reintentos. La fila se queda
// como esta -con su motivo- y no se borra: es el rastro de que se intento.
func Abandonar(motivo string) Cierre { return Cierre{Motivo: motivo} }

// Exitoso dice si este cierre es el de un trabajo que salio bien.
func (c Cierre) Exitoso() bool { return c.Motivo == "" }

// Reintentado dice si el trabajo vuelve a la cola.
func (c Cierre) Reintentado() bool { return !c.Volver.IsZero() }

// ---------------------------------------------------------------------------
// La politica de reintentos
// ---------------------------------------------------------------------------

// Reintentos fija cuantas veces se reintenta un trabajo y cuanto se espera
// entre intentos.
//
// Ninguno de los tres valores es normativo: no salen del reglamento y no
// entran por ParametrosNormativos (ADR 0004). Son configuracion de operacion,
// y por eso los fija cmd/worker desde el entorno.
type Reintentos struct {
	// Maximo de tomas del mismo trabajo, contando la primera. Con 1 no hay
	// reintentos.
	Maximo int

	// Base es lo que se espera tras el primer fallo.
	Base time.Duration

	// Techo corta el crecimiento exponencial. Cero significa sin techo.
	Techo time.Duration
}

// Siguiente dice cuanto esperar tras el fallo numero `intentos`, y si queda
// algun reintento.
//
// La espera crece al doble -Base, 2xBase, 4xBase...- hasta el techo. Un fallo
// transitorio se resuelve en el primer reintento; uno que no lo es deja de
// castigar la base de datos enseguida.
func (r Reintentos) Siguiente(intentos int) (time.Duration, bool) {
	// Base cero dejaria el trabajo disponible en el acto: el worker giraria
	// sobre el sin ceder y el resto de la cola se quedaria detras. Sin espera
	// no hay reintento, que es la lectura segura de una configuracion asi.
	if r.Base <= 0 || intentos < 1 || intentos >= r.Maximo {
		return 0, false
	}
	espera := r.Base
	for range intentos - 1 {
		espera *= 2
		if r.Techo > 0 && espera > r.Techo {
			return r.Techo, true
		}
	}
	return espera, true
}

// ---------------------------------------------------------------------------
// El despachador
// ---------------------------------------------------------------------------

// ErrPermanente marca un fallo que no mejora reintentando.
//
// Un manejador que lo envuelve pide que el trabajo se abandone en vez de
// reprogramarse. Sin esta distincion, un trabajo mal formado -o de un tipo que
// este worker no sabe atender- consume todos los reintentos antes de rendirse,
// y la espera exponencial retrasa el aviso justo cuando el aviso es lo unico
// util que queda.
var ErrPermanente = errors.New("fallo permanente")

// Manejador procesa un trabajo de un tipo concreto.
//
// Devolver nil significa "hecho y cerrado". Cualquier error se registra en la
// fila y decide si hay reintento; envolver [ErrPermanente] pide que no lo
// haya.
type Manejador interface {
	Manejar(ctx context.Context, t Trabajo) error
}

// ManejadorFunc adapta una funcion suelta al puerto.
type ManejadorFunc func(ctx context.Context, t Trabajo) error

func (f ManejadorFunc) Manejar(ctx context.Context, t Trabajo) error { return f(ctx, t) }

// Despachador toma un trabajo de la cola y se lo entrega al manejador de su
// tipo.
//
// Vive aqui y no en cmd/worker porque es politica: que se reintenta, cuando y
// cuantas veces. En un main no se puede probar sin levantar un proceso; aqui
// se prueba con dobles y sin base de datos, que es lo que pide el ADR 0002.
//
// El bucle -el tic, la senal, el apagado- si es de cmd/worker: eso es ciclo de
// vida del proceso y no tiene nada que decidir.
type Despachador struct {
	Cola        ColaTrabajos
	Reloj       Reloj
	Reintentos  Reintentos
	Manejadores map[TipoTrabajo]Manejador
}

// ProcesarUno toma un trabajo, lo despacha y lo cierra.
//
// Devuelve ErrSinTrabajo cuando la cola esta vacia, que es la condicion de
// parada del bucle de vaciado y no un fallo.
//
// Cuando el manejador falla, el trabajo YA quedo cerrado -reprogramado o
// abandonado- y el error sube igualmente: quien llama lo registra en el log de
// operacion. Son dos rastros distintos y hacen falta los dos, el de la fila
// para el estado y el del log para quien opera.
func (d Despachador) ProcesarUno(ctx context.Context) (Trabajo, error) {
	ahora := d.Reloj.Ahora()

	t, err := d.Cola.Tomar(ctx, ahora)
	if err != nil {
		return Trabajo{}, err
	}

	if errManejo := d.manejar(ctx, t); errManejo != nil {
		cierre := d.cierreTrasFallo(t, ahora, errManejo)
		if err := d.Cola.Cerrar(ctx, t.ID, cierre); err != nil {
			return t, errors.Join(errManejo, err)
		}
		return t, errManejo
	}

	if err := d.Cola.Cerrar(ctx, t.ID, Hecho()); err != nil {
		return t, fmt.Errorf("cerrar el trabajo %s: %w", t.Clave, err)
	}
	return t, nil
}

// manejar busca el manejador del tipo y lo invoca, aislando el panico.
//
// El recover no es defensa generica: sin el, un manejador con un defecto tumba
// el worker y deja SU FILA en curso para siempre -nadie la vuelve a tomar,
// porque Tomar solo mira las pendientes-. Con el, el defecto queda escrito en
// la fila, el trabajo se abandona y los que venian detras se siguen
// procesando. Es permanente porque un panico no se arregla reintentandolo.
func (d Despachador) manejar(ctx context.Context, t Trabajo) (err error) {
	m, ok := d.Manejadores[t.Clave.Tipo]
	if !ok {
		return fmt.Errorf("sin manejador para %s: %w", t.Clave, ErrPermanente)
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panico manejando %s: %v: %w", t.Clave, r, ErrPermanente)
		}
	}()
	return m.Manejar(ctx, t)
}

// cierreTrasFallo decide si el trabajo vuelve a la cola o se abandona.
func (d Despachador) cierreTrasFallo(t Trabajo, ahora time.Time, causa error) Cierre {
	if errors.Is(causa, ErrPermanente) {
		return Abandonar(causa.Error())
	}
	espera, quedan := d.Reintentos.Siguiente(t.Intentos)
	if !quedan {
		return Abandonar(causa.Error())
	}
	return Reintentar(ahora.Add(espera), causa.Error())
}

// ---------------------------------------------------------------------------
// El planificador
// ---------------------------------------------------------------------------

// Planificador encola las corridas que el calendario declara vencidas.
//
// No es dueno de las fechas: las fija el Consejo Directivo y pueden cambiar
// por fuerza mayor (RD 12, ADR 0004). Por eso lee `calendario` en cada pasada
// en vez de llevar una expresion cron escrita a mano.
type Planificador struct {
	Calendario Calendario
	Cola       ColaTrabajos
	Reloj      Reloj
}

// Disparar encola el reparto de cada periodo vencido y lo marca disparado.
// Devuelve las claves que se encolaron de verdad.
//
// # Por que encolar va antes de marcar, y por que no hace falta transaccion
//
// Los dos puertos son independientes -el nucleo no sabe que detras hay una
// sola base de datos- asi que no hay una transaccion que los abrace. No hace
// falta, porque las dos operaciones son idempotentes y el orden es el que
// tolera la interrupcion:
//
//   - Si el proceso muere entre Encolar y MarcarDisparado, el periodo queda
//     encolado y sin marcar. La pasada siguiente lo vuelve a encolar (no-op
//     por clave natural) y lo marca. Nada se duplica.
//   - Al reves -marcar antes de encolar- una muerte en medio dejaria el
//     periodo marcado y sin trabajo: la corrida del ano no se ejecutaria y
//     nadie se enteraria hasta diciembre.
//
// # Un fallo corta la pasada
//
// El primer error interrumpe y devuelve lo hecho hasta ahi. El siguiente tic
// reconcilia, y como encolar es idempotente eso no cuesta nada. Con una
// distribucion al ano rara vez hay mas de un periodo vencido a la vez; seguir
// adelante tras un fallo compraria muy poco a cambio de agregar errores.
func (p Planificador) Disparar(ctx context.Context) ([]ClaveTrabajo, error) {
	ahora := p.Reloj.Ahora()

	periodos, err := p.Calendario.Pendientes(ctx, ahora)
	if err != nil {
		return nil, fmt.Errorf("leer el calendario: %w", err)
	}

	var encoladas []ClaveTrabajo
	for _, periodo := range periodos {
		// Corrida 1: el calendario abre la corrida ORIGINAL del periodo. Una
		// de ajuste la pide una persona con su aval, nunca un temporizador.
		clave := ClaveTrabajo{Tipo: TrabajoEjecutarReparto, Periodo: periodo, Corrida: 1}

		encolado, err := p.Cola.Encolar(ctx, clave, nil)
		if err != nil {
			return encoladas, fmt.Errorf("encolar %s: %w", clave, err)
		}
		if encolado {
			encoladas = append(encoladas, clave)
		}

		if err := p.Calendario.MarcarDisparado(ctx, periodo); err != nil {
			return encoladas, fmt.Errorf("marcar disparado %q: %w", periodo, err)
		}
	}
	return encoladas, nil
}
