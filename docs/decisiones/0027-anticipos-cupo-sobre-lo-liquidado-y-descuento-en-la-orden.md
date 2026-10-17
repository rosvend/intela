# 0027 Anticipos: el cupo se mide sobre lo liquidado y el descuento va en la orden de pago

Fecha: 2026-10-16
Estado: Vigente, base del 25% provisional (P-24)

Origen: issue #56 (`R-30` a `R-33`).

## Contexto

El Reglamento de Anticipos V7 (`docs/reglamentos/anticipos/`) fija las reglas pero deja abiertas
las preguntas que el codigo no puede dejar abiertas:

1. `RA 3.1.g`: el monto "no podra superar el 25% de los Derechos de Autor generados a su favor en
   los procesos de reparto ordinarios de los ultimos dos anos, tanto por recaudos nacionales como
   internacionales". ¿Que cifra del sistema es "los derechos generados a su favor", y desde cuando
   se cuentan "los ultimos dos anos"?
2. `RA 3.1.h`: "no podra ser superior a los cinco (5) salarios minimos mensuales legales
   vigentes". ¿Vigentes cuando?
3. `RA 3.1.b`: "La solicitud de un anticipo no supone su autorizacion", y `RA 3.1.e`: "No podran
   hacer desembolsos de anticipos a los afiliados que tengan embargos reportados". ¿Se mira al
   pedir, al aprobar o en los dos?
4. `RA 2.1`: el anticipo "sera cubierto de manera automatica con cargo a los derechos generados a
   futuro por el afiliado en los procesos de recaudo nacional e internacional". ¿Contra que cifra
   se descuenta, y como convive con la menor cuantia de `R-11`?
5. ¿Que es un embargo reportado en el sistema, y quien lo reporta?
6. `RA 3.1.f`: "La Solicitud tendra que ser hecha directamente por el Socio". ¿Quien puede pedir?

## Decision

**La base del 25% son las lineas de reparto del socio que ya se liquidaron.** Es la suma de
`resultados_titular.importe` del titular en las corridas que entraron en alguna orden de pago SUYA
enviada en `(hoy - 2 anos, hoy]`, en dias civiles UTC como el resto de fechas del sistema. No es:

- el **neto de la orden**, que ya lleva descontado el anticipo anterior (`R-33`): sumarlo le
  bajaria el cupo a quien ya pago uno, como si esos derechos no los hubiera generado; y que, si
  la orden absorbio una diferida de `R-11`, lleva tambien el neto de otra orden y contaria dos
  veces el mismo derecho;
- una corrida **sin orden**: todavia puede cambiar en verificacion, y lo "generado en los
  procesos de reparto" es lo de un proceso que ya se liquido;
- una bolsa **accesoria** (reserva liberada, rendimientos): no pasa por `resultados_titular`, asi
  que no es reparto ordinario.

La linea es lo que el reparto asigno al titular, despues de las deducciones de la corrida
(administracion, bienestar social, reserva). Tomar el bruto antes de esas deducciones seria la
lectura mas generosa de "generados a su favor"; se toma la linea porque es la cifra que el reparto
le reconocio. Es la respuesta del equipo, sin confirmar con REDES: **P-24**.

Nacional e internacional se suman sin distincion: cuentan las ordenes de los dos circuitos. Hoy
solo el nacional emite ordenes (`Procesos.liquidar`, ADR 0024). Cuando el internacional las emita,
entran si su reparto escribe `resultados_titular`, que es de donde sale la linea; si no, hay que
cambiar `DerechosLiquidados`.

**El SMMLV es el vigente el dia de la decision**, leido de `parametros` (ADR 0004). Si no hay, la
solicitud falla con `ErrParametroAusente`: un tope de cero diria "no tienes cupo" sin decir por
que.

**Los dos topes son techo y se truncan al centavo**: "no podra superar" y "no podra ser superior"
no admiten redondear hacia arriba. Un monto con mas de dos decimales se rechaza, porque
`NUMERIC(18,2)` lo redondearia en silencio y podria quedar por encima del tope que se comprobo.

**Las reglas se aplican al pedir y otra vez al aprobar**, con la situacion de cada fecha. Pedir
devuelve el motivo en el acto a quien no cumple. Aprobar es lo que el reglamento regula: entre
pedir y aprobar puede llegar un embargo, perderse la calidad de socio o moverse la ventana.

