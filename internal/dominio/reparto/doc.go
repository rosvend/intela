// Package reparto valoriza los usos de un periodo y reparte la bolsa neta
// entre las obras y, a traves de las declaraciones, entre sus titulares.
//
// # Motor
//
// [Reparto] es una funcion pura (ADR 0005): recibe bolsa, usos, snapshot y
// declaraciones; no hace E/S, no lee el reloj y no usa aleatoriedad. Los
// recorridos van sobre claves ordenadas de forma estable.
//
// # Regla de redondeo
//
// Los importes en dinero se redondean a dos decimales con Round
// half-away-from-zero (shopspring/decimal.Round). El residuo de una
// asignacion proporcional (neto - suma de lineas) queda en
// [Resultado.Residuo]; nunca se absorbe en la ultima linea. Importes enteros
// que el reglamento no reparte (grupo vacio, exclusion R-27, peso cero) van
// a [Resultado.NoDistribuido], no a Residuo.
//
// # Modalidades
//
//   - TV (`RD 9.1.1`): puntos = ponderacion * duracion * rating * emisiones.
//   - Cine / Teatro (`RD 9.2` / `RD 9.3`): proporcional a espectadores o
//     taquilla segun [Snapshot.BaseCineTeatro] (P-18).
//   - Transporte (`RD 9.4`): proporcional a exhibiciones.
//   - OTT (`RD 9.7`): Pi = PB*Wa + DU*Wb + V*Wc.
//   - Suscripcion / Hotel (`RD 9.5` / `RD 9.6`): excluye fuera de repertorio
//     (R-27) antes del split; reparte el neto  por porcentajes de grupo del
//     snapshot; aplica 9.1.1 dentro de cada grupo con valor punto propio.
//   - [AsignarPlataformaTerceros]: bolsa derivada al % del snapshot, con
//     deducciones y declaraciones, en proporciones del padre restringidas
//     a las obras comunicadas en la plataforma (parrafo final de `RD 9.7`).
//
// Los invariantes de [Uso] sin dinero, R-04 (retencion total) y R-01
// (solo IPI en lineas de titular) se mantienen.
package reparto
