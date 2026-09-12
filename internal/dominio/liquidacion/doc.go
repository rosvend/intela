// Package liquidacion convierte las lineas de titular de una corrida en
// ordenes de pago, aplica retenciones y anticipos pendientes, y registra el
// pago.
//
// # La frontera
//
// Liquidacion consume el resultado de Reparto; no lo recalcula. Si una cifra
// aqui no coincide con la de la corrida, la corrida manda. Por eso este
// paquete no importa recaudo (la bolsa ya se resolvio) ni identificacion
// (las obras ya estan identificadas).
//
// Es el ultimo punto donde se comprueba R-01 (solo escritor persona natural
// recibe orden de pago, RD 4.5) antes de que el dinero salga, y donde se
// aplican las prescripciones de RD 15.
//
// # Lo que RD 13.2 exige ver
//
// Cada [OrdenDePago] muestra bruto, cada deduccion y neto por separado. No
// se colapsan en el neto: OE-4 y OE-6 piden el desglose en cada consulta y
// en cada reporte.
//
// # Plazos
//
// El dominio no lee el reloj (depguard deniega `time`). Recibe el dia civil
// como `YYYY-MM-DD`, que aplicacion obtiene de PuertoReloj:
//
//   - R-10 / RD 13.2: sin respuesta a los 15 dias calendario, la liquidacion
//     se entiende aceptada.
//   - R-11 / RD 13.3: si el neto no supera el 2% de un SMMLV y no hay
//     respuesta, el monto se arrastra al siguiente periodo.
//
// R-12 (RUT y certificacion bancaria) no impide liquidar: impide pagar.
package liquidacion
