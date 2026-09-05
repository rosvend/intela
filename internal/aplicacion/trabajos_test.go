package aplicacion

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Dobles de los dos puertos de operacion. Stdlib y nada mas, como el resto del
// paquete. Viven aqui y no en un paquete aparte por lo mismo que los de
// autenticacion_test.go: son la forma de estos casos de uso.

// colaMemoria entrega los trabajos de `pendientes` en orden y anota todo lo
// que se le pide. No simula el reclamo exclusivo -eso es propiedad del
// adaptador y se prueba contra PostgreSQL de verdad, en cola_test.go-.
type colaMemoria struct {
	pendientes []Trabajo
	encoladas  []ClaveTrabajo
	duplicadas map[string]bool
	cierres    map[int64]Cierre
	orden      []string

	errEncolar error
	errTomar   error
	errCerrar  error
}

func nuevaCola() *colaMemoria {
	return &colaMemoria{duplicadas: map[string]bool{}, cierres: map[int64]Cierre{}}
}

func (c *colaMemoria) Encolar(_ context.Context, clave ClaveTrabajo, _ []byte) (bool, error) {
	c.orden = append(c.orden, "encolar "+clave.String())
	if c.errEncolar != nil {
		return false, c.errEncolar
	}
	if c.duplicadas[clave.String()] {
		return false, nil
	}
	c.encoladas = append(c.encoladas, clave)
	return true, nil
}

func (c *colaMemoria) Tomar(context.Context, time.Time) (Trabajo, error) {
	if c.errTomar != nil {
		return Trabajo{}, c.errTomar
	}
	if len(c.pendientes) == 0 {
		return Trabajo{}, ErrSinTrabajo
	}
	t := c.pendientes[0]
	c.pendientes = c.pendientes[1:]
	return t, nil
}

func (c *colaMemoria) Cerrar(_ context.Context, id int64, cierre Cierre) error {
	if c.errCerrar != nil {
		return c.errCerrar
	}
	c.cierres[id] = cierre
	return nil
}

type calendarioMemoria struct {
	pendientes []string
	marcados   []string
	orden      *[]string

	errPendientes error
	errMarcar     error
}

func (c *calendarioMemoria) Pendientes(context.Context, time.Time) ([]string, error) {
	return c.pendientes, c.errPendientes
}

func (c *calendarioMemoria) MarcarDisparado(_ context.Context, periodo string) error {
	if c.orden != nil {
		*c.orden = append(*c.orden, "marcar "+periodo)
	}
	if c.errMarcar != nil {
		return c.errMarcar
	}
	c.marcados = append(c.marcados, periodo)
	return nil
}

var (
	_ ColaTrabajos = (*colaMemoria)(nil)
	_ Calendario   = (*calendarioMemoria)(nil)
)

// ---------------------------------------------------------------------------
// La clave natural
// ---------------------------------------------------------------------------

func TestClaveTrabajoValida(t *testing.T) {
	casos := []struct {
		nombre string
		clave  ClaveTrabajo
		valida bool
	}{
		{"periodo anual", ClaveTrabajo{TrabajoEjecutarReparto, "2026", 1}, true},
		{"periodo mensual", ClaveTrabajo{TrabajoResolverUsos, "2026-12", 1}, true},
		{"corrida de ajuste", ClaveTrabajo{TrabajoEjecutarReparto, "2026", 2}, true},

		{"tipo vacio", ClaveTrabajo{"", "2026", 1}, false},
		// La errata es el caso que motiva que TipoTrabajo sea una lista
		// cerrada: se encolaria sin que nada protestara.
		{"tipo con errata", ClaveTrabajo{"ejecutar_repato", "2026", 1}, false},
		{"periodo vacio", ClaveTrabajo{TrabajoEjecutarReparto, "", 1}, false},
		{"periodo con mes de tres cifras", ClaveTrabajo{TrabajoEjecutarReparto, "2026-123", 1}, false},
		{"periodo en otro formato", ClaveTrabajo{TrabajoEjecutarReparto, "dic-2026", 1}, false},
		{"corrida cero", ClaveTrabajo{TrabajoEjecutarReparto, "2026", 0}, false},
		{"corrida negativa", ClaveTrabajo{TrabajoEjecutarReparto, "2026", -1}, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := c.clave.Valida()
			if c.valida && err != nil {
				t.Fatalf("Valida() = %v, se esperaba nil", err)
			}
			if !c.valida && err == nil {
				t.Fatal("Valida() = nil, se esperaba error")
			}
		})
	}
}

