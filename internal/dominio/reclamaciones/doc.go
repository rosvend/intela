// Package reclamaciones tramita las disputas sobre una cifra ya liquidada:
// quien reclama, sobre que corrida, y con que resolucion.
//
// Vacio a proposito: es andamiaje.
//
// # La frontera
//
// Una reclamacion NO reescribe una corrida cerrada. Se resuelve con un ajuste
// trazable en una corrida posterior, o por la reserva para correccion de
// errores tecnicos (RD 14). Reabrir una corrida cerrada rompe la
// reproducibilidad que exige el ADR 0005.
//
// Es el modulo que mas depende de la bitacora: para responder una reclamacion
// hay que poder explicar la cifra hasta su origen -fuente, reporte y regla
// aplicada- que es justo lo que pide RD 16.
package reclamaciones
