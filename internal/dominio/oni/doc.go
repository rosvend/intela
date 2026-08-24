// Package oni gestiona la cola de Obras No Identificadas: lo que la cascada
// de identificacion no reconocio con certeza suficiente y espera resolucion
// manual.
//
// Vacio a proposito: es andamiaje.
//
// # Que es y que no es
//
// ONI no es un fallo del sistema, es una etapa del diseno (ADR 0007). Un uso
// en ONI es dinero que todavia no tiene dueno conocido, no dinero perdido: se
// retiene hasta que alguien lo resuelve o hasta que prescribe (RD 15).
//
// Resolver un ONI a mano ensena un alias, y ese alias hace que el mismo
// titulo de la misma fuente ya no vuelva a caer aqui. Por eso la cola tiene
// que encogerse con el uso; si no encoge, el escalon de alias no esta
// funcionando.
//
// Quien resuelve un ONI y con que evidencia queda en la bitacora: es una
// decision humana sobre a quien se le paga.
package oni
