# 0024 La liquidacion se emite al entrar a `liquidacion_final` y espera a todas las corridas del periodo

Fecha: 2026-09-28
Estado: Vigente

## Contexto

`Liquidaciones.GenerarLiquidacion` (#36) existia con pruebas pero nada la invocaba
en runtime: ni `httpapi`, ni `cmd/api`, ni `cmd/lambda`, ni `cmd/worker` (#193). Una
corrida nacional podia recorrer las nueve etapas del `RD 13.5`, con sus dos compuertas
firmadas, y terminar en `auditoria` sin una sola orden en `ordenes_pago`.

Al reproducirlo contra Postgres real aparecieron defectos que un disparador, por si
solo, no arreglaba y en parte empeoraba:

1. **La compuerta de la liquidacion no se podia cumplir.** Pedia las firmas de
   `distribucion` y `contabilidad` sobre la revision *vigente* de una corrida en
   `liquidacion_final`. Pero `ProcesoDeReparto.AvanzarEtapa` sube la revision al salir
   de una compuerta (#159), asi que las firmas de `verificacion` quedan siempre en
   `revision - 1`. Las pruebas de #80 sembraban un estado que la maquina no produce.
2. **La agregacion del ADR 0019 dejaba de ocurrir.** Con un disparador automatico, la
   primera corrida de un periodo y circuito en llegar emitiria sola, y las demas
   chocarian con la guarda de idempotencia y se quedarian sin orden, sin error ni
   asiento.
3. **Dos corridas de la misma bolsa se sumaban.** `procesos` no restringe una corrida
   por bolsa (`AbrirCorridaDelPeriodo` con otra `corrida` abre `proc-<bolsa>-2`).
4. **Un lote sin ordenes no era idempotente.** La guarda miraba solo las ordenes: un
   periodo con todo retenido volvia a emitir en cada reintento y duplicaba el asiento
   `liquidacion.residuo_prorrateo`.
5. **El notificador no notificaba.** `notificaciones.Bitacora` escribia al log y
   devolvia un acuse, y el comentario de `GenerarLiquidacion` exigia que quien la
   conectara trajera un canal real: sin el, el plazo de `R-10` corre sin que el
   titular haya recibido nada.

## Decision

**Emitir al entrar a `liquidacion_final`, dentro de la misma unidad que `AvanzarEtapa`,
solo en el circuito nacional.** `RD 13.5` genera ahi la liquidacion final que se
remite para el pago. Avanzar y emitir son un solo hecho asentado, igual que valorizar
al entrar a `importe_obra`: si la emision falla, la corrida no cambia de etapa. El
internacional no valoriza (`RD 7.4`), no tiene lineas de titular de las que emitir, y
su reparto por la proporcion de la sociedad hermana no esta modelado todavia.

**La compuerta pide las firmas de la verificacion, no las de la revision vigente.**
`MetaProceso.CerroLaVerificacion` exige que los dos roles hayan firmado una misma
revision anterior a la vigente. La regla vive en Go (`internal/aplicacion`) y no en el
SQL, para que se pruebe sin base de datos: el adaptador devuelve todas las corridas
del periodo con todas sus firmas (`CorridasDePeriodo`).

**La liquidacion del periodo espera a todas sus corridas.** Mientras alguna corrida del
mismo periodo y circuito siga antes de `liquidacion_final`, entrar no emite y la
corrida queda esperando (`ErrLiquidacionEnEspera`, tolerado al entrar). Salir de
`liquidacion_final` hacia `pago_registro` vuelve a pedir la liquidacion antes de
guardar, y si el periodo sigue esperando responde 409 con los ids que faltan: no se
paga lo que no se liquido. La ultima corrida en llegar emite por todas, incluidas las
que ya estuvieran mas adelante. El cerrojo de aviso del periodo (`BloquearPeriodo`)
serializa dos corridas que entran a la vez: la segunda ve a la primera ya confirmada
en `liquidacion_final` y emite por las dos.

**Una corrida que llega a un periodo ya liquidado no avanza.** `ErrPeriodoYaLiquidado`,
409. Incorporarla a ordenes ya enviadas reabriria un plazo de `R-10` que puede estar
corriendo o vencido, y una segunda orden del mismo periodo choca con el
`UNIQUE (titular_id, periodo, circuito)`. Como se paga ese dinero lo decide el Consejo
Directivo (corrida de ajuste por `R-11` o reapertura); lo que el sistema no hace es
dejarla pasar como si estuviera liquidada.

**Dos corridas listas sobre la misma bolsa no se suman.** `ErrCorridaNoCuadra`, sin
emitir. Elegir una en silencio decidiria por el operador cual de los dos resultados
vale.

**La idempotencia se apoya tambien en el asiento del lote.** El asiento
`liquidacion.residuo_prorrateo` lleva ahora las corridas que aportaron (`procesos`):
es la marca de que el lote se emitio aunque no haya producido ninguna orden, y lo que
separa un reintento (la corrida esta entre ellas) de una corrida que llega tarde.

**El aviso va por el portal, en la misma transaccion que la orden.** `RD 13.8.8` admite
como notificacion la puesta a disposicion en la pagina web. `postgres.AvisoPortal`
escribe la fila de `notificaciones` (`via = 'portal'`, destino `/mis-liquidaciones`)
dentro de la unidad que emite la orden, y se niega a escribir fuera de una: el aviso
existe si y solo si la orden existe. El acuse es la huella del contenido anunciado,
recalculable anos despues (ADR 0006). Se retira el adaptador de log
(`internal/infraestructura/notificaciones`), redefiniendo el alcance de #55 para que
el acuse de portal exista desde la emision y no dependa de la visita posterior del titular.

## Alternativas consideradas

**Emitir al entrar a `pago_registro`.** Descartada: `RD 13.5` genera la liquidacion en
`liquidacion_final` y la remite para el pago; emitir al pasar la siguiente compuerta
haria firmar el pago de algo que el titular todavia no ha visto.

**Emitir con las corridas que ya estan listas y dejar las demas para despues.** Es lo
que hacia la guarda anterior. Descartada: con un disparador automatico convierte la
agregacion del ADR 0019 en la excepcion, y las corridas tardias quedan sin orden y
sin rastro.

**Un `POST` para emitir a mano.** Descartada: la emision es consecuencia de la etapa, y
una accion aparte se puede olvidar o repetir.

**Mantener la regla de la compuerta en el SQL.** Descartada: era justo donde vivia el
defecto de la revision, y ahi no se prueba sin base.

## Consecuencias

Positivas: una corrida nacional que cierra su verificacion deja ordenes visibles en
`GET /liquidaciones` y `GET /mis-liquidaciones` desde el mismo commit, con su aviso y
su asiento. Reintentar la transicion no emite dos veces. El periodo se liquida una vez,
con todas sus bolsas, como pide el ADR 0019.

A cambio:

- Una corrida abandonada en una etapa temprana deja el periodo esperando: no hay hoy
  una operacion para cancelar una corrida. El 409 de salida nombra la que falta.
- Un reproceso de la misma bolsa (`proc-<bolsa>-2`) bloquea el periodo con
  `ErrBolsaRepetida` hasta que exista la forma de descartar una de las dos.
- Entrar a `liquidacion_final` exige un `smmlv` vigente en `parametros`; sin el falla
  con `ErrParametroAusente` (ADR 0004). El seed siembra uno sintetico.
- Las corridas que ya pasaron `liquidacion_final` antes de este cambio no reciben
  ordenes de forma retroactiva. Se incorporan si una corrida hermana del mismo periodo
  llega despues; si no, su emision es una operacion aparte.
- Reajuste explícito de alcance respecto a #55: la propuesta inicial de #55 preveía
  mantener un adaptador dummy de log y registrar el acuse de portal cuando el titular
  iniciara sesión e interactuara con el proyecto. Este cambio retira el adaptador de log
  falso (`internal/infraestructura/notificaciones/log.go`), ya que emitir un acuse sin
  efecto en base ni portal corría los plazos de `R-10` y `R-20` sin notificación real.
  La puesta a disposición en portal (`RD 13.8.8`) queda satisfecha al emitir las órdenes
  transaccionalmente en `/mis-liquidaciones`. El alcance restante de #55 cubre:
  1. El adaptador de correo electrónico (`via = 'email'`) cuando se configure.
  2. El centro de notificaciones en administración para auditoría de envíos y acuses.
  3. El reloj diferenciado de aviso de recaudo para `RD 13.1.3` (`fecha_informe_recaudo`).