// La corrida 1 es la original; cualquier otra es un ajuste. De esto depende
// que una segunda corrida del periodo produzca una liquidacion de ajuste y no
// un segundo pago del mismo importe.
func TestClaveTrabajoEsAjuste(t *testing.T) {
	original := ClaveTrabajo{TrabajoEjecutarReparto, "2026", 1}
	if original.EsAjuste() {
		t.Error("la corrida 1 no es un ajuste")
	}
	if !(ClaveTrabajo{TrabajoEjecutarReparto, "2026", 2}).EsAjuste() {
		t.Error("la corrida 2 si es un ajuste")
	}
}

// Dos claves distintas no pueden dar la misma cadena: si la dieran, el rastro
// del log no distinguiria la corrida original del ajuste.
func TestClaveTrabajoStringDistingueLasTresPartes(t *testing.T) {
	base := ClaveTrabajo{TrabajoEjecutarReparto, "2026", 1}
	otras := []ClaveTrabajo{
		{TrabajoResolverUsos, "2026", 1},
		{TrabajoEjecutarReparto, "2026-12", 1},
		{TrabajoEjecutarReparto, "2026", 2},
	}
	for _, o := range otras {
		if base.String() == o.String() {
			t.Errorf("%#v y %#v comparten cadena %q", base, o, base.String())
		}
	}
}

// ---------------------------------------------------------------------------
// La politica de reintentos
// ---------------------------------------------------------------------------

func TestReintentosSiguiente(t *testing.T) {
	politica := Reintentos{Maximo: 5, Base: 30 * time.Second, Techo: 2 * time.Minute}

	casos := []struct {
		nombre   string
		intentos int
		espera   time.Duration
		quedan   bool
	}{
		{"tras el primer fallo se espera la base", 1, 30 * time.Second, true},
		{"la espera se dobla", 2, time.Minute, true},
		{"y se vuelve a doblar", 3, 2 * time.Minute, true},
		{"el techo corta el crecimiento", 4, 2 * time.Minute, true},
		{"agotado el maximo no queda reintento", 5, 0, false},
		{"pasado el maximo tampoco", 9, 0, false},
		{"cero intentos no tiene sentido", 0, 0, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			espera, quedan := politica.Siguiente(c.intentos)
			if quedan != c.quedan {
				t.Fatalf("quedan = %v, se esperaba %v", quedan, c.quedan)
			}
			if espera != c.espera {
				t.Fatalf("espera = %v, se esperaba %v", espera, c.espera)
			}
		})
	}
}

// Sin techo la espera sigue doblando. Se comprueba porque el cero de Techo
// significa "sin limite" y es facil que se lea como "limite cero".
func TestReintentosSinTechoSigueDoblando(t *testing.T) {
	politica := Reintentos{Maximo: 10, Base: time.Second}
	espera, quedan := politica.Siguiente(5)
	if !quedan || espera != 16*time.Second {
		t.Fatalf("Siguiente(5) = %v, %v; se esperaba 16s, true", espera, quedan)
	}
}

// Base cero dejaria el trabajo disponible en el acto y el worker giraria sobre
// el sin ceder. La lectura segura de esa configuracion es "sin reintentos".
func TestReintentosBaseCeroNoReintenta(t *testing.T) {
	if _, quedan := (Reintentos{Maximo: 5}).Siguiente(1); quedan {
		t.Fatal("con Base cero no puede haber reintento")
	}
}

// ---------------------------------------------------------------------------
// El despachador
// ---------------------------------------------------------------------------

var ahoraPrueba = time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)

func nuevoDespachador(c *colaMemoria, m map[TipoTrabajo]Manejador) Despachador {
	return Despachador{
		Cola:        c,
		Reloj:       relojFijo{instante: ahoraPrueba},
		Reintentos:  Reintentos{Maximo: 3, Base: time.Minute, Techo: time.Hour},
		Manejadores: m,
	}
}

func trabajo(tipo TipoTrabajo, id int64, intentos int) Trabajo {
	return Trabajo{
		ID:       id,
		Clave:    ClaveTrabajo{Tipo: tipo, Periodo: "2026", Corrida: 1},
		Intentos: intentos,
	}
}

