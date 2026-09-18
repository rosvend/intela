# 0019 Una corrida por bolsa; la liquidacion agrega por periodo

Fecha: 2026-09-18
Estado: Vigente

## Contexto

`procesos.bolsa_id` es una sola referencia a `bolsas(id)`, y `bolsas` es unica por
`(usuario_id, periodo, circuito)`. Un periodo con N pagadores son N bolsas y, con
el modelo actual, N procesos. Eso encaja con `RD 9.1`: cada canal se reparte
**de forma independiente** conforme al pago que hizo, y el valor punto de
`RD 9.1.1` es por canal.

Choca con la liquidacion. `RD 13.2` describe **un** documento por titular con
montos brutos, deducciones y netos, y una sola ventana de silencio de quince dias.
`RD 13.3` retiene cualquier importe menor o igual al **2% de un SMMLV** y solo lo
paga cuando, acumulado, sobrepasa el umbral. Si cada proceso emite su propia orden
por titular, casi cada participacion por canal cae bajo el umbral y se aplaza
indefinidamente, aunque el agregado del periodo hubiera pagado de inmediato.

La issue #119 pide que esta decision se escriba donde se fija la forma del uso
(atribucion de canal). #34 y #36 la consumen.

## Decision

**Una corrida de valorizacion = una bolsa = un proceso.** El motor de reparto
(`internal/dominio/reparto`) recibe una bolsa y los usos que la ponderan; el
valor punto de television se calcula dentro de esa corrida. Dos canales en el
mismo periodo son dos bolsas y dos procesos, y producen dos valor punto
independientes.

**La liquidacion agrega por titular y periodo antes del umbral.** Quien implemente
`GenerarLiquidacion` (#36) suma las lineas de titular de todos los procesos del
mismo periodo (y circuito) antes de aplicar el 2% de SMMLV y antes de emitir el
documento unico de `RD 13.2`. El proceso de aprobaciones sigue siendo por corrida;
el pago es por titular-periodo.

## Alternativas consideradas

**Una corrida que abarca todas las bolsas del periodo.** Descartada: forzaria al
motor a inventar un valor punto multi-canal o a reintroducir la independencia de
`RD 9.1` como un bucle interno, y mezclarian firmas de etapas que hoy cuelgan de
un solo `procesos.bolsa_id`.

**Liquidar por proceso sin agregar.** Descartada: convierte el umbral de
`RD 13.3` en una trampa de fraccionamiento y contradice el documento unico de
`RD 13.2`.

## Consecuencias

Positivas: el motor permanece una funcion pura sobre una bolsa; el valor punto
por canal es comprobable con un fixture de dos canales; la liquidacion puede
pagar lo que el reglamento manda a pagar.

A cambio: #36 debe conocer el periodo del proceso y agregar; el panel de corridas
mostrara N corridas por periodo, no una. El ADR se enlaza desde #34 y #36.
