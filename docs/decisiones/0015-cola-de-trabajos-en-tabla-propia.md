# 0015 La cola de trabajos es una tabla propia, no River

Fecha: 2026-09-04
Estado: Vigente
Modifica a: [0010 Stack de aplicacion: Go](0010-stack-go.md) — solo la fila "Cola" de su tabla de
stack. El resto de `0010` sigue vigente sin cambios.

## Contexto

La tabla de stack de `0010` decia, en una linea: **Cola — `River` (respaldada por PostgreSQL) —
encolado transaccional: conserva la transaccion local por etapa que exige `0003`**. Nunca se
implemento, y con ella se quedo sin implementar la garantia transaccional que prometia: ver
"El encolado no es transaccional" en las consecuencias.

Lo que si existe, desde la migracion `00001`, es una tabla `cola_trabajos` escrita a mano y un
puerto `ColaTrabajos` con tres metodos (`Encolar`, `Tomar`, `Cerrar`) declarado en
`internal/aplicacion/puertos.go`. Ninguna de las dos cosas tenia una ADR detras: el andamiaje
eligio una via distinta a la que la ADR de stack habia escrito, y la divergencia se quedo sin
registrar.

El issue #35 es el que tiene que llenar esa cola de codigo, asi que es el momento de decidir en vez
de heredar. La eleccion no es libre: hay tres restricciones del proyecto que la acotan.

**El perfil de carga es un pico anual.** `RD 12` exige **al menos una distribucion por ano
calendario**, y en la practica se hace en la primera semana de diciembre (`RD 10.1`). El universo
son los afiliados de una sociedad de gestion colectiva colombiana de escritores audiovisuales:
miles de obras, no millones. Las muestras reales del cliente son de 59 y 49 filas. `0003` ya uso
exactamente este argumento para no partir el sistema en microservicios.

**La idempotencia que hace falta es de dominio, no de transporte.** El objetivo 7 / KR-3 no pide
"cada mensaje se entrega al menos una vez": pide que **una corrida no se repita y que un periodo no
se pague dos veces**. Eso no se resuelve con reintentos ni con confirmaciones; se resuelve con una
clave que diga que dos encolados son el mismo trabajo logico. En este dominio esa clave es
`(tipo, periodo, corrida)`, y `corrida > 1` significa corrida de **ajuste**, que es la unica forma
legitima de volver sobre un periodo ya repartido (`RD 14.5.10` a `RD 14.5.12`: se ajusta con una
liquidacion nueva y avalada, no reabriendo la anterior).

**La auditoria lee el estado.** `RD 16` permite una auditoria interna o externa **en cualquier
tiempo y lugar**, y `RD 13.4` exige documentacion conservada diez anos. Que el estado de la cola se
pueda mirar con un `SELECT` sobre una tabla cuyas columnas estan en la misma migracion que el resto
del esquema no es un detalle estetico: es la diferencia entre responder con una consulta y
responder con una investigacion.

## Decision

**Se conserva `cola_trabajos` como tabla propia del esquema. River queda descartada por ahora, y la
fila "Cola" de `0010` se lee a traves de esta ADR.**

La forma concreta que fija esta decision:

**La clave natural es `(tipo, periodo, corrida)`, con restriccion `UNIQUE` en la base.** Encolar es
un `INSERT ... ON CONFLICT DO NOTHING`: un duplicado es un no-op que devuelve `false`, sin error.
Es la base la que arbitra y no una lectura previa en Go, porque entre un `SELECT` que no encuentra
nada y el `INSERT` que le sigue caben dos schedulers.

**Reclamar es `SELECT ... FOR UPDATE SKIP LOCKED`** dentro de una sola sentencia que ademas marca
la fila `en_curso`. Es lo que permite levantar varias replicas de `worker` sin eleccion de lider,
que es como `0003` describe el escalado del reparto.

