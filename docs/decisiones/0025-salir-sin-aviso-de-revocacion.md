# 0025 "Salir" no avisa si el servidor no confirmo la revocacion; el servidor cubre el riesgo

Fecha: 2026-10-05
Estado: Vigente
Enmienda: ADR 0013

Origen de la decision: el product owner, en la revision de la PR #215
(https://github.com/rosvend/intela/pull/215): sin aviso en la interfaz, y el riesgo se cierra en el
servidor.

## Contexto

La `0013` hace de la sesion un token opaco en tabla porque "poder cortar una sesion ya emitida vale
mas que ahorrarse una consulta". Revocarla es `DELETE /api/auth/session`, que borra la fila de
`sesiones`. Si ese `DELETE` no llega (sin red) o falla (5xx), la fila sigue ahi.

Antes de esta decision, eso dejaba el token vivo hasta `SESION_TTL` (12 h absolutas): no habia
caducidad por inactividad, e iniciar sesion otra vez no invalidaba las sesiones anteriores. Es
decir, la garantia de la `0013` dependia de que un unico `DELETE` del navegador saliera bien, y eso
iba contra OWASP ASVS V3.3.1 (el logout invalida la sesion en el servidor) y V3.3.2 (reautenticacion
tras inactividad), en una API que autoriza movimientos de dinero de terceros.

En la revision de #71 se pidio que "Salir" lo dijera, y `b47a131` anadio un aviso. El rediseno de la
PR #215 lo quito, y la revision de esa PR lo marco como bloqueante.

## Decision

**La interfaz no muestra el aviso, y el servidor deja de depender de que el `DELETE` llegue.** Son
cuatro mitigaciones, y las cuatro estan en el codigo:

1. **Reintento antes de borrar el token** (`web/src/sesion.tsx`, `salir()`). El `DELETE` se
   intenta hasta tres veces, con esperas de 300 ms y 800 ms, mientras el token sigue guardado: sin
   token no hay con que reintentar, asi que se reintenta antes. Un 4xx no se reintenta (un 401
   significa que la sesion ya no existe). Al final el token se borra siempre, y en silencio.
2. **Caducidad por inactividad en el servidor** (ASVS V3.3.2). `sesiones.ultimo_uso` (migracion
   `00029`) se compara en el `WHERE` de `PorToken` con `ahora - SESION_INACTIVIDAD` (30 min por
   defecto, `cmd/api` y `cmd/lambda`). El instante sigue entrando por `PuertoReloj` (`0005`), como
   el TTL. Cada uso corre la ventana; la escritura se limita a una por minuto y sesion.
3. **Un login nuevo revoca las sesiones previas del usuario.** `Crear` borra las filas del usuario y
   da de alta la nueva en una sola sentencia, asi que es atomico. Un token que quedo vivo tras un
   logout fallido muere en cuanto su dueno vuelve a entrar.
4. **El token sale del equipo.** Como antes: tras "Salir" no queda en `localStorage`, y el siguiente
   que use el navegador no hereda la sesion.

El aviso se quita porque asustaba a la persona con algo que ella no podia resolver. Con las
mitigaciones 1 a 3 ya no hace falta que lo resuelva ella.

### Los sondeos de la interfaz no cuentan como uso (#223)

La mitigacion 2 mide la inactividad por peticiones: cada una que lleva el token corre
`ultimo_uso`, y el servidor no sabe si la pidio una persona o un temporizador. Distribucion sondea
cada 15 s (`INTERVALO_SONDEO_MS`), y con un `setInterval` sin condiciones una pestana abierta y
desatendida sostenia la sesion hasta `SESION_TTL`: para esa pantalla la inactividad no existia
(https://github.com/rosvend/intela/issues/223).

Se corrige en el cliente. Todo sondeo va por `web/src/useSondeo.ts`, que:

- no pide nada con la pestana oculta (`document.visibilityState`) y, al volver a verse, pide
  enseguida si la persona sigue dentro de la ventana;
- deja de pedir cuando pasan `SESION_INACTIVIDAD_MS` sin teclado ni puntero (`keydown`,
  `pointerdown`, `pointermove`, `wheel`). Que la pestana vuelva a verse no cuenta como actividad;
- con la siguiente accion de la persona pide enseguida. Si la sesion ya cayo, ese 401 lleva a la
  pantalla de entrada como cualquier otro.

`SESION_INACTIVIDAD_MS` (`web/src/sesion.tsx`) es una copia del valor por defecto de
`SESION_INACTIVIDAD`, porque la API no lo publica. `web/src/sesion.test.tsx` lee `cmd/api`,
`cmd/lambda`, `internal/aplicacion/autenticacion.go` e `infra/modules/api/variables.tf`, y falla si
alguno se separa; `ci.yml` vuelve a correr las etapas del frontend cuando una PR toca cualquiera de
esos archivos. Un valor distinto puesto en un entorno concreto (variable de entorno o de Terraform)
no lo ve esa prueba.

La alternativa era el servidor: que una peticion marcada como sondeo no corra `ultimo_uso`. Se
descarta por ahora porque cambia el contrato del puerto `Sesiones` y la API, toca el mismo `WHERE`
de `PorToken` que esta cambiando #222, y no evita que una pestana oculta siga pidiendo. Si se
quiere que la inactividad sea exacta tambien en Distribucion, esa es la salida.

## Enmienda a la 0013

La `0013` sigue vigente: token opaco, resumen SHA-256, revocar es `DELETE`. Esta decision le anade
dos cosas al contrato del puerto `Sesiones`:

- una sesion vale mientras no caduque **y** se haya usado dentro de la inactividad maxima;
- un usuario tiene **una** sesion viva: la nueva sustituye a las anteriores. Esto contradice la
  prueba que habia ("dos sesiones del mismo usuario conviven"), que se reemplazo por
  `TestCrearRevocaLasSesionesPreviasDelUsuario`.

## Alternativas consideradas

**Restaurar el aviso.** Le dice a la persona que hay un riesgo pero no le da nada que hacer. El
product owner lo descarto.

**Solo reintentar en el cliente.** Mejora el caso de un fallo puntual, pero sin red el token sigue
vivo 12 h. Por eso va junto con las mitigaciones del servidor, no en su lugar.

**No borrar el token si el servidor no confirma.** Deja la sesion abierta en el equipo compartido,
que es peor que el riesgo que se quiere cubrir.

**Varias sesiones por usuario, con "cerrar mis otras sesiones".** Necesita interfaz y endpoint
nuevos. Revocar al iniciar sesion da la misma garantia sin superficie nueva; el coste es que entrar
en el movil cierra la sesion del portatil.

## Consecuencias

Si el `DELETE` falla tres veces, un token copiado fuera del navegador vale como mucho
`SESION_INACTIVIDAD` sin uso, o hasta que su dueno vuelva a iniciar sesion, y nunca mas de
`SESION_TTL`.

En una pantalla que no sondea, la sesion cae `SESION_INACTIVIDAD` despues de la ultima peticion, y
la siguiente accion de la persona la lleva a la pantalla de entrada. En Distribucion no es exacto:
el ultimo sondeo sale antes de `SESION_INACTIVIDAD` sin teclado ni puntero, y la inactividad del
servidor corre desde ese sondeo. Con la pestana a la vista la sesion cae, como mucho, dos veces
`SESION_INACTIVIDAD` (60 min con los valores de hoy) despues de la ultima accion de la persona. Si
la pestana se oculta, el sondeo para en el siguiente tic y la sesion cae `SESION_INACTIVIDAD`
despues. Quien vuelve antes de que caiga sigue dentro sin volver a entrar. Antes de #223 esa
pantalla la sostenia hasta `SESION_TTL`.

Si eso molesta, se sube `SESION_INACTIVIDAD`; no se quita. Si se cambia, se cambia tambien
`SESION_INACTIVIDAD_MS`. Un sondeo nuevo que no pase por `useSondeo` vuelve a abrir #223, y no hay
ninguna comprobacion automatica que lo impida.

Un usuario no puede tener dos sesiones a la vez. Si se necesita, la salida es "cerrar mis otras
sesiones" y una decision nueva que sustituya esta.

Sigue pendiente que un administrador pueda cerrar todas las sesiones de otro usuario desde el
producto. Con la mitigacion 3, el propio usuario ya lo consigue volviendo a entrar.