// La tabla de despacho: cada tipo va a SU manejador y no al otro. Es la
// propiedad que hace que anadir un tipo sea anadir una entrada, no un if.
func TestDespachadorEntregaAlManejadorDeSuTipo(t *testing.T) {
	var recibidos []string
	anota := func(nombre string) Manejador {
		return ManejadorFunc(func(_ context.Context, tr Trabajo) error {
			recibidos = append(recibidos, nombre+" "+tr.Clave.String())
			return nil
		})
	}

	cola := nuevaCola()
	cola.pendientes = []Trabajo{
		trabajo(TrabajoEjecutarReparto, 1, 1),
		trabajo(TrabajoResolverUsos, 2, 1),
	}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: anota("reparto"),
		TrabajoResolverUsos:    anota("usos"),
	})

	for range 2 {
		if _, err := d.ProcesarUno(t.Context()); err != nil {
			t.Fatalf("ProcesarUno: %v", err)
		}
	}

	esperados := []string{"reparto ejecutar_reparto 2026 #1", "usos resolver_usos 2026 #1"}
	if len(recibidos) != len(esperados) {
		t.Fatalf("recibidos = %v, se esperaban %v", recibidos, esperados)
	}
	for i := range esperados {
		if recibidos[i] != esperados[i] {
			t.Errorf("recibidos[%d] = %q, se esperaba %q", i, recibidos[i], esperados[i])
		}
	}
}

func TestDespachadorCierraHechoTrasExito(t *testing.T) {
	cola := nuevaCola()
	cola.pendientes = []Trabajo{trabajo(TrabajoEjecutarReparto, 7, 1)}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: ManejadorFunc(func(context.Context, Trabajo) error { return nil }),
	})

	if _, err := d.ProcesarUno(t.Context()); err != nil {
		t.Fatalf("ProcesarUno: %v", err)
	}
	cierre, hay := cola.cierres[7]
	if !hay {
		t.Fatal("el trabajo no se cerro")
	}
	if !cierre.Exitoso() || cierre.Reintentado() {
		t.Fatalf("cierre = %+v, se esperaba un cierre exitoso sin reintento", cierre)
	}
}

// Un fallo transitorio vuelve a la cola con la espera de la politica. Es el
// criterio "un trabajo que falla se registra y se reintenta, no se pierde".
func TestDespachadorReprogramaTrasUnFalloTransitorio(t *testing.T) {
	cola := nuevaCola()
	cola.pendientes = []Trabajo{trabajo(TrabajoEjecutarReparto, 7, 1)}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: ManejadorFunc(func(context.Context, Trabajo) error {
			return errors.New("la base no responde")
		}),
	})

	_, err := d.ProcesarUno(t.Context())
	if err == nil {
		t.Fatal("el fallo del manejador tiene que subir")
	}

	cierre := cola.cierres[7]
	if !cierre.Reintentado() {
		t.Fatal("un fallo transitorio se reprograma")
	}
	if quiero := ahoraPrueba.Add(time.Minute); !cierre.Volver.Equal(quiero) {
		t.Errorf("Volver = %v, se esperaba %v", cierre.Volver, quiero)
	}
	// El motivo queda escrito: sin el, la fila dice "fallido" y no por que.
	if cierre.Motivo == "" {
		t.Error("el cierre tiene que llevar el motivo del fallo")
	}
}

// Agotados los intentos deja de reprogramarse. Sin esto un trabajo roto se
// reintentaria para siempre y la cola no avanzaria mas alla de el.
func TestDespachadorAbandonaAlAgotarLosIntentos(t *testing.T) {
	cola := nuevaCola()
	cola.pendientes = []Trabajo{trabajo(TrabajoEjecutarReparto, 7, 3)}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: ManejadorFunc(func(context.Context, Trabajo) error {
			return errors.New("sigue fallando")
		}),
	})

	if _, err := d.ProcesarUno(t.Context()); err == nil {
		t.Fatal("el fallo del manejador tiene que subir")
	}
	cierre := cola.cierres[7]
	if cierre.Reintentado() || cierre.Exitoso() {
		t.Fatalf("cierre = %+v, se esperaba abandono", cierre)
	}
}