**La politica de reintentos vive en el nucleo**, en `aplicacion.Reintentos`, y el adaptador solo
guarda el instante en `disponible_en`. Los tres valores —maximo de intentos, espera base y techo—
**no son normativos**: no salen del reglamento y por tanto no entran por `ParametrosNormativos`
(`0004`). Son configuracion de operacion y los fija `cmd/worker` desde el entorno.

**El calendario sigue siendo dato en `calendario`**, no cron del sistema operativo. Esa mitad no se
discute aqui: ya la decidieron `0004` y `0008`.

### Lo que esta decision NO dice

No dice que River sea peor. Dice que hoy no compra lo que cuesta. Si el perfil de carga cambia
—ingesta continua en vez de un pico anual, decenas de miles de trabajos, necesidad de un panel
operativo— la decision se revisa, y revisarla es barato **a proposito**: `ColaTrabajos` son tres
metodos y un adaptador nuevo los satisface sin tocar el nucleo. La costura esta marcada, igual que
`0003` dejo marcada la de extraer el matching a un servicio.

## Alternativas consideradas

**Adoptar River ahora, como decia `0010`.** Es la alternativa seria y estuvo sobre la mesa. Da
gratis lo que aqui hay que escribir a mano: reintentos con espera exponencial, trabajos periodicos,
un catalogo de estados, rescate de trabajos huerfanos, y un panel de observabilidad. Y su encolado
si es transaccional —acepta la transaccion de quien llama—, que es lo que `0003` pide. **Eso es lo
que esta decision deja sobre la mesa**, y es la unica de sus ventajas que no se recupera escribiendo
mas codigo en esta capa: ver "El encolado no es transaccional" en las consecuencias.

Se descarta por tres razones, en este orden:

1. **La propiedad que el issue exige, River no la da por defecto.** Sus reintentos garantizan que
   un trabajo se ejecute; no garantizan que un periodo no se pague dos veces. Para eso hay que usar
   su funcionalidad de trabajos unicos, es decir, declarar igualmente una clave natural. La parte
   dificil del problema —cual es la clave, y que `corrida > 1` es un ajuste y no un duplicado— es
   de dominio y hay que escribirla en cualquiera de los dos caminos.
2. **El coste que evita es pequeno en este perfil.** Lo que hay que escribir a mano son unas
   cuarenta lineas de politica de reintentos y una consulta de reclamo, las dos probadas. A cambio
   entra una dependencia con su propio esquema, sus propias migraciones y su propio ciclo de
   versiones, en un proyecto de tres meses y cuatro estudiantes.
3. **El esquema ya modela `cola_trabajos`, y River traeria el suyo.** Convivirian dos colas en la
   misma base: la de River y la tabla que sigue en `00001`. Migrar de verdad significa una
   migracion que la elimine y reescribir el puerto, que es trabajo real que este issue tendria que
   hacer antes de empezar el suyo.

**Usar River solo como planificador periodico** y dejar `cola_trabajos` para el resto. Descartada:
el calendario es dato que edita el administrador (`0004`), no una expresion periodica de
configuracion. Seria adoptar la dependencia por la mitad de su valor.

**Cron del sistema operativo en vez del calendario en base.** Descartada por `0004` y `0008`: las
fechas las fija el Consejo Directivo y las puede modificar por fuerza mayor con re-notificacion
(`RD 12`). Con cron, mover una fecha que aprobo un organo social seria un despliegue.

**`LISTEN`/`NOTIFY` en vez de sondeo.** Descartada por ahora: quita unos segundos de latencia en un
proceso que corre una vez al ano y anade una via de fallo silencioso —una notificacion perdida
mientras el worker reconecta no se recupera sola, y haria falta el sondeo igualmente como red de
seguridad—. El sondeo cada cinco segundos es suficiente y no tiene ese modo de fallo.

## Consecuencias

