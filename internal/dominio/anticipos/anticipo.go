package anticipos

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/rosvend/intela/internal/dominio/afiliacion"
)

// Estado de un anticipo. Es el mismo conjunto que el CHECK de
// `anticipos.estado` (migracion 00001).
//
//	solicitado -> aprobado -> descontado
//	solicitado -> rechazado
//
// `descontado` es "saldado": el saldo llego a cero con los descuentos de R-33.
// No hay vuelta atras desde ninguno de los finales.
//
// El giro (RA 3.2.4) no es un estado sino un hecho dentro de `aprobado`: el
// Consejo aprueba y Tesoreria gira despues, hasta diez dias habiles mas tarde.
// Solo un anticipo girado se descuenta (ver [Anticipo.Girar]).
type Estado string

const (
	EstadoSolicitado Estado = "solicitado"
	EstadoAprobado   Estado = "aprobado"
	EstadoRechazado  Estado = "rechazado"
	EstadoDescontado Estado = "descontado"
)

// ParseEstado lee un estado de su texto. La cadena vacia es "ninguno" y vuelve
// vacia sin error; cualquier otro texto que no sea un estado es
// [ErrSolicitudInvalida].
func ParseEstado(s string) (Estado, error) {
	switch e := Estado(strings.TrimSpace(s)); e {
	case "", EstadoSolicitado, EstadoAprobado, EstadoRechazado, EstadoDescontado:
		return e, nil
	default:
		return "", fmt.Errorf("%w: el estado %q no existe", ErrSolicitudInvalida, s)
	}
}

// Anticipo es una liquidacion adelantada de derechos de autor (RA 2.1).
//
// Monto es lo otorgado y no cambia; Saldo es lo que falta por cubrir con los
// derechos de las liquidaciones que siguen, y solo es positivo mientras el
// anticipo esta aprobado. La identidad que se conserva es
//
//	Monto = Saldo + suma de lo descontado
//
// y es lo que deja responder "cuanto queda" sin recalcular ninguna corrida.
type Anticipo struct {
	ID        string
	TitularID string
	Monto     decimal.Decimal
	Saldo     decimal.Decimal
	Estado    Estado

	// Justificacion es el destino de los recursos y la necesidad que lo
	// motiva (RA 3.2.1.d y e; las circunstancias de RA 2.3).
	Justificacion string

	// Soporte es la ubicacion del acta y la resolucion de aprobacion en el
	// repositorio electronico (RA 3.2.5: "indicada su ubicacion ... con el fin
	// de garantizar trazabilidad"). Vacio mientras no se aprueba.
	Soporte string

	// MotivoRechazo es la justificacion literal de la negacion (RA 3.2.3).
	MotivoRechazo string

	// Girado dice si el dinero ya salio hacia el socio (RA 3.2.4).
	// SoporteGiro es la ubicacion del registro de la transferencia (RA 3.3,
	// "Registro: Resolucion y registro de transferencia"). Mientras no hay
	// giro, R-33 no descuenta nada: no se le cobra al socio lo que todavia no
	// recibio.
	Girado      bool
	SoporteGiro string
}

// Solicitud es lo que el socio pide (RA 3.2.1). El id lo genera quien llama.
type Solicitud struct {
	ID            string
	TitularID     string
	Monto         decimal.Decimal
	Justificacion string
}

// Situacion es lo que R-30, R-31 y R-32 miran del socio en la fecha de la
// decision. La arma aplicacion con lo que leen los puertos; aqui no se
// pregunta nada a nadie.
type Situacion struct {
	// Clase es la del padron (`titulares.clase`), no la de la solicitud de
	// afiliacion: el padron es el que dice quien es socio hoy.
	Clase afiliacion.Clase

	// EmbargoReportado: hay un embargo vigente reportado a la sociedad.
	EmbargoReportado bool

	// Anticipos son TODOS los del titular, en cualquier estado.
	Anticipos []Anticipo

	// Derechos es la base de RA 3.1.g para la ventana de [VentanaAnios].
	Derechos decimal.Decimal

	// SMMLV es el vigente en la fecha de la decision (ADR 0004).
	SMMLV decimal.Decimal
}

