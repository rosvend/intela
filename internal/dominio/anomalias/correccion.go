package anomalias

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rosvend/intela/internal/dominio/identificacion"
	"github.com/rosvend/intela/internal/dominio/repertorio"
)

// Acciones correctivas de una alerta critica (#164, ADR 0021).
//
// Resolver una critica sin corregir el dato dejaba pagar el doble conteo: la
// alerta se cerraba y la fila duplicada seguia ponderando. Desde #164 cerrar
// una critica exige decir que se hizo con el dato, y la accion es la que lo
// hace en la misma unidad que el cierre.
const (
	// AccionExcluirUso saca del reparto una fila de un duplicado de registro:
	// la de la alerta o la otra copia, la que NO manda.
	AccionExcluirUso = "excluir_uso"
	// AccionExcluirEntrega saca del reparto todas las filas de una entrega que
	// repite los bytes de otra.
	AccionExcluirEntrega = "excluir_entrega"
	// AccionAsignarTipoObra pone la categoria de RD 9.1.1 a la fila que no la trae.
	AccionAsignarTipoObra = "asignar_tipo_obra"
	// AccionAceptarTalCual cierra un duplicado que una persona juzga falso
	// positivo (P-21) SIN tocar el dato. La compuerta lo deja pasar y lo cuenta
	// aparte, para que la corrida diga con cuantas criticas aceptadas se calculo.
	AccionAceptarTalCual = "aceptar_tal_cual"
)

// Acciones devuelve las cuatro, en orden fijo.
func Acciones() []string {
	return []string{AccionExcluirUso, AccionExcluirEntrega, AccionAsignarTipoObra, AccionAceptarTalCual}
}

// AccionesDe devuelve las acciones que admite un tipo de alerta. Vacio para
// los tipos que no son criticos: su cierre es solo una nota.
//
// `tipo_obra_sin_mapear` no admite aceptar tal cual: aceptada, la fila sigue
// sin tipo y el motor sigue abortando la corrida (ErrRepartoInvalido), asi
// que la aceptacion abriria una compuerta que no lleva a ningun sitio.
func AccionesDe(tipo string) []string {
	switch tipo {
	case TipoDuplicadoRegistro:
		return []string{AccionExcluirUso, AccionAceptarTalCual}
	case TipoDuplicadoArchivo:
		return []string{AccionExcluirEntrega, AccionAceptarTalCual}
	case TipoTipoObraSinMapear:
		return []string{AccionAsignarTipoObra}
	default:
		return nil
	}
}

// MaxNotaCorreccion es el tope de la nota de un cierre con accion, en runas.
// Es el de la resolucion manual porque la nota viaja a `usos.nota_resolucion`,
// que tiene ese tope por CHECK (migracion 00022).
const MaxNotaCorreccion = identificacion.MaxNotaResolucion

var (
	// ErrAccionInvalida: el pedido no cuadra con la alerta -- una critica sin
	// accion, una accion que el tipo no admite, un objetivo que no repite el
	// registro de la alerta --. Es un 400: el cuerpo esta mal.
	ErrAccionInvalida = errors.New("accion correctiva invalida")

	// ErrAccionNoAplica: el pedido tiene buena forma pero el dato ya no esta
	// como la alerta lo describe -- la fila ya no pondera, la entrega ya esta
	// excluida, el tipo ya esta puesto -- o aplicarla dejaria el hecho sin
	// contar. Es un 409: hay que reevaluar el periodo y mirar otra vez.
	ErrAccionNoAplica = errors.New("la accion correctiva ya no aplica")

	// ErrNotaDemasiadoLarga: la nota de un cierre con accion pasa de [MaxNotaCorreccion].
	ErrNotaDemasiadoLarga = fmt.Errorf("la nota de una correccion no puede pasar de %d caracteres", MaxNotaCorreccion)
)

// PedidoDeCorreccion es lo que llega del cuerpo al cerrar una alerta, crudo.
type PedidoDeCorreccion struct {
	Accion string
	// UsoID solo con excluir_uso: la fila a excluir. Vacio = la de la alerta.
	UsoID string
	// ReporteID solo con excluir_entrega: la entrega a excluir. Vacio = la de la alerta.
	ReporteID string
	// TipoObra solo con asignar_tipo_obra, y obligatorio.
	TipoObra string
}

// Correccion es la accion ya decidida para una alerta concreta.
type Correccion struct {
	Accion string
	// Objetivo es el registro sobre el que actua: el id del uso, el id de la
	// entrega o el tipo asignado. Vacio solo con aceptar tal cual, o sin accion.
	Objetivo string
}

