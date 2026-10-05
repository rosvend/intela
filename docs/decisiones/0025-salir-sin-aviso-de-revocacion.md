# 0025 "Salir" no avisa si el servidor no confirmo la revocacion

Fecha: 2026-10-05
Estado: Vigente

## Contexto

La `0013` hace de la sesion un token opaco en tabla: revocarla es `DELETE /auth/session`, que
borra la fila de `sesiones`. Si ese `DELETE` no llega (sin red) o falla (5xx), la fila sigue
ahi y el token vale hasta que caduque. `SESION_TTL` es de **12 h** por defecto
(`cmd/api/main.go`, `cmd/lambda/main.go`).

En la revision de #71 se pidio que "Salir" lo dijera, y `b47a131` anadio el aviso: *"Se cerro
la sesion en este equipo, pero el servidor no confirmo la revocacion..."*, con tres pruebas en
`sesion.test.tsx` y dos en `Login.test.tsx`. El rediseno de la PR #215 lo quito, y la revision
de esa PR lo marco como bloqueante porque la decision no estaba escrita.

El caso que preocupa es un equipo compartido: quien sale no sabe que el token sigue vivo en el
servidor y no avisa a nadie.

## Decision

**El cliente siempre borra el token local y la interfaz no muestra el aviso.** `salir()` en
`web/src/sesion.tsx` intenta el `DELETE`, ignora el fallo, borra el token de `localStorage` y
limpia el usuario.

El aviso se quita por decision del product owner. Asustaba a la persona con algo que ella no
puede resolver: no tiene como reintentar la revocacion ni como cerrar sesiones desde la
interfaz, y en un portal de escritores la mayoria no sabria a quien avisar ni que pedir.

Mitigaciones con las que se cuenta hoy:

- **El token sale del equipo.** Tras "Salir" no queda en `localStorage`, asi que el siguiente
  que use el navegador no hereda la sesion. Es el riesgo principal del equipo compartido.
- **La caducidad es corta y configurable.** El token vale como mucho `SESION_TTL` (12 h por
  defecto). La caducidad la impone el `WHERE` de `PorToken` en el servidor, no el cliente.
- **Revocacion en el servidor, solo a mano.** No hay endpoint para que un administrador cierre
  las sesiones de otro usuario: el puerto `Sesiones` solo tiene `Revocar(token)`, y hace falta el
  token en claro. Lo unico posible es que un operador con acceso a la base ejecute
  `DELETE FROM sesiones WHERE usuario_id = ...` (hay un indice `sesiones_usuario`). Esto no
  es una funcion del producto y queda como deuda.

## Alternativas consideradas

**Restaurar el aviso, con otro estilo.** Es lo que propuso la revision. Se descarta porque el
problema no era el estilo sino el mensaje: le dice a la persona que hay un riesgo, pero no le da
nada que hacer.

**Reintentar el `DELETE` en segundo plano.** Sin token en el cliente no hay con que reintentar, y
guardarlo para reintentar despues es justo lo que "Salir" tiene que evitar.

**No borrar el token si el servidor no confirma.** Deja la sesion abierta en el equipo
compartido, que es peor que el riesgo que se quiere cubrir.

## Consecuencias

Se acepta el riesgo residual: si el `DELETE` falla, un token que ya se hubiera copiado fuera del
navegador sigue valiendo hasta `SESION_TTL`, y nadie se entera. Que el `DELETE` falle no basta;
ademas alguien tiene que tener una copia del token.

`Login.test.tsx` fija que no aparece el aviso. Si se quiere volver a mostrarlo, se escribe un ADR
que sustituya a este.

Queda pendiente, sin issue todavia: una forma de que un administrador cierre todas las sesiones de
un usuario (por ejemplo `DELETE` por `usuario_id` detras de un endpoint de administracion). Con
eso, la tercera mitigacion pasaria a ser una funcion del producto y no una consulta a mano.
