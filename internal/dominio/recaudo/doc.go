// Package recaudo registra lo que cada usuario pago en un periodo y produce
// la bolsa que se reparte.
//
// # Recibe, no factura
//
// Intela NO liquida tarifas y NO emite cuentas de cobro. Recibe el importe ya
// cobrado, por usuario y periodo (P-08 en docs/dominio/preguntas-cliente.md).
// La tabla `T-01` a `T-11` del Reglamento de Tarifas es documentacion de
// referencia -queda en docs/dominio/reglas-negocio.md-, no logica de este
// paquete: aqui no hay porcentaje del 4%, ni tarifa por plaza, ni recargo por
// incumplimiento, ni conversion de TRM.
//
// La consecuencia practica es que [Convenio] y [Tarifa] no son tipos. Lo que
// queda de ellos es la PROCEDENCIA de cada bolsa -que convenio, que tarifa,
// que factura la originaron-, que viaja como texto en la fila y responde la
// pregunta 1 del ADR 0006: de donde salio este dinero.
//
// # El dinero no llega por fila
//
// Ningun reporte de uso trae importes. El flujo es: REDES SGC cobra una bolsa
// a cada usuario, y los reportes de uso solo PONDERAN como se reparte esa
// bolsa entre las obras (docs/dominio/formulas.md). Es un asignador de bolsa,
// no un atribuidor de ingresos transaccionales. Por eso [Bolsa] es lo unico
// que sale de aqui y por eso reparto.Uso no tiene campo de dinero.
//
// # La frontera
//
// Recaudo es EL UNICO modulo que conoce [Usuario] -el pagador, no la cuenta
// que inicia sesion-. Aguas abajo solo circula una bolsa (ADR 0003). Reparto
// lee de Recaudo; Recaudo no conoce a Reparto, y depguard lo impide -la regla
// modulos-recaudo de .golangci.yml ya estaba escrita antes de que existiera
// este paquete-.
//
// Ese corte es lo que hace estructuralmente imposible que un importe de
// factura acabe siendo un insumo de valorizacion.
//
// [Bolsa] y [Circuito] vivian en internal/dominio/reparto. Se mudaron aqui
// porque es este modulo el que las produce; `reparto` las nombra ahora por un
// alias de tipo, de forma que el motor de #33 las sigue usando igual.
//
// Reglamento aplicable: tarifas v6, y RD 11 para la determinacion de las
// asignaciones a distribuir.
package recaudo