// ValidarForma comprueba el pedido sin mirar la alerta: accion conocida y
// solo los campos que esa accion lee. Va antes de abrir la unidad, para no
// tomar el cerrojo del periodo por un cuerpo que se va a rechazar igual.
//
// Devuelve el pedido recortado, con el tipo en minusculas como lo guarda la
// ingesta (`normalizacion`).
func ValidarForma(p PedidoDeCorreccion) (PedidoDeCorreccion, error) {
	p = PedidoDeCorreccion{
		Accion:    strings.TrimSpace(p.Accion),
		UsoID:     strings.TrimSpace(p.UsoID),
		ReporteID: strings.TrimSpace(p.ReporteID),
		TipoObra:  strings.ToLower(strings.TrimSpace(p.TipoObra)),
	}

	sobran := func(campos ...string) error {
		for _, c := range campos {
			var valor string
			switch c {
			case "uso_id":
				valor = p.UsoID
			case "reporte_id":
				valor = p.ReporteID
			case "tipo_obra":
				valor = p.TipoObra
			}
			if valor != "" {
				return fmt.Errorf("%w: %q no admite %s", ErrAccionInvalida, p.Accion, c)
			}
		}
		return nil
	}

	switch p.Accion {
	case "", AccionAceptarTalCual:
		if err := sobran("uso_id", "reporte_id", "tipo_obra"); err != nil {
			if p.Accion == "" {
				return PedidoDeCorreccion{}, fmt.Errorf("%w: uso_id, reporte_id y tipo_obra solo van con una accion", ErrAccionInvalida)
			}
			return PedidoDeCorreccion{}, err
		}
	case AccionExcluirUso:
		if err := sobran("reporte_id", "tipo_obra"); err != nil {
			return PedidoDeCorreccion{}, err
		}
	case AccionExcluirEntrega:
		if err := sobran("uso_id", "tipo_obra"); err != nil {
			return PedidoDeCorreccion{}, err
		}
	case AccionAsignarTipoObra:
		if err := sobran("uso_id", "reporte_id"); err != nil {
			return PedidoDeCorreccion{}, err
		}
		if !slices.Contains(repertorio.TiposObra(), repertorio.TipoObra(p.TipoObra)) {
			return PedidoDeCorreccion{}, fmt.Errorf("%w: tipo_obra %q, se esperaba uno de %v (RD 9.1.1)",
				ErrAccionInvalida, p.TipoObra, repertorio.TiposObra())
		}
	default:
		return PedidoDeCorreccion{}, fmt.Errorf("%w: %q, se esperaba una de %v", ErrAccionInvalida, p.Accion, Acciones())
	}
	return p, nil
}

// CorreccionPara decide la correccion de una alerta de tipo `tipo` cuyo
// registro ofensor es `refID`, a partir de un pedido ya pasado por
// [ValidarForma].
//
// Una critica sin accion se rechaza: es exactamente el cierre que dejaba
// pagar el doble conteo. Una no critica con accion tambien: su anomalia no
// tiene dato que corregir desde aqui (la declaracion se corrige en el
// catalogo, el ONI en la bandeja de identificacion).
//
// El objetivo por defecto es el registro de la alerta. Que sea OTRO -- la otra
// copia de un duplicado, la que no manda -- lo comprueban
// [ValidarExclusionDeUso] y [ValidarExclusionDeEntrega] contra la foto del
// periodo.
func CorreccionPara(tipo, refID string, p PedidoDeCorreccion) (Correccion, error) {
	admitidas := AccionesDe(tipo)
	if len(admitidas) == 0 {
		if p.Accion != "" {
			return Correccion{}, fmt.Errorf("%w: una alerta %q no es critica y se cierra solo con la nota", ErrAccionInvalida, tipo)
		}
		return Correccion{}, nil
	}
	if p.Accion == "" {
		return Correccion{}, fmt.Errorf("%w: una alerta %q es critica y cerrarla exige una accion (%s): cerrarla sin tocar el dato dejaria pagar la anomalia",
			ErrAccionInvalida, tipo, strings.Join(admitidas, ", "))
	}
	if !slices.Contains(admitidas, p.Accion) {
		return Correccion{}, fmt.Errorf("%w: una alerta %q admite %s, no %q",
			ErrAccionInvalida, tipo, strings.Join(admitidas, ", "), p.Accion)
	}

	c := Correccion{Accion: p.Accion}
	switch p.Accion {
	case AccionExcluirUso:
		c.Objetivo = cmp.Or(p.UsoID, refID)
	case AccionExcluirEntrega:
		c.Objetivo = cmp.Or(p.ReporteID, refID)
	case AccionAsignarTipoObra:
		c.Objetivo = p.TipoObra
	}
	return c, nil
}

// ValidarNota aplica el tope de [MaxNotaCorreccion] a la nota ya recortada de
// un cierre con accion. Sin accion la nota no tiene tope aqui: no viaja a `usos`.
func ValidarNota(c Correccion, nota string) error {
	if c.Accion == "" || c.Accion == AccionAceptarTalCual {
		return nil
	}
	if utf8.RuneCountInString(nota) > MaxNotaCorreccion {
		return ErrNotaDemasiadoLarga
	}
	return nil
}