// Solicitar construye una solicitud en tramite o la rechaza.
//
// Pedir un anticipo es ya pasar por las reglas: un socio que no tiene cupo, o
// que tiene un anticipo sin saldar, no deja una solicitud que el Consejo tenga
// que negar, recibe el motivo en el acto. Y aun asi "la solicitud de un
// anticipo no supone su autorizacion" (RA 3.1.b): el saldo nace al aprobar.
func Solicitar(s Solicitud, sit Situacion) (Anticipo, error) {
	id := strings.TrimSpace(s.ID)
	titular := strings.TrimSpace(s.TitularID)
	justificacion := strings.TrimSpace(s.Justificacion)

	switch {
	case id == "" || titular == "":
		return Anticipo{}, fmt.Errorf("%w: falta el identificador o el titular", ErrSolicitudInvalida)
	case !s.Monto.IsPositive():
		return Anticipo{}, fmt.Errorf("%w: el monto %s no es positivo", ErrSolicitudInvalida, s.Monto)
	case !s.Monto.Equal(s.Monto.Round(2)):
		return Anticipo{}, fmt.Errorf("%w: el monto %s tiene mas de dos decimales", ErrSolicitudInvalida, s.Monto)
	case justificacion == "":
		return Anticipo{}, fmt.Errorf("%w: falta la justificacion de la necesidad (RA 3.2.1.d y e)", ErrSolicitudInvalida)
	}

	a := Anticipo{
		ID:            id,
		TitularID:     titular,
		Monto:         s.Monto,
		Saldo:         decimal.Zero,
		Estado:        EstadoSolicitado,
		Justificacion: justificacion,
	}
	if err := a.elegible(sit); err != nil {
		return Anticipo{}, err
	}
	return a, nil
}

// Aprobar es la decision del Consejo Directivo (RA 3.2.3). Exige la ubicacion
// del soporte (RA 3.2.5) y vuelve a pasar las reglas con la situacion de la
// fecha de aprobacion: entre pedir y aprobar puede llegar un embargo, perderse
// la calidad de socio o moverse la ventana del cupo, y lo que el reglamento
// prohibe es otorgar -"no podran hacer desembolsos" (RA 3.1.e)-, no pedir.
//
// Al aprobar, el saldo es todo lo otorgado.
func (a Anticipo) Aprobar(soporte string, sit Situacion) (Anticipo, error) {
	if a.Estado != EstadoSolicitado {
		return Anticipo{}, fmt.Errorf("%w: aprobar exige %q y esta en %q", ErrEstadoInvalido, EstadoSolicitado, a.Estado)
	}
	soporte = strings.TrimSpace(soporte)
	if soporte == "" {
		return Anticipo{}, fmt.Errorf("%w: aprobar exige la ubicacion del acta o resolucion (RA 3.2.5)", ErrResolucionIncompleta)
	}
	if err := a.elegible(sit); err != nil {
		return Anticipo{}, err
	}
	a.Estado = EstadoAprobado
	a.Saldo = a.Monto
	a.Soporte = soporte
	return a, nil
}

// Girar registra la transferencia al socio de un anticipo aprobado (RA 3.2.4).
// soporte es la ubicacion del registro de la transferencia (RA 3.3).
//
// Revisa otra vez el embargo, porque lo que RA 3.1.e prohibe es desembolsar:
// un embargo que llega entre la aprobacion y el giro detiene el giro. El
// saldo no cambia: nacio al aprobar como lo comprometido, y desde el giro es
// lo que R-33 puede descontar.
func (a Anticipo) Girar(soporte string, embargoReportado bool) (Anticipo, error) {
	if a.Estado != EstadoAprobado || a.Girado {
		return Anticipo{}, fmt.Errorf("%w: girar exige un anticipo %q sin girar y esta en %q (girado: %t)",
			ErrEstadoInvalido, EstadoAprobado, a.Estado, a.Girado)
	}
	soporte = strings.TrimSpace(soporte)
	if soporte == "" {
		return Anticipo{}, fmt.Errorf("%w: girar exige la ubicacion del registro de la transferencia (RA 3.3)", ErrResolucionIncompleta)
	}
	if embargoReportado {
		return Anticipo{}, fmt.Errorf("%w: titular %s", ErrEmbargoReportado, a.TitularID)
	}
	a.Girado = true
	a.SoporteGiro = soporte
	return a, nil
}

