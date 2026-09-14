package recaudo

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrBolsaInvalida es el unico centinela que emite [NuevaBolsa].
//
// Uno solo, igual que [ErrUsuarioInvalido] y que repertorio.ErrObraInvalida:
// quien llama necesita separar "lo que me mandaron no forma una bolsa" -un
// 400- de "la base no responde" -un 500-, y para eso basta un centinela. El
// detalle de que falta va en el texto envuelto, que lo lee una persona.
var ErrBolsaInvalida = errors.New("bolsa invalida")

// Circuito de la corrida. Son dos recorridos distintos, no una variante de
// uno: el internacional no valoriza por puntos (RD 7.4). Ver ADR 0008.
//
// No es una etiqueta sobre el mismo dinero. `RD 10.3` (R-35) los invierte por
// separado para poder saber a que tipo de reparto corresponden sus
// rendimientos, y de ahi salen tres diferencias de calculo aguas abajo: la
// reserva del 5% aplica solo al nacional (R-07, `RD 14.5.4`), los Fees in
// Error vuelven integros (R-16, `RD 13.7`) y las fechas de corte no coinciden
// -20 de octubre y 30 de septiembre, R-34-.
type Circuito string

const (
	Nacional      Circuito = "nacional"
	Internacional Circuito = "internacional"
)

// Circuitos devuelve los circuitos validos, en el orden en que los declara el
// CHECK de la columna `bolsas.circuito`.
func Circuitos() []Circuito {
	return []Circuito{Nacional, Internacional}
}

// precisionBruto es la escala de la columna `bolsas.bruto` (NUMERIC(18,2)):
// dos decimales, que son los centavos del peso.
//
// [NuevaBolsa] RECHAZA -no redondea- un importe con mas precision que esta,
// por el mismo motivo que repertorio.precisionPorcentaje: redondear en el
// dominio obligaria a que el modo de redondeo de shopspring/decimal coincida
// bit a bit con el de Postgres al truncar a la escala de la columna. Lo que
// este validador acepta ya cabe exacto en la columna, y la cifra que el
// dominio valido es la misma que la base guarda.
//
// Aqui ademas mueve dinero de terceros: una bolsa validada como $100,555 que
// se guarda como $100,56 es medio centavo que el reparto distribuye y que no
// se recaudo.
const precisionBruto = 2

// periodoValido es el mismo patron que el CHECK de las tablas `reportes`,
// `bolsas`, `procesos` y `cola_trabajos`, y el mismo que aplicacion valida al
// encolar un trabajo.
//
// Duplicado a proposito, por lo que ya explica aplicacion.periodoValido: la
// base lo comprueba porque una fila mal formada no se puede permitir aunque
// la escriba otro cliente, y el nucleo porque rechazar un periodo invalido
// antes de la insercion es mas barato que leerlo en una violacion de CHECK.
var periodoValido = regexp.MustCompile(`^[0-9]{4}(-[0-9]{2})?$`)

// Bolsa a repartir en un periodo. Es lo unico que Recaudo pasa aguas abajo:
// Reparto no conoce Usuario, Convenio ni Tarifa (ADR 0003).
//
// Vivia en internal/dominio/reparto. Se mudo aqui porque es este modulo el
// que la produce, y porque la regla de ADR 0003 se lee al reves de como
// estaba: el consumidor no puede ser el dueno del tipo que le entregan.
// `reparto` la sigue nombrando por un alias, asi que nada aguas abajo cambio.
//
// Struct plano con constructor validador -no entidad con campos privados-
// igual que repertorio.Declaracion: el motor de reparto la lee campo a campo
// como dato de entrada de una funcion pura.
type Bolsa struct {
	UsuarioID string
	Periodo   string
	Circuito  Circuito
	Bruto     decimal.Decimal
}

// NuevaBolsa valida lo cobrado por un usuario en un periodo antes de que
// llegue a persistirse.
//
// No calcula nada. Bajo P-08 Intela RECIBE el importe ya cobrado: no liquida
// tarifas ni factura, asi que aqui no hay tarifa que aplicar ni convenio que
// resolver -la tabla `T-01` a `T-11` es documentacion de referencia-. Lo que
// este constructor hace es impedir que entre una bolsa que el reparto no
// pueda tratar.
//
// Un bruto de cero se acepta: que un usuario no pagara nada en el periodo es
// un hecho del negocio, y borrar la fila lo volveria indistinguible de "no se
// ha cargado todavia".
func NuevaBolsa(usuarioID, periodo string, circuito Circuito, bruto decimal.Decimal) (Bolsa, error) {
	usuarioID = strings.TrimSpace(usuarioID)
	if usuarioID == "" {
		return Bolsa{}, fmt.Errorf("%w: falta el usuario que pago", ErrBolsaInvalida)
	}

	periodo = strings.TrimSpace(periodo)
	if !periodoValido.MatchString(periodo) {
		return Bolsa{}, fmt.Errorf("%w: periodo %q, se esperaba AAAA o AAAA-MM",
			ErrBolsaInvalida, periodo)
	}

	if !slices.Contains(Circuitos(), circuito) {
		return Bolsa{}, fmt.Errorf("%w: circuito %q, se esperaba uno de %v",
			ErrBolsaInvalida, circuito, Circuitos())
	}

	if bruto.IsNegative() {
		return Bolsa{}, fmt.Errorf("%w: el bruto es %s y no puede ser negativo",
			ErrBolsaInvalida, bruto.String())
	}
	if !bruto.Equal(bruto.Round(precisionBruto)) {
		return Bolsa{}, fmt.Errorf("%w: el bruto %s admite hasta %d decimales",
			ErrBolsaInvalida, bruto.String(), precisionBruto)
	}

	return Bolsa{UsuarioID: usuarioID, Periodo: periodo, Circuito: circuito, Bruto: bruto}, nil
}