// SigueEnJuego dice si una fila puede llegar a ponderar: todo escalon menos
// las dos decisiones humanas que la sacan para siempre ('descartado' y
// 'duplicado'). 'excluido' si sigue: la exclusion R-27 es configuracion y la
// cascada la reevalua en cada corrida.
func SigueEnJuego(escalon string) bool {
	return escalon != identificacion.EscalonDescartado && escalon != identificacion.EscalonDuplicado
}

// ValidarExclusionDeUso comprueba, sobre la foto del periodo, que excluir la
// fila `objetivoID` corrige el duplicado de registro que levanto la alerta
// sobre `refID` y deja el hecho contado UNA vez.
//
// El objetivo tiene que repetir el registro de la alerta (misma fuente, misma
// clave), seguir en juego, y dejar al menos otra copia en juego: excluir la
// unica que queda borraria la emision del reparto en vez de desduplicarla.
func ValidarExclusionDeUso(usos []Uso, refID, objetivoID string) (Uso, error) {
	ref, hay := usoPorID(usos, refID)
	if !hay {
		return Uso{}, fmt.Errorf("%w: el uso %q de la alerta ya no esta en el periodo", ErrAccionNoAplica, refID)
	}
	if ref.ClaveRegistro == "" {
		return Uso{}, fmt.Errorf("%w: el uso %q ya no tiene clave de registro que comparar", ErrAccionNoAplica, refID)
	}
	objetivo, hay := usoPorID(usos, objetivoID)
	if !hay {
		return Uso{}, fmt.Errorf("%w: el uso %q no es una fila de este periodo", ErrAccionInvalida, objetivoID)
	}
	if objetivo.Fuente != ref.Fuente || objetivo.ClaveRegistro != ref.ClaveRegistro {
		return Uso{}, fmt.Errorf("%w: el uso %q no repite el registro %s de la fuente %q",
			ErrAccionInvalida, objetivoID, ref.ClaveRegistro, ref.Fuente)
	}
	if !SigueEnJuego(objetivo.Escalon) {
		return Uso{}, fmt.Errorf("%w: el uso %q ya no pondera (escalon %q); reevalue el periodo",
			ErrAccionNoAplica, objetivoID, objetivo.Escalon)
	}
	for _, u := range usos {
		if u.ID != objetivo.ID && u.Fuente == ref.Fuente && u.ClaveRegistro == ref.ClaveRegistro && SigueEnJuego(u.Escalon) {
			return objetivo, nil
		}
	}
	return Uso{}, fmt.Errorf("%w: el uso %q es la unica fila que queda con el registro %s: excluirla dejaria el hecho sin contar",
		ErrAccionNoAplica, objetivoID, ref.ClaveRegistro)
}

// ValidarExclusionDeEntrega comprueba que excluir la entrega `objetivoID`
// corrige el duplicado de huella que levanto la alerta sobre `refID`.
//
// El objetivo tiene que ser del periodo de la alerta -- el cerrojo que
// serializa la correccion es el de ese periodo, y tocar otro mes se colaria
// entre su compuerta y su calculo --, compartir la huella, no estar excluido
// ya, y dejar otra entrega con esos bytes sin excluir.
func ValidarExclusionDeEntrega(entregas []Entrega, periodo, refID, objetivoID string) (Entrega, error) {
	ref, hay := entregaPorID(entregas, refID)
	if !hay {
		return Entrega{}, fmt.Errorf("%w: la entrega %q de la alerta ya no existe", ErrAccionNoAplica, refID)
	}
	objetivo, hay := entregaPorID(entregas, objetivoID)
	if !hay {
		return Entrega{}, fmt.Errorf("%w: la entrega %q no existe", ErrAccionInvalida, objetivoID)
	}
	if objetivo.SHA256 != ref.SHA256 {
		return Entrega{}, fmt.Errorf("%w: la entrega %q no trae los mismos bytes que %q", ErrAccionInvalida, objetivoID, refID)
	}
	if objetivo.Periodo != periodo {
		return Entrega{}, fmt.Errorf("%w: la entrega %q es del periodo %q y la alerta del %q; se excluye desde una alerta de su periodo",
			ErrAccionInvalida, objetivoID, objetivo.Periodo, periodo)
	}
	if objetivo.Excluida {
		return Entrega{}, fmt.Errorf("%w: la entrega %q ya esta excluida; reevalue el periodo", ErrAccionNoAplica, objetivoID)
	}
	for _, e := range entregas {
		if e.ID != objetivo.ID && e.SHA256 == ref.SHA256 && !e.Excluida {
			return objetivo, nil
		}
	}
	return Entrega{}, fmt.Errorf("%w: la entrega %q es la unica con esos bytes que sigue en el reparto: excluirla dejaria sus filas sin contar",
		ErrAccionNoAplica, objetivoID)
}

func usoPorID(usos []Uso, id string) (Uso, bool) {
	for _, u := range usos {
		if u.ID == id {
			return u, true
		}
	}
	return Uso{}, false
}

func entregaPorID(entregas []Entrega, id string) (Entrega, bool) {
	for _, e := range entregas {
		if e.ID == id {
			return e, true
		}
	}
	return Entrega{}, false
}
