package identificacion

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// MaxNotaResolucion es el tope de la nota de una resolucion manual, en runas
// (D5). Se cuenta en runas y no en bytes para que una nota en espanol con
// tildes o enes no valga menos que una en ASCII, y se cuenta TRAS recortar los
// espacios de los extremos: el tope es del texto que se guarda.
const MaxNotaResolucion = 300

// Decision es lo que una persona hace con un caso ONI: darle una obra o decir
// que no es un uso del repertorio de REDES SGC.
type Decision string

const (
	DecisionAsignar   Decision = "asignar"
	DecisionDescartar Decision = "descartar"
)

// Errores de la resolucion manual.
//
// Son del dominio y no de [aplicacion] porque la transicion que describen es
// una regla del modelo, no una forma de la peticion: el caso de uso no los
// inventa, los aplica. Se envuelven con el detalle concreto (que escalon se
// encontro, que decision llego) para que el adaptador HTTP pueda responder 400
// o 409 con un mensaje que diga que paso.
var (
	// ErrNotaVacia: resolver un caso exige justificar la decision. No basta la
	// firma: la auditoria de RD 16 pregunta por que se decidio (ADR 0021).
	ErrNotaVacia = errors.New("la nota es obligatoria para resolver un caso")

	// ErrNotaDemasiadoLarga: la nota pasa del tope de [MaxNotaResolucion] runas.
	ErrNotaDemasiadoLarga = fmt.Errorf("la nota no puede pasar de %d caracteres", MaxNotaResolucion)

	// ErrDecisionInvalida: la decision no es 'asignar' ni 'descartar', o trae
	// una obra que no le corresponde.
	ErrDecisionInvalida = errors.New("decision invalida")

	// ErrCasoYaResuelto: el caso ya lo cerro otra persona, asignando o
	// descartando. Es "llegaste segundo", y por eso un 409 y no un 200: la
	// firma que quedo escrita no es la de quien acaba de pulsar el boton
	// (ADR 0006), y decir que si afirmaria lo contrario.
	ErrCasoYaResuelto = errors.New("otra persona ya resolvio este caso")

	// ErrCasoNoPendiente: la fila esta en un escalon que la cola de resolucion
	// manual no sirve -- la cascada ya la resolvio, o nunca fue un caso. Se
	// envuelve siempre nombrando el escalon actual.
	ErrCasoNoPendiente = errors.New("el caso ya no esta pendiente")
)

// NormalizarNota recorta los espacios de los extremos y valida el resultado.
//
// Devuelve el texto YA recortado, que es el que se guarda y el que viaja al
// asiento: recortar en un sitio y guardar en otro dejaria dos versiones de la
// misma justificacion, y la que se audita es la del asiento.
func NormalizarNota(nota string) (string, error) {
	nota = strings.TrimSpace(nota)
	if nota == "" {
		return "", ErrNotaVacia
	}
	if utf8.RuneCountInString(nota) > MaxNotaResolucion {
		return "", ErrNotaDemasiadoLarga
	}
	return nota, nil
}

// ValidarDecision comprueba la FORMA del pedido antes de leer nada de la base:
// asignar exige obra, descartar la prohibe. Es lo que evita abrir una
// transaccion y tomar el cerrojo de un periodo para acabar diciendo que el
// cuerpo estaba mal.
func ValidarDecision(d Decision, obraID string) error {
	switch d {
	case DecisionAsignar:
		if strings.TrimSpace(obraID) == "" {
			return fmt.Errorf("%w: %q necesita obra_id", ErrDecisionInvalida, DecisionAsignar)
		}
		return nil
	case DecisionDescartar:
		if strings.TrimSpace(obraID) != "" {
			return fmt.Errorf("%w: %q no admite obra_id", ErrDecisionInvalida, DecisionDescartar)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q, se esperaba %q o %q",
			ErrDecisionInvalida, d, DecisionAsignar, DecisionDescartar)
	}
}

// ResolverCaso aplica la transicion de una resolucion manual sobre el escalon
// leido, y devuelve el Resultado que se persiste y que va al asiento.
//
// Solo 'oni' es resoluble: es el unico escalon que significa "no se pudo
// reconocer" (RD 13.8) y sobre el que una persona tiene algo que decidir. Los
// otros dos desenlaces son distintos y por eso llevan sentinelas distintos:
//
//   - 'manual' y 'descartado' -> [ErrCasoYaResuelto]: alguien decidio antes.
//   - 'pendiente', 'alias', 'id_global', 'difuso', 'excluido' ->
//     [ErrCasoNoPendiente]: nunca fue un caso de esta cola, o la cascada ya lo
//     cerro sola.
//
// Corregir una decision manual -- reasignar, o deshacer un descarte -- queda
// fuera de alcance (D4): no hay transicion para eso, y este es el unico sitio
// donde podria colarse.
func ResolverCaso(escalonActual string, d Decision, obraID string, candidatos []Candidato) (Resultado, error) {
	switch escalonActual {
	case EscalonONI:
		// resoluble
	case EscalonManual, EscalonDescartado:
		return Resultado{}, fmt.Errorf("%w: el caso esta en %q", ErrCasoYaResuelto, escalonActual)
	default:
		return Resultado{}, fmt.Errorf("%w: el caso esta en %q", ErrCasoNoPendiente, escalonActual)
	}

	if d == DecisionDescartar {
		return Resultado{
			Escalon:   EscalonDescartado,
			ONI:       false,
			Evidencia: "descartado a mano: no es un uso del repertorio",
		}, nil
	}

	return asignacionManual(obraID, candidatos), nil
}

// asignacionManual arma el Resultado de un "asignar".
//
// El puntaje sale del candidato SOLO si la obra elegida estaba entre los
// propuestos: es lo que deja medir despues si una persona confirma al motor o
// lo corrige. Una obra buscada en el catalogo y que el motor no propuso no
// tiene puntaje que heredar -- inventarle uno afirmaria un parecido que nadie
// calculo --, y queda en cero con una evidencia que lo dice.
func asignacionManual(obraID string, candidatos []Candidato) Resultado {
	for _, c := range candidatos {
		if c.ObraID == obraID {
			return Resultado{
				ObraID:  obraID,
				Escalon: EscalonManual,
				Puntaje: c.Puntaje,
				ONI:     false,
				Evidencia: fmt.Sprintf("manual: candidata %s (%s) de %d propuestas",
					obraID, puntajeLegible(c.Puntaje), len(candidatos)),
			}
		}
	}
	return Resultado{
		ObraID:  obraID,
		Escalon: EscalonManual,
		Puntaje: decimal.Zero,
		ONI:     false,
		Evidencia: fmt.Sprintf("manual: obra %s buscada en el catalogo, fuera de las %d candidatas",
			obraID, len(candidatos)),
	}
}
