// Package liquidacion convierte las lineas de titular de una corrida en
// ordenes de pago, aplica retenciones y anticipos pendientes, y registra el
// pago.
//
// # La frontera
//
// Liquidacion consume el resultado de Reparto; no lo recalcula. Si una cifra
// aqui no coincide con la de la corrida, la corrida manda.
//
// Es el ultimo punto donde se comprueba R-01 (solo escritor persona natural
// recibe orden de pago, RD 4.5) antes de que el dinero salga, y donde se
// aplican las prescripciones de RD 15.
//
// # El reporte
//
// Las deducciones (R-06, R-07) se restan de la bolsa ANTES de partir por
// obra. [Prorratear] solo deshace esa resta sobre la linea del titular para
// que el reporte pueda mostrar bruto, cada deduccion y neto por obra.
package liquidacion
