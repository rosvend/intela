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

// brutoMaximo es el importe mas grande que cabe en `bolsas.bruto`
// (NUMERIC(18,2)): 18 digitos en total menos 2 de escala son 16 enteros, o sea
// 9.999.999.999.999.999,99.
//
// Sin esta cota, un importe de 17 digitos enteros pasaba la comprobacion de
// escala -- tiene dos decimales, es positivo -- y lo rechazaba Postgres al
// insertar. El error salia como violacion de restriccion numerica y no como
// "este importe no cabe", que es lo que de verdad pasa. Mismo criterio que
// [precisionBruto]: lo que este constructor acepta ya cabe exacto en la
// columna.
var brutoMaximo = decimal.RequireFromString("9999999999999999.99")

// periodoValido es el patron de un periodo de recaudo: un ano, o un ano y un
// mes REAL.
//
// Mas estricto que el CHECK de las tablas `reportes`, `bolsas`, `procesos` y
// `cola_trabajos`, y que aplicacion.periodoValido, que usan `[0-9]{2}` para el
// mes y por tanto admiten `2025-00` y `2025-13`. Que el dominio sea mas
// estricto que el esquema es la direccion segura: el constructor es la puerta,
// y guardar dinero bajo un mes que no existe produce un periodo que no cuadra
// con ningun corte del reglamento (`R-34`) y que ningun reparto sabe cerrar.
//
// Los otros dos validadores tienen el mismo hueco y no se tocan aqui:
// estrecharlos pide una migracion sobre cuatro tablas y un repaso de la cola de
// trabajos, que no es el alcance de este PR.
//
// Duplicado a proposito, por lo que ya explica aplicacion.periodoValido: la
// base lo comprueba porque una fila mal formada no se puede permitir aunque
// la escriba otro cliente, y el nucleo porque rechazar un periodo invalido
// antes de la insercion es mas barato que leerlo en una violacion de CHECK.
var periodoValido = regexp.MustCompile(`^[0-9]{4}(-(0[1-9]|1[0-2]))?$`)

// ValidarPeriodo recorta un periodo y devuelve el normalizado, o
// [ErrBolsaInvalida].
//
// Exportada porque el FILTRO de lectura tiene que aceptar exactamente lo mismo
// que acepta [NuevaBolsa], y con dos copias del patron no lo hacia: la capa de
// aplicacion validaba con su propio `[0-9]{2}`, asi que `GET /bolsas?periodo=2025-13`
// respondia 200 con una lista vacia. Quien pregunta lee eso como "ese mes no
// tuvo recaudo", cuando lo que pasa es que ese mes no existe.
//
// Un filtro mas estricto que el constructor seria igual de malo por el otro
// lado: habria bolsas escribibles que no se pueden consultar. Una sola funcion
// para las dos cosas es lo que impide las dos derivas.
func ValidarPeriodo(periodo string) (string, error) {
	periodo = strings.TrimSpace(periodo)
	if !periodoValido.MatchString(periodo) {
		return "", fmt.Errorf("%w: periodo %q, se esperaba AAAA o AAAA-MM con un mes entre 01 y 12",
			ErrBolsaInvalida, periodo)
	}
	return periodo, nil
}

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

	periodo, err := ValidarPeriodo(periodo)
	if err != nil {
		return Bolsa{}, err
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
	if bruto.GreaterThan(brutoMaximo) {
		return Bolsa{}, fmt.Errorf("%w: el bruto %s no cabe en la columna, el maximo es %s",
			ErrBolsaInvalida, bruto.String(), brutoMaximo.String())
	}

	return Bolsa{UsuarioID: usuarioID, Periodo: periodo, Circuito: circuito, Bruto: bruto}, nil
}
