// Package reparto valoriza los usos de un periodo y reparte la bolsa neta
// entre las obras y, a traves de las declaraciones, entre sus titulares.
//
// # Que hay aqui y que no
//
// Este paquete es andamiaje: declara el vocabulario del modulo -modalidades,
// etapas del RD 13.5, circuitos, y las lineas de un resultado- y nada mas.
// El motor de valorizacion y la maquina de estados del proceso entran en PRs
// propios. Ver docs/decisiones/0005 y 0008.
//
// # Lo que el motor tendra que respetar
//
// Cuatro invariantes que no fallan con una excepcion: producen un numero, el
// numero se paga, y aparece en una auditoria de RD 16 anos despues.
//
//   - No se suman importes por fila. [Uso] no tiene campo de dinero, asi que
//     la operacion de sumar importes por reporte sencillamente no existe: los
//     reportes ponderan la bolsa, no la aportan.
//   - Ningun camino de tipos lleva de una parrilla a un porcentaje de pago.
//     Las columnas de autoria de un reporte son evidencia de matching, jamas
//     insumo de reparto (R-02, R-03).
//   - Si lo declarado no suma 100%, se retiene el total en reserva. No hay
//     reparto parcial (R-04, RD 13.1.3). Retener es mover a reserva, no
//     descontar: la suma de neto tiene que cerrar contra lo repartido mas lo
//     retenido mas el residuo de redondeo.
//   - No existe firma que emita orden de pago a quien no sea escritor persona
//     natural (R-01, RD 4.5).
//
// # Deuda declarada del PR del motor
//
// El ADR 0005 exige que las pruebas de reproducibilidad con los ejemplos
// numericos del propio reglamento -el canal Z de RD 9.1.1, las peliculas X e
// Y de RD 9.2- esten desde el primer commit del motor, no despues. Ese PR
// empieza por esas pruebas.
//
// El ADR 0008 pide dos maquinas de estado distintas, una por circuito, no una
// con un condicional: el internacional no valoriza por puntos (RD 7.4) y
// "Fees in Error" no pasa por deducciones (R-16, RD 13.7). Por eso aqui se
// declaran las etapas de ambos recorridos pero no el agregado Proceso: el
// tipo lo fija el PR que traiga las transiciones.
package reparto