// ErrPermanente se abandona en el primer fallo, sin gastar reintentos: la
// espera exponencial solo retrasaria el aviso.
func TestDespachadorAbandonaUnFalloPermanenteEnElPrimerIntento(t *testing.T) {
	cola := nuevaCola()
	cola.pendientes = []Trabajo{trabajo(TrabajoEjecutarReparto, 7, 1)}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: ManejadorFunc(func(context.Context, Trabajo) error {
			return errors.New("payload ilegible: " + ErrPermanente.Error())
		}),
	})
	// El error de arriba NO envuelve el centinela: se reprograma.
	if _, err := d.ProcesarUno(t.Context()); err == nil {
		t.Fatal("el fallo tiene que subir")
	}
	if !cola.cierres[7].Reintentado() {
		t.Fatal("un error que solo MENCIONA el centinela no es permanente")
	}

	cola2 := nuevaCola()
	cola2.pendientes = []Trabajo{trabajo(TrabajoEjecutarReparto, 8, 1)}
	d2 := nuevoDespachador(cola2, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: ManejadorFunc(func(context.Context, Trabajo) error {
			return errors.Join(errors.New("payload ilegible"), ErrPermanente)
		}),
	})
	if _, err := d2.ProcesarUno(t.Context()); err == nil {
		t.Fatal("el fallo tiene que subir")
	}
	if cola2.cierres[8].Reintentado() {
		t.Fatal("un fallo permanente no se reprograma")
	}
}

// Un tipo sin manejador es permanente: reintentar no va a hacer que aparezca.
func TestDespachadorAbandonaUnTipoSinManejador(t *testing.T) {
	cola := nuevaCola()
	cola.pendientes = []Trabajo{trabajo(TrabajoResolverUsos, 7, 1)}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{})

	_, err := d.ProcesarUno(t.Context())
	if !errors.Is(err, ErrPermanente) {
		t.Fatalf("err = %v, se esperaba ErrPermanente", err)
	}
	if cola.cierres[7].Reintentado() {
		t.Fatal("un tipo sin manejador no se reprograma")
	}
}

// Un manejador que entra en panico no puede tumbar el worker ni dejar su fila
// en curso para siempre: Tomar solo mira las pendientes y nadie la recogeria.
func TestDespachadorAislaElPanicoDeUnManejador(t *testing.T) {
	cola := nuevaCola()
	cola.pendientes = []Trabajo{trabajo(TrabajoEjecutarReparto, 7, 1)}
	d := nuevoDespachador(cola, map[TipoTrabajo]Manejador{
		TrabajoEjecutarReparto: ManejadorFunc(func(context.Context, Trabajo) error {
			panic("indice fuera de rango")
		}),
	})

	_, err := d.ProcesarUno(t.Context())
	if !errors.Is(err, ErrPermanente) {
		t.Fatalf("err = %v, se esperaba ErrPermanente", err)
	}
	if cierre := cola.cierres[7]; cierre.Reintentado() || cierre.Exitoso() {
		t.Fatalf("cierre = %+v, se esperaba abandono con motivo", cierre)
	}
}

// Cola vacia es la condicion normal de un worker: pasa en todos los tics menos
// uno al ano. Sube tal cual para que el bucle de vaciado la reconozca.
func TestDespachadorPropagaColaVacia(t *testing.T) {
	d := nuevoDespachador(nuevaCola(), map[TipoTrabajo]Manejador{})
	if _, err := d.ProcesarUno(t.Context()); !errors.Is(err, ErrSinTrabajo) {
		t.Fatalf("err = %v, se esperaba ErrSinTrabajo", err)
	}
}

// ---------------------------------------------------------------------------
// El planificador
// ---------------------------------------------------------------------------

func nuevoPlanificador(cal *calendarioMemoria, cola *colaMemoria) Planificador {
	cal.orden = &cola.orden
	return Planificador{Calendario: cal, Cola: cola, Reloj: relojFijo{instante: ahoraPrueba}}
}

