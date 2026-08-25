// Package liquidacion convierte las lineas de titular de una corrida en
// ordenes de pago, aplica retenciones y anticipos pendientes, y registra el
// pago.
//
// Vacio a proposito: es andamiaje.
//
// # La frontera
//
// Liquidacion consume el resultado de Reparto; no lo recalcula. Si una cifra
// aqui no coincide con la de la corrida, la corrida manda.
//
// Es el ultimo punto donde se comprueba R-01 (solo escritor persona natural
// recibe orden de pago, RD 4.5) antes de que el dinero salga, y donde se
// aplican las prescripciones de RD 15.
package liquidacion
