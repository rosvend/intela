package identificacion

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// SugerenciaNinguna es la accion cuando no hay con que proponer: ni candidatas
// ni resoluciones anteriores del mismo titulo. No es una decision; es la
// ausencia de una.
const SugerenciaNinguna = "ninguna"

// Pesos del rankeador heuristico (#53).
//
// No es un modelo entrenado: ADR 0007 deja el aprendizaje automatico para
// cuando haya datos de verdad. Mientras tanto, la similitud del escalon
// difuso pesa mas que el historial, y el historial solo empata cuando varias
// personas ya decidieron lo mismo sobre el mismo titulo.
//
// La suma es 1 para que la confianza que sale no pase de 1 cuando el puntaje
// y la frecuencia tambien estan en [0, 1].
var (
	pesoSimilitud = decimal.RequireFromString("0.70")
	pesoHistoria  = decimal.RequireFromString("0.30")
)

// umbralDescarte y minEjemplosParaDescartar: una sola resolucion anterior que
// descarto no basta para sugerir lo mismo. Hacen falta al menos dos, y que
// sean la mayoria (60 %). Por debajo de eso se sugiere una candidata.
var (
	umbralDescarte           = decimal.RequireFromString("0.60")
	minEjemplosParaDescartar = 2
)

// EjemploEtiquetado es una resolucion manual ya hecha: que titulo era y que
// eligio la persona. Es el dato con el que el rankeador aprende, no una
// decision pendiente.
type EjemploEtiquetado struct {
	Clave    string
	Decision Decision
	ObraID   string
}

// PedidoTriage es lo que el rankeador necesita de UN caso: su titulo ya
// normalizado, las candidatas que propuso el difuso y las resoluciones
// anteriores. No lleva el uso ni el actor: rankear no resuelve.
type PedidoTriage struct {
	Clave      string
	Candidatos []Candidato
	Historia   []EjemploEtiquetado
}

// Sugerencia es una propuesta. Decision vale "asignar", "descartar" o
// [SugerenciaNinguna]. Orden son los ids de obra de mejor a peor ajuste; la
// bandeja original no se reescribe con el.
//
// Nada en este tipo se aplica solo. Quien llama lo muestra y una persona
// confirma o elige otra cosa (ADR 0007: la cola manual es el ultimo escalon,
// y una sugerencia no lo sustituye).
type Sugerencia struct {
	Decision  string
	ObraID    string
	Confianza decimal.Decimal
	Motivo    string
	Orden     []string
}

// AceptadaPor dice si la decision de la persona es la que se le propuso.
// Sirve para contar cuantas sugerencias se confirman y cuantas se cambian.
// "ninguna" nunca cuenta como confirmada: la persona decidio sin propuesta.
func (s Sugerencia) AceptadaPor(d Decision, obraID string) bool {
	switch d {
	case DecisionDescartar:
		return s.Decision == string(DecisionDescartar)
	case DecisionAsignar:
		return s.Decision == string(DecisionAsignar) && s.ObraID == obraID
	default:
		return false
	}
}

// ClaveDeTitulo deja un titulo listo para agrupar resoluciones del mismo
// caso: minusculas, sin tildes, y todo lo que no es letra o digito colapsado
// a un espacio. Se prefiere el titulo emitido; el original solo entra si el
// emitido viene vacio.
//
// No es titulo_normalizado() de Postgres (eso indexa el catalogo). Esta clave
// la calcula el nucleo para que el mismo titulo agrupen igual el caso de uso
// y la prueba, sin una base en el medio.
func ClaveDeTitulo(titulo, tituloOriginal string) string {
	base := strings.TrimSpace(titulo)
	if base == "" {
		base = strings.TrimSpace(tituloOriginal)
	}
	return normalizarTitulo(base)
}

func normalizarTitulo(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	espacio := true
	for _, r := range strings.ToLower(s) {
		r = sinAcento(r)
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			espacio = false
			continue
		}
		if !espacio {
			b.WriteByte(' ')
			espacio = true
		}
	}
	return strings.TrimSpace(b.String())
}