func TestPlanificadorEncolaYMarcaCadaPeriodoVencido(t *testing.T) {
	cal := &calendarioMemoria{pendientes: []string{"2025", "2026"}}
	cola := nuevaCola()

	encoladas, err := nuevoPlanificador(cal, cola).Disparar(t.Context())
	if err != nil {
		t.Fatalf("Disparar: %v", err)
	}

	if len(encoladas) != 2 {
		t.Fatalf("encoladas = %v, se esperaban dos", encoladas)
	}
	for i, clave := range encoladas {
		if clave.Tipo != TrabajoEjecutarReparto {
			t.Errorf("encoladas[%d].Tipo = %q", i, clave.Tipo)
		}
		// El calendario abre la corrida ORIGINAL. Una de ajuste la pide una
		// persona con su aval, nunca un temporizador (RD 14.5.10-12).
		if clave.Corrida != 1 || clave.EsAjuste() {
			t.Errorf("encoladas[%d].Corrida = %d, se esperaba 1", i, clave.Corrida)
		}
	}
	if len(cal.marcados) != 2 {
		t.Fatalf("marcados = %v, se esperaban dos", cal.marcados)
	}
}

// Encolar va ANTES de marcar. Al reves, una muerte del proceso en medio
// dejaria el periodo marcado y sin trabajo: la corrida del ano no se
// ejecutaria y nadie se enteraria hasta diciembre.
func TestPlanificadorEncolaAntesDeMarcar(t *testing.T) {
	cal := &calendarioMemoria{pendientes: []string{"2026"}}
	cola := nuevaCola()

	if _, err := nuevoPlanificador(cal, cola).Disparar(t.Context()); err != nil {
		t.Fatalf("Disparar: %v", err)
	}

	quiero := []string{"encolar ejecutar_reparto 2026 #1", "marcar 2026"}
	if len(cola.orden) != len(quiero) {
		t.Fatalf("orden = %v, se esperaba %v", cola.orden, quiero)
	}
	for i := range quiero {
		if cola.orden[i] != quiero[i] {
			t.Fatalf("orden = %v, se esperaba %v", cola.orden, quiero)
		}
	}
}

// Un periodo ya encolado no se duplica, y aun asi se marca: es la
// reconciliacion de la pasada anterior, que encolo y murio antes de marcar.
func TestPlanificadorNoDuplicaUnPeriodoYaEncolado(t *testing.T) {
	cal := &calendarioMemoria{pendientes: []string{"2026"}}
	cola := nuevaCola()
	cola.duplicadas["ejecutar_reparto 2026 #1"] = true

	encoladas, err := nuevoPlanificador(cal, cola).Disparar(t.Context())
	if err != nil {
		t.Fatalf("Disparar: %v", err)
	}
	if len(encoladas) != 0 {
		t.Fatalf("encoladas = %v, no se esperaba ninguna", encoladas)
	}
	if len(cal.marcados) != 1 {
		t.Fatalf("marcados = %v, el periodo tiene que quedar marcado igual", cal.marcados)
	}
}

func TestPlanificadorSinPeriodosVencidosNoEncolaNada(t *testing.T) {
	cal := &calendarioMemoria{}
	cola := nuevaCola()

	encoladas, err := nuevoPlanificador(cal, cola).Disparar(t.Context())
	if err != nil {
		t.Fatalf("Disparar: %v", err)
	}
	if len(encoladas) != 0 || len(cola.encoladas) != 0 {
		t.Fatalf("encoladas = %v / %v, no se esperaba ninguna", encoladas, cola.encoladas)
	}
}

// Un fallo corta la pasada y devuelve lo que si se hizo: perder ese dato
// dejaria un trabajo en la cola del que el log no dice nada.
func TestPlanificadorDevuelveLoHechoAntesDelFallo(t *testing.T) {
	cal := &calendarioMemoria{pendientes: []string{"2025", "2026"}, errMarcar: errors.New("sin conexion")}
	cola := nuevaCola()

	encoladas, err := nuevoPlanificador(cal, cola).Disparar(t.Context())
	if err == nil {
		t.Fatal("el fallo al marcar tiene que subir")
	}
	if len(encoladas) != 1 {
		t.Fatalf("encoladas = %v, se esperaba solo el primero", encoladas)
	}
}

func TestPlanificadorPropagaUnFalloDelCalendario(t *testing.T) {
	cal := &calendarioMemoria{errPendientes: errors.New("sin conexion")}
	if _, err := nuevoPlanificador(cal, nuevaCola()).Disparar(t.Context()); err == nil {
		t.Fatal("el fallo del calendario tiene que subir")
	}
}
