# 0013 La sesion es un token opaco en tabla, no un JWT

Fecha: 2026-08-30
Estado: Vigente

## Contexto

Antes de que exista cualquier endpoint operativo hace falta autenticacion: una forma de que un
administrador, un titular o un auditor obtengan una sesion y la presenten en la peticion
siguiente. El esquema ya traia una tabla `sesiones` y el puerto `Sesiones` declarado, pero sin
implementar y sin una decision escrita detras.

Es una decision que hay que poder defender ante una auditoria de `RD 16`, y que alguien va a
querer revisar en cuanto lea que no hay JWT. Conviene que la respuesta este escrita y no haya que
reconstruirla leyendo el adaptador.

Dos hechos acotan el problema. El primero: la `0003` ya midio la carga —el pico es anual, no
sostenido— y decidio un monolito modular con un solo binario. No hay presion de escala horizontal,
que es el argumento que normalmente inclina la balanza hacia sesiones sin estado. El segundo: esta
API autoriza movimientos de dinero de terceros, asi que **poder cortar una sesion ya emitida** vale
mas que ahorrarse una consulta.

## Decision

**La sesion es un token opaco de 256 bits, guardado como resumen SHA-256 en la tabla `sesiones`.**

Tres partes:

1. **Opaco y en tabla, no JWT.** El token no lleva informacion: es una clave para buscar una fila.
   Revocar es `DELETE`, y surte efecto en la peticion siguiente.
2. **Guardado hasheado.** En `sesiones.token` va el SHA-256 hexadecimal, no el token. Quien lea la
   tabla —una copia de seguridad, un volcado de depuracion, una inyeccion de solo lectura— no
   consigue nada con lo que se pueda suplantar a nadie.
3. **SHA-256 y no bcrypt** para ese resumen, que es la parte contraintuitiva. Ver abajo.

La caducidad la comprueba el `WHERE` de la consulta, con el instante que entra por `PuertoReloj`
(`0005`), no con `now()`: asi una sesion caducada se prueba moviendo el reloj y no esperando.

El token en claro existe una sola vez, en la respuesta del login. De la base no se puede volver a
el.

## Alternativas consideradas

**JWT o sesiones sin estado.** Es lo que casi todo el mundo elige, y aqui el intercambio sale al
reves. Un JWT vale hasta que caduca: para cortar una sesion comprometida antes de tiempo hay que
montar una lista de revocacion, que es *otra vez* una consulta a una tabla en cada peticion —
exactamente el coste que el JWT prometia evitar, mas la complejidad de la firma, la rotacion de
claves y la eleccion de algoritmo. Lo que compra a cambio, verificar sin tocar la base, solo se
cobra cuando hay varias instancias sin estado compartido, y la `0003` dice que no las hay ni se
esperan.

**bcrypt para el resumen del token.** Es lo que usa el adaptador de contrasenas de al lado, asi que
parece lo coherente. Es la eleccion equivocada por dos razones independientes:

- bcrypt lleva sal aleatoria, asi que el resumen **no es determinista** y no se puede buscar por
  clave primaria. Habria que traerse todas las sesiones vivas y compararlas una a una en cada
  peticion autenticada.
- bcrypt existe para **estirar secretos de baja entropia**, que es lo que es una contrasena elegida
  por una persona: encarece el intento para que un diccionario no sea viable. Este token trae 256
  bits de `crypto/rand`; no hay diccionario ni fuerza bruta que lo alcance, y encarecer el resumen
  no compra nada. Aqui solo hace falta que la funcion sea de un solo sentido.

**Guardar el token en claro.** Ahorra una linea. A cambio, cualquier lectura de la tabla es una
suplantacion inmediata de todas las sesiones abiertas.

**Autenticacion de terceros (Auth0, Keycloak).** Descartada para un sistema academico de un solo
inquilino: anade una dependencia externa, secretos que gestionar y un punto de fallo, para un
padron de usuarios que cabe en una tabla.

## Consecuencias

Cada peticion autenticada hace una consulta a `sesiones` con un `JOIN` a `usuarios`. Es una lectura
por clave primaria y trae el usuario entero de una vez, asi que el middleware no necesita un
segundo viaje; con el volumen de la `0003` no es un problema, y si algun dia lo fuera, una cache
con TTL corto delante es un cambio local al adaptador.

Revocar una sesion es inmediato y no hace falta nada mas: ni lista negra, ni esperar a que caduque
un token, ni rotar una clave de firma.

El hasheo vive en el **adaptador**, no en el caso de uso: "no guardar el secreto en claro" es una
propiedad del almacenamiento. Los tres metodos del puerto `Sesiones` siguen significando "token en
claro" y el nucleo no aprende que existe SHA-256, igual que no sabe que existe bcrypt.

Queda pendiente, y con issue propio: limitar el ritmo de intentos de login. Esta decision no lo
resuelve — de momento los fallos se registran con `slog`, con el correo y sin la clave.

Y queda una consecuencia que hay que recordar cuando se toque el esquema: `sesiones` lleva
`CHECK (expira > creada)` con `creada DEFAULT now()`. `Crear` deja esa columna en su valor por
defecto a proposito; rellenarla desde el reloj inyectado mientras la otra sale del reloj del
servidor haria que una diferencia de relojes rechazara un login legitimo.
