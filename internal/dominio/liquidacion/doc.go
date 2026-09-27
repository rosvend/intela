// Package liquidacion convierte las lineas de titular de una corrida en
// ordenes de pago, aplica retenciones y anticipos pendientes, y registra el
// pago.
//
// # La frontera
//
// Liquidacion consume el resultado de Reparto; no lo recalcula. Del motor
// llega el neto POR TITULAR, y de la corrida el bruto y las tres deducciones;
// bruto y desglose de cada orden se reconstruyen de ahi con [Prorratear], asi
// que no hay dos cifras que puedan discrepar y entre las que haya que elegir.
// El residuo de redondeo del prorrateo es explicito ([ResiduoProrrateo],
// ADR 0005) y quien liquida lo registra en el asiento de emision.
// Por eso este paquete no importa recaudo (la bolsa ya se resolvio) ni
// identificacion (las obras ya estan identificadas).
//
// # Lo que este paquete NO comprueba
//
// R-01 (solo un escritor persona natural recibe orden de pago, RD 4.5) NO se
// re-comprueba aqui. Se hace cumplir aguas arriba, y en dos sitios: el nucleo
// lo rechaza al guardar la declaracion (aplicacion.ErrTitularNoEsPersonaNatural)
// y el trigger `resultados_titular_persona_natural` de la migracion 00001 lo
// cierra en la base al escribir las lineas de la corrida. Una linea de titular
// que llega hasta aqui ya paso las dos, y volver a mirarlo con un padron que
// este paquete no tiene seria una tercera definicion de la regla.
//
// RD 15 (prescripciones de 3 y 10 anos) tampoco vive aqui: esta en
// dominio/prescripcion.
//
// # Lo que RD 13.2 exige ver
//
// Cada [OrdenDePago] muestra bruto, cada deduccion y neto por separado. No
// se colapsan en el neto: OE-4 y OE-6 piden el desglose en cada consulta y
// en cada reporte.
//
// # Una orden por titular, periodo y circuito
//
// No una por corrida (ADR 0019). Un periodo se cierra con tantas corridas como
// bolsas tenga el circuito, y el umbral de menor cuantia de R-11 se mide sobre
// lo que el titular cobra POR EL PERIODO: partirlo por corrida diferiria
// saldos que juntos si pasan el 2% de un SMMLV.
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