**Un solo anticipo vigente lo hace cumplir la base**, con un indice unico parcial sobre los
estados `solicitado` y `aprobado` (migracion 00031). La comprobacion del caso de uso no tiene
carrera con el indice detras.

**El descuento va en la orden de pago, en la misma unidad de trabajo que la emision.**
`Liquidaciones.GenerarLiquidacion` le pregunta al modulo de anticipos, por cada orden, cuanto
descontar de lo que esa orden iba a girar:

- despues de incorporar las diferidas de `R-11` -tambien son derechos que no se le han girado- y
  antes del aviso, para que la orden notificada muestre el renglon;
- con el concepto `anticipo` en `ordenes_pago_deducciones`, sin tocar el bruto: el anticipo no
  cambia lo que el socio genero en el periodo, solo lo que se le gira. Tampoco toca la
  valorizacion ni el valor punto: si los tocara, el anticipo de un autor alteraria lo que cobran
  sus coautores;
- con la fila del anticipo bloqueada (`FOR UPDATE`) y una escritura condicional sobre el saldo
  anterior: dos liquidaciones del mismo socio en periodos distintos no comparten el cerrojo de
  aviso del periodo, y sin el de fila descontarian las dos sobre el mismo saldo;
- con un asiento `anticipo.descontado` por descuento (orden, monto, saldo antes y despues), del
  propio modulo de anticipos (ADR 0006).

El puerto es obligatorio para emitir: una liquidacion que no pudiera descontar giraria entero lo
que el socio debe, y eso es un fallo de cableado, no un "sin anticipos".

Con `R-11`, el umbral del 2% de un SMMLV se mide sobre el neto ya descontado. Una orden que el
anticipo absorbe entera queda en neto cero, no es pagable y, sin respuesta, se difiere sin nada
que arrastrar: no hay nada que girar.

**Un embargo es una fila de `embargos`**, con el oficio como soporte y su vigencia
`[reportado, levantado)` en dias civiles. No un booleano en `titulares`: la pregunta de una
auditoria (`RD 16`) es si el socio estaba embargado el dia en que se aprobo, y un booleano solo
contesta por hoy. Registrar y levantar embargos **no tiene caso de uso todavia**: la tabla se
escribe a mano hasta que llegue el suyo.

**Solo pide el titular, para si mismo** (`RA 3.1.f`): el rol `titular` y el titular de la sesion,
nunca uno del cuerpo de la peticion. Decide `administrador`, que representa al Consejo Directivo
(`RA 3.2.3`, `docs/architecture/roles.md`).

## Alternativas consideradas

**La base es el neto de `ordenes_pago`.** Descartada por las dos razones de arriba: el anticipo
anterior baja el cupo del siguiente, y una diferida absorbida se cuenta dos veces.

**La base son todas las lineas de las corridas abiertas en la ventana**, liquidadas o no.
Descartada: una corrida en verificacion todavia no es un derecho reconocido.

**`embargos_reportados BOOLEAN` en `titulares`.** Mas simple, y no responde la pregunta que se le
va a hacer: si lo estaba el dia de la aprobacion.

**Descontar solo del neto del periodo, sin los arrastres.** Descartada: le giraria entero un
arrastre a quien debe un anticipo.

**Recoupment manual por el administrador.** Descartada en la propia issue: es dinero que se debe a
la sociedad, y `RA 2.1` lo hace automatico.

## Consecuencias

Positivas: el cupo con que se decidio queda asentado con su base, su ventana, el SMMLV y el tope
(`anticipo.solicitado` y `anticipo.aprobado`), que es el "estado de cuenta unico y calculo del
cupo" de `RA 3.2.2.d`. Cada peso descontado esta en una orden de pago y en un asiento del
anticipo, y la identidad `monto = saldo + suma de lo descontado` se puede comprobar sin recalcular
ninguna corrida.

A cambio: si REDES responde P-24 de otra forma -el bruto, o la ventana por periodo y no por fecha
de envio-, cambia una consulta (`DerechosLiquidados`) y no el dominio. Mientras no responda, el
cupo es defendible solo como lectura del equipo.

Lo que queda fuera, y no se finge: el reintegro voluntario de `RA 2.1`, el apoderado por fuerza
mayor de `RA 3.1.f`, la delegacion en el Gerente del paragrafo de `RA 3.2.3`, los plazos de
notificacion y giro de `RA 3.2.3` y `RA 3.2.4`, `RA 3.1.a` (al dia con los estatutos), el alta de
embargos y la liquidacion del circuito internacional.