Positivas: cero dependencias nuevas. El estado de la cola se consulta con `SELECT` sobre columnas
que estan en la misma migracion que el resto del esquema, que es lo que `RD 16` acaba necesitando.
La clave natural convierte "no pagar dos veces" en una restriccion de la base, es decir, en algo que
no depende de que ningun proceso se comporte bien.

A cambio, y esto es el precio real:

- **El encolado NO es transaccional.** Es el punto donde esta ADR se aparta de `0003`, y conviene
  decirlo sin rodeos porque quien escriba #33/#34 va a querer exactamente lo contrario: guardar la
  liquidacion y encolar el paso siguiente sin ventana entre medias. Hoy no se puede.
  `Store.Encolar` ejecuta `s.pool.Exec` (`postgres/cola.go`), que saca su propia conexion del pool.
  El puerto es `Encolar(ctx, clave, payload) (bool, error)` y **no tiene por donde recibir una
  transaccion**: no puede tenerla, porque `internal/aplicacion` no puede nombrar `pgx.Tx` sin
  romper la frontera que vigila `depguard` (`0012`). El unico mecanismo transaccional del proyecto
  es `Store.EnTransaccion(ctx, func(pgx.Tx) error)`, que pasa la `tx` **dentro** del adaptador, y no
  hay propagacion por contexto.

  Se puede vivir con ello por lo mismo que documenta `Planificador.Disparar` en
  `internal/aplicacion/trabajos.go`: **las dos operaciones son idempotentes y el orden tolera la
  interrupcion**. Encolar va antes de marcar, asi que una muerte en medio deja el periodo encolado
  y sin marcar, y la pasada siguiente lo vuelve a encolar —no-op por clave natural— y lo marca.
  Nada se duplica. Al reves si habria dano: marcado y sin trabajo, la corrida del ano no se
  ejecutaria y nadie se enteraria. La atomicidad se sustituye por idempotencia mas orden, que en
  este perfil es suficiente y no depende de una transaccion que abrace dos puertos.

  Recuperar la promesa de `0003` no es un cambio de documentacion: exige que el puerto tenga forma
  de participar en la transaccion de quien llama. Si #33/#34 lo necesitan de verdad, es una ADR
  nueva, no una nota al pie de esta.

- **No hay panel.** El estado se mira consultando la tabla hasta que #40 lo exponga.
- **Los reintentos son nuestros.** Estan probados, pero cualquier defecto en la espera exponencial
  es un defecto propio y no de una biblioteca con miles de usuarios.
- **No hay rescate de trabajos huerfanos.** Si un worker muere con un trabajo `en_curso` —matado
  por el sistema, no por un panico, que si esta cubierto— la fila se queda ahi: `Tomar` solo mira
  las pendientes y nadie la recoge. Con una corrida al ano y una consola administrativa se ve y se
  corrige a mano, pero es un hueco conocido. Cerrarlo es una consulta que devuelva a `pendiente` lo
  que lleve demasiado `en_curso`; se deja fuera de #35 a proposito, porque exige decidir cuanto es
  "demasiado" y eso depende de cuanto tarde una corrida de verdad, que todavia no existe.

Riesgo asumido: que "por ahora" se convierta en "para siempre" sin que nadie lo vuelva a mirar. La
mitigacion es que el puerto siga teniendo tres metodos. En cuanto `ColaTrabajos` crezca —trabajos
periodicos, prioridades, cuotas por tipo, dependencias entre trabajos— la decision hay que
reabrirla, porque a esas alturas se estaria escribiendo River a mano y peor.

## Pendiente de confirmacion

Esta decision la toma el issue #35 porque el issue pide explicitamente decidir y dejarlo escrito.
**@rosvend y @killgreck la confirman o la revierten en la revision del PR.** Si la revierten, lo que
hay que rehacer es el adaptador `internal/infraestructura/postgres/cola.go` y la migracion `00003`;
el nucleo (`ClaveTrabajo`, `Despachador`, `Planificador`) y sus pruebas se conservan tal cual, que es
justamente lo que la frontera de `0002` existe para permitir.