func sinAcento(r rune) rune {
	switch r {
	case 'á', 'à', 'ä', 'â', 'ã':
		return 'a'
	case 'é', 'è', 'ë', 'ê':
		return 'e'
	case 'í', 'ì', 'ï', 'î':
		return 'i'
	case 'ó', 'ò', 'ö', 'ô', 'õ':
		return 'o'
	case 'ú', 'ù', 'ü', 'û':
		return 'u'
	case 'ñ':
		return 'n'
	case 'ç':
		return 'c'
	default:
		return r
	}
}

type candidataAjustada struct {
	id       string
	puntaje  decimal.Decimal
	ajustado decimal.Decimal
}

// Rankear ordena las candidatas y propone una accion.
//
// El ajuste de cada candidata es
//
//	0.70 * similitud + 0.30 * (veces que se eligio esa obra / resoluciones del mismo titulo)
//
// Las resoluciones de OTRO titulo no cuentan: mezclarlas ensenaria al
// rankeador con casos que no se parecen. Una clave vacia tampoco agrupa,
// porque dos filas sin titulo no son la misma obra.
//
// Si la mayoria de esas resoluciones descartaron el caso, la propuesta es
// descartar, aunque haya una candidata con buen puntaje. Si no, se propone
// asignar la mejor del orden. Sin candidatas y sin esa mayoria, no hay
// propuesta.
//
// No escribe escalon, obra ni alias. Llamarla no resuelve el caso.
func Rankear(p PedidoTriage) Sugerencia {
	propias := historiaDelMismoTitulo(p.Clave, p.Historia)
	n := len(propias)
	descartes := 0
	porObra := map[string]int{}
	for _, e := range propias {
		if e.Decision == DecisionDescartar {
			descartes++
			continue
		}
		if e.ObraID != "" {
			porObra[e.ObraID]++
		}
	}

	filas := make([]candidataAjustada, 0, len(p.Candidatos))
	for _, c := range p.Candidatos {
		freq := decimal.Zero
		if n > 0 && porObra[c.ObraID] > 0 {
			freq = decimal.NewFromInt(int64(porObra[c.ObraID])).Div(decimal.NewFromInt(int64(n)))
		}
		filas = append(filas, candidataAjustada{
			id:       c.ObraID,
			puntaje:  c.Puntaje,
			ajustado: pesoSimilitud.Mul(c.Puntaje).Add(pesoHistoria.Mul(freq)),
		})
	}
	slices.SortStableFunc(filas, func(a, b candidataAjustada) int {
		if c := b.ajustado.Cmp(a.ajustado); c != 0 {
			return c
		}
		if c := b.puntaje.Cmp(a.puntaje); c != 0 {
			return c
		}
		return strings.Compare(a.id, b.id)
	})

	orden := make([]string, len(filas))
	for i, f := range filas {
		orden[i] = f.id
	}

	if n >= minEjemplosParaDescartar {
		tasa := decimal.NewFromInt(int64(descartes)).Div(decimal.NewFromInt(int64(n)))
		if tasa.Cmp(umbralDescarte) >= 0 {
			return Sugerencia{
				Decision:  string(DecisionDescartar),
				Confianza: tasa,
				Motivo: fmt.Sprintf(
					"%d de %d resoluciones anteriores de este titulo se descartaron",
					descartes, n),
				Orden: orden,
			}
		}
	}

	if len(filas) == 0 {
		return Sugerencia{
			Decision:  SugerenciaNinguna,
			Confianza: decimal.Zero,
			Motivo:    "no hay candidatas ni resoluciones anteriores suficientes para sugerir",
			Orden:     []string{},
		}
	}

	mejor := filas[0]
	confianza := mejor.ajustado
	if confianza.GreaterThan(decimal.NewFromInt(1)) {
		confianza = decimal.NewFromInt(1)
	}
	motivo := "sin resoluciones anteriores de este titulo: se sugiere la candidata de mayor similitud"
	if n > 0 {
		motivo = "la similitud se ajusto con resoluciones anteriores del mismo titulo"
	}
	return Sugerencia{
		Decision:  string(DecisionAsignar),
		ObraID:    mejor.id,
		Confianza: confianza,
		Motivo:    motivo,
		Orden:     orden,
	}
}

func historiaDelMismoTitulo(clave string, historia []EjemploEtiquetado) []EjemploEtiquetado {
	if clave == "" {
		return nil
	}
	var propias []EjemploEtiquetado
	for _, e := range historia {
		if e.Clave == clave {
			propias = append(propias, e)
		}
	}
	return propias
}
