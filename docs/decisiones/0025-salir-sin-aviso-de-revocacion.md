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

Quien deja una pestana abierta mas de 30 minutos sin hacer nada vuelve a la pantalla de entrada. Si
eso molesta, se sube `SESION_INACTIVIDAD`; no se quita.

Un usuario no puede tener dos sesiones a la vez. Si se necesita, la salida es "cerrar mis otras
sesiones" y una decision nueva que sustituya esta.

Sigue pendiente que un administrador pueda cerrar todas las sesiones de otro usuario desde el
producto. Con la mitigacion 3, el propio usuario ya lo consigue volviendo a entrar.
