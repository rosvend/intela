package oni

import (
	"errors"
	"strings"
)

// Errores de la proyeccion publica y de la publicacion.
var (
	// ErrTituloAusente: sin titulo no hay nada que publicar. RD 13.8.2 pide
	// titulos para facilitar la identificacion.
	ErrTituloAusente = errors.New("falta el titulo")

	// ErrPeriodoAusente: R-18 / RD 13.8.4.2 exigen el periodo.
	ErrPeriodoAusente = errors.New("falta el periodo")

	// ErrDireccionAusente: R-18 / RD 13.8.4.3 exigen direccion fisica y
	// electronica para allegar documentacion.
	ErrDireccionAusente = errors.New("faltan las direcciones de notificacion")

	// ErrFechaAusente: R-18 / RD 13.8.4.1 exigen la fecha del proceso.
	ErrFechaAusente = errors.New("falta la fecha del proceso")
)

// DatosIdentificatorios es lo que se puede saber de un uso en ONI sin
// mirar dinero. Es el insumo de [Proyectar]: cualquier campo que parezca un
// importe no existe en este tipo, asi que la proyeccion publica no puede
// filtrarlo "por descuido".
type DatosIdentificatorios struct {
	ID        string
	Titulo    string
	Fuente    string
	IDsFuente string
	Modalidad string
	Periodo   string
}

// ProyeccionPublica es lo que R-18 permite poner en la web: titulo e
// informacion identificatoria. No tiene, y no va a tener, campo de dinero.
//
// El asiento interno si guarda el monto (ADR 0006); esta vista no lo expone.
// Trazabilidad interna completa no es lo mismo que publicidad.
type ProyeccionPublica struct {
	ID        string
	Titulo    string
	Fuente    string
	IDsFuente string
	Modalidad string
	Periodo   string
}

// Proyectar recorta un uso en ONI a lo publicable.
//
// No hay rama que copie un importe porque el tipo de entrada no tiene
// ninguno: el invariante de R-18 se sostiene en el sistema de tipos, no en
// una lista de columnas que alguien puede olvidar filtrar.
func Proyectar(d DatosIdentificatorios) (ProyeccionPublica, error) {
	if strings.TrimSpace(d.Titulo) == "" {
		return ProyeccionPublica{}, ErrTituloAusente
	}
	// Conversion directa: los dos tipos son el mismo conjunto de campos
	// identificatorios. Si alguien anade un importe a uno y no al otro,
	// esto deja de compilar.
	return ProyeccionPublica(d), nil
}

// ValidarMetadatos comprueba lo que RD 13.8.4 exige junto al listado:
// periodo y las dos direcciones a las que se allega documentacion.
//
// La fecha del proceso no entra aqui: depguard deniega el paquete time
// dentro del dominio, y el instante llega por [AnclarFecha] ya formateado.
func ValidarMetadatos(periodo, fisica, electronica string) error {
	if strings.TrimSpace(periodo) == "" {
		return ErrPeriodoAusente
	}
	if strings.TrimSpace(fisica) == "" || strings.TrimSpace(electronica) == "" {
		return ErrDireccionAusente
	}
	return nil
}
