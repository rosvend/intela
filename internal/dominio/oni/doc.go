// Package oni gestiona la cola de Obras No Identificadas: lo que la cascada
// de identificacion no reconocio con certeza suficiente y espera resolucion
// manual.
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
//
// # Listado publico (R-18)
//
// La proyeccion publica lleva titulo e informacion identificatoria y NUNCA
// montos: RD 13.8.2 lo prohibe y el ADR 0006 lo convierte en invariante de
// tipo. La informacion economica vive en el asiento interno; la vista publica
// no puede empezar a exponerla por descuido. La fecha de publicacion se
// registra una sola vez: es el ancla de los tres anos de R-19, y reescribirla
// resetearia el reloj.
package oni
