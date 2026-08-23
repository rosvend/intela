---
name: recaudo-y-tarifas
description: Usar al trabajar con el lado del ingreso de REDES SGC - tarifas por categoria de usuario, convenios, facturacion, la bolsa a repartir, recargos por incumplimiento, conversion de tarifas en dolares, o al modelar Usuario, Convenio, Tarifa y Recaudo. Tambien al decidir cuanto se le cobra a un canal, una sala de cine, un hotel, una plataforma OTT o una empresa de transporte, y al distinguir recaudo nacional de internacional.
---

# Recaudo y tarifas

## Por que existe esta skill

El reparto empieza donde termina el recaudo. `reparto-y-distribucion` toma la bolsa como dada;
aqui es donde la bolsa se forma. Confundir las dos mitades es el origen del error mas caro del
proyecto: creer que el dinero llega por fila en un reporte de uso. **No llega.** El dinero entra
por una factura a un usuario, calculada con el Reglamento de Tarifas, y los reportes de uso solo
sirven despues para ponderar como se reparte.

## La regla que domina todas las demas

**La tarifa pactada vence a la tarifa publicada.**

Por mandato legal las tarifas deben concertarse con el usuario, asi que la tabla del `RT` es un
**marco de negociacion**, no un precio. Lo que se factura es lo que dice el convenio firmado con
ese usuario concreto.
Fuente: `T-11`, `RT 1`, `RT` Presentacion, Decreto 1066 de 2015 Art. 2.6.1.2.5

Implementacion: el modelo guarda `Convenio(usuario, tarifa_pactada, vigencia)`. La tabla del
reglamento es el **valor por defecto cuando no hay convenio**, nunca la verdad. Un sistema que
factura leyendo la tabla publicada esta mal aunque de el numero correcto por casualidad.

## Categorias de usuario y su base de calculo

| Categoria | Tarifa | Base | Regla |
| --------- | ------ | ---- | ----- |
| Television abierta | 4% | Ingresos vinculados a la utilizacion del repertorio | `T-01`, `RT 3.1.1` |
| Television cerrada | 4% | Ingresos vinculados a la utilizacion del repertorio | `T-01`, `RT 3.1.2` |
| Salas de cine | 4% | **Contradictoria, ver abajo** | `T-02`, `RT 3.2` / `RT 4` |
| Transporte aereo | $1.492 COP | Por plaza utilizada | `T-03`, `RT 3.3` |
| Transporte terrestre | $20.600 a $74.621 COP | Mensual por vehiculo, segun capacidad | `T-04`, `RT 3.4.1` |
| Transporte fluvial | USD 100 a USD 1.200 | Por aparcada en puerto, segun pasajeros | `T-05`, `RT 3.4.2` |
| Hoteles y alojamiento | Tabla con un vacio | Mensual por habitacion x categoria de precio | `T-06`, `RT 3.5` |
| Establecimientos de salud | $1.150 a $6.888 COP | Mensual por habitacion | `T-07`, `RT 3.5` |
| Medios digitales | 4% | Ingresos netos por suscripcion mensual | `T-08`, `RT 3.6` |
| Otros abiertos al publico | $15.000 COP | Mensual por receptor | `T-09`, `RT 3.7` |

Nota de lectura: `RT 3` anuncia "seis tipos de usuario" y a continuacion enumera siete; la tabla
resumen de `RT 4` lista diez categorias. La discrepancia esta en el documento fuente. Modelar por
la tabla de `RT 4`, que es la operativa.

## Recargo por incumplimiento: 50%

Aplica en todas las categorias. Es un recargo sobre la tarifa vigente, no una sancion aparte.
Fuente: `T-10`, `RT 3.1.1` a `RT 3.7`

## Los dos vacios que no se rellenan con un valor inventado

Estos parametros **faltan** y se modelan como ausentes. Un recaudo que los necesite y no los
encuentre debe fallar ruidosamente. Poner cero, o elegir la interpretacion mas comoda, produce
una factura defendible ante nadie.
Ver `0004-parametros-normativos-como-dato.md`.

**`T-02` Base de calculo de salas de cine — Conflicto.** Dos partes del mismo reglamento dicen
cosas distintas y ninguna referencia a la otra:

- `RT 3.2`: 4% de los **ingresos netos de explotacion del exhibidor**, comprendiendo taquilla,
  publicidad y servicios de restauracion.
- `RT 4` (tabla resumen): el 4% se calcula **a partir del 50% del recaudo de taquilla**, porque el
  50% restante va al distribuidor.

Las dos bases dan resultados muy distintos. **Preguntar al cliente cual aplica** antes de
implementar el calculo de cine.

**`T-06` Hueco en la tabla hotelera.** El tramo *71 a 100 habitaciones* define Categoria 4 hasta
$42.000 y Categoria 5 desde $160.001, y deja **sin tarifa el rango $42.001 a $160.000**. El tramo
*100 en adelante* solo define desde $42.000. Pedir al cliente la tabla corregida.

## Tarifas en dolares dentro de un sistema en pesos

El transporte fluvial se tarifa en USD (`T-05`, `RT 3.4.2`). Convertir exige una TRM con **fuente
y fecha declaradas**, que es un parametro normativo mas: la TRM del dia de causacion no es la del
dia de facturacion ni la del dia de pago, y la diferencia tiene que ser explicable.

## Lo que no es comunicacion publica y por tanto no se recauda

- Uso con fines estrictamente educativos dentro de institutos de educacion, sin cobro de entrada.
- Establecimientos abiertos al publico que usan la obra para entretenimiento de **sus
  trabajadores**, o cuya finalidad no sea entretener al publico consumidor con animo de lucro.

Fuente: `R-26`, `RD 8.2`, `RT 1`, Ley 1835 de 2017 paragrafo 2

## Nacional e internacional se gestionan por separado

No es una etiqueta: son dos circuitos distintos que se invierten por separado para poder
identificar a que tipo de reparto corresponden sus rendimientos (`R-35`, `RD 10.3`). Y la
diferencia tiene consecuencias de calculo aguas abajo:

- La **reserva por errores tecnicos** del 5% aplica **solo al recaudo nacional** (`R-07`,
  `RD 14.5.4`).
- Los **Fees in Error** se devuelven integros, sin deduccion administrativa ni de bienestar
  social (`R-16`, `RD 13.7`).
- Las **fechas de corte** de rendimientos difieren: nacional el 20 de octubre, internacional el
  30 de septiembre (`R-34`, `RD 10.1`, `RD 10.2.1`).

Ver `proceso-y-aprobaciones` para las dos maquinas de estado que esto produce.

## Frontera del modulo

`Recaudo` es el unico modulo que conoce `Usuario`, `Convenio` y `Tarifa`. Aguas abajo solo circula
una **bolsa por usuario y periodo**. Ningun modulo de reparto debe poder leer una tarifa: si
pudiera, alguien acabaria calculando un importe por obra a partir del precio, que es precisamente
el modelo equivocado.
Fuente: `0003-monolito-modular.md`, pagina 2 del diagrama de arquitectura.
