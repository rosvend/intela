// Package recaudo factura a los usuarios segun tarifa y convenio, y produce
// la bolsa que se reparte en un periodo.
//
// Vacio a proposito: es andamiaje. El paquete existe para que la frontera
// este declarada desde el primer commit, no para que haya codigo.
//
// # La frontera
//
// Recaudo es EL UNICO modulo que conoce Usuario, Convenio y Tarifa. Aguas
// abajo solo circula una bolsa (ADR 0003). Reparto lee de Recaudo; Recaudo no
// conoce a Reparto, y depguard lo impide -la regla modulos-recaudo de
// .golangci.yml ya estaba escrita antes de que existiera este paquete.
//
// Ese corte es lo que hace estructuralmente imposible que un importe de
// factura acabe siendo un insumo de valorizacion.
//
// Reglamento aplicable: tarifas v6, y RD 11 para la determinacion de las
// asignaciones a distribuir.
package recaudo
