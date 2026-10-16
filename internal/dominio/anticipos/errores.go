package anticipos

import "errors"

// Los centinelas nombran la regla que se incumple, porque quien recibe el
// rechazo es un socio o el Consejo Directivo y "no se puede" a secas no le dice
// que tiene que cambiar. El detalle (montos, estado encontrado) va envuelto en
// el texto.
var (
	// ErrSolicitudInvalida: la solicitud no tiene lo que RA 3.2.1 exige que
	// conste -monto y la justificacion de la necesidad- o le falta identidad.
	ErrSolicitudInvalida = errors.New("solicitud de anticipo invalida")

	// ErrNoEsSocio es R-30 (RA 2.2): un Titular Administrado no pide anticipo.
	ErrNoEsSocio = errors.New("solo un socio puede pedir anticipo (R-30, RA 2.2)")

	// ErrSuperaTope es R-31 (RA 3.1.g y 3.1.h).
	ErrSuperaTope = errors.New("el monto supera el tope del anticipo (R-31, RA 3.1.g y 3.1.h)")

	// ErrAnticipoVigente es RA 3.1.c: ya hay una solicitud en tramite.
	ErrAnticipoVigente = errors.New("ya hay un anticipo vigente (R-32, RA 3.1.c)")

	// ErrSaldoPendiente es RA 3.1.d: hay un anticipo otorgado sin saldar.
	ErrSaldoPendiente = errors.New("hay un anticipo con saldo pendiente (R-32, RA 3.1.d)")

	// ErrEmbargoReportado es RA 3.1.e.
	ErrEmbargoReportado = errors.New("el afiliado tiene embargos reportados (R-32, RA 3.1.e)")

	// ErrEstadoInvalido: la transicion no sale del estado en que esta el
	// anticipo. Solo una solicitud en tramite se aprueba o se rechaza.
	ErrEstadoInvalido = errors.New("el anticipo no esta en un estado que admita esa decision")

	// ErrResolucionIncompleta: una aprobacion sin la ubicacion de su soporte
	// (RA 3.2.5) o un rechazo sin justificacion (RA 3.2.3).
	ErrResolucionIncompleta = errors.New("la decision sobre el anticipo esta incompleta")
)