// Rechazar es la negacion del Consejo Directivo. RA 3.2.3: "En caso de
// negacion, debera justificarlo al afiliado de manera literal a la indicada en
// el Consejo Directivo", asi que sin motivo no hay rechazo.
func (a Anticipo) Rechazar(motivo string) (Anticipo, error) {
	if a.Estado != EstadoSolicitado {
		return Anticipo{}, fmt.Errorf("%w: rechazar exige %q y esta en %q", ErrEstadoInvalido, EstadoSolicitado, a.Estado)
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return Anticipo{}, fmt.Errorf("%w: el rechazo exige la justificacion (RA 3.2.3)", ErrResolucionIncompleta)
	}
	a.Estado = EstadoRechazado
	a.MotivoRechazo = motivo
	return a, nil
}

// Recuperar es R-33 (RA 2.1): el anticipo "sera cubierto de manera automatica
// con cargo a los derechos generados a futuro".
//
// disponible es lo que la liquidacion le iba a pagar al socio. Se descuenta lo
// que quede de saldo, sin pasar de lo disponible: una liquidacion no deja al
// socio debiendo, y un anticipo no se cobra dos veces. Cuando el saldo llega a
// cero el anticipo queda descontado y las liquidaciones siguientes se pagan
// enteras.
//
// Devuelve el descuento y el anticipo despues de aplicarlo. Si no hay nada que
// descontar -no esta aprobado, no se ha girado, no hay saldo o no hay
// disponible- el descuento es cero y el anticipo vuelve igual.
func (a Anticipo) Recuperar(disponible decimal.Decimal) (decimal.Decimal, Anticipo) {
	if a.Estado != EstadoAprobado || !a.Girado || !a.Saldo.IsPositive() || !disponible.IsPositive() {
		return decimal.Zero, a
	}
	descuento := decimal.Min(a.Saldo, disponible)
	a.Saldo = a.Saldo.Sub(descuento)
	if a.Saldo.IsZero() {
		a.Estado = EstadoDescontado
	}
	return descuento, a
}

// elegible aplica R-30, R-32 y R-31, en ese orden: primero quien es, luego lo
// que ya debe, y por ultimo cuanto puede pedir. El orden decide que motivo se
// le da a quien incumple varias a la vez, y el de la clase es el que no tiene
// arreglo.
func (a Anticipo) elegible(sit Situacion) error {
	if sit.Clase != afiliacion.ClaseSocio {
		return fmt.Errorf("%w: el titular %s figura como %q", ErrNoEsSocio, a.TitularID, sit.Clase)
	}
	if sit.EmbargoReportado {
		return fmt.Errorf("%w: titular %s", ErrEmbargoReportado, a.TitularID)
	}
	for _, otro := range sit.Anticipos {
		// El mismo anticipo, cuando se vuelve a mirar al aprobarlo, no se
		// bloquea a si mismo.
		if otro.ID == a.ID {
			continue
		}
		switch otro.Estado {
		case EstadoSolicitado:
			return fmt.Errorf("%w: la solicitud %s sigue en tramite", ErrAnticipoVigente, otro.ID)
		case EstadoAprobado:
			return fmt.Errorf("%w: al anticipo %s le quedan %s por descontar",
				ErrSaldoPendiente, otro.ID, otro.Saldo.StringFixed(2))
		}
	}
	tope := Tope(sit.Derechos, sit.SMMLV)
	if a.Monto.GreaterThan(tope) {
		return fmt.Errorf("%w: se piden %s y el tope es %s (25%% de %s en derechos de los ultimos %d anos, o %d SMMLV de %s)",
			ErrSuperaTope, a.Monto.StringFixed(2), tope.StringFixed(2),
			sit.Derechos.StringFixed(2), VentanaAnios, SalariosMinimos, sit.SMMLV.StringFixed(2))
	}
	return nil
}
