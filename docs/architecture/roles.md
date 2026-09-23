# Roles y capacidades

Los cinco `aplicacion.Rol` son los roles del reglamento, no roles inventados
de aplicacion. El middleware `requiereRol` decide contra este valor. ADR 0008
nombra las firmas de las compuertas; OE-6 recorta lo que ve un titular.

| `aplicacion.Rol` | Rol del reglamento | Capacidad |
| ---------------- | ------------------ | --------- |
| `administrador` | Consejo Directivo | Opera el pipeline. Firma de anticipos (`RA 3.2`). Lectura de auditoria. |
| `distribucion` | Distribucion | Co-firma de las compuertas: reclamaciones, pagos al exterior, ONI (`RD 14.5.10-12`, `RD 13.6`, `RD 13.8.6`). |
| `contabilidad` | Contabilidad | La otra firma de las mismas compuertas. Una sola persona no puede ostentar los dos. |
| `auditor` | Revisor Fiscal | Lectura de todo. No opera el pipeline ni firma. |
| `titular` | titular | Solo las obras donde tiene participacion registrada (`OE-6`). |

Las rutas se agrupan por capacidad, no por caso de uso. Quien anada un
endpoint lo mete en el grupo que le corresponde; el chequeo no se escribe
en el handler. Ese chequeo de grupo es grueso: `requiereRol` no sustituye
a la autorizacion dentro del caso de uso. El middleware solo cierra la
puerta del prefijo; la autorizacion fina vive con el caso de uso.

| Prefijo | Roles |
| ------- | ----- |
| `/admin/*` | `administrador` |
| `/auditoria/*` | `auditor`, `administrador` |
| `/obras/*` | `administrador` |
| `/recaudo/*` | `contabilidad`, `administrador` |
| `/bolsas/*` | `contabilidad`, `administrador`, `distribucion`, `auditor` |
| `/reportes/*` | `administrador` |
| `GET /procesos/*` | `administrador`, `distribucion`, `contabilidad`, `auditor` |
| `POST /procesos`, `POST /procesos/{id}/avanzar` | `administrador` |
| `POST /procesos/{id}/firmar`, `POST /procesos/{id}/rechazar` | `distribucion`, `contabilidad` |

`/recaudo/*` y `/bolsas/*` son el mismo modulo partido por capacidad, y el
corte es deliberado: por `/recaudo/*` **entra dinero**, asi que escribe
`contabilidad` —quien factura (`RD 13.5`)— y nadie mas. `/bolsas/*` solo lee,
y ahi entra `distribucion`, que necesita la bolsa para correr el reparto, y
`auditor`, que lee todo.

`distribucion` no registra recaudo a proposito. Es la **otra** firma de las
compuertas del `RD 13.5`, y una sola persona no puede ostentar las dos: quien
co-firma la salida del dinero no debe poder declarar cuanto entro.

`titular` queda fuera de los dos, lectura incluida: solo ve las obras donde
participa (`OE-6`), no el ingreso de la sociedad.

`/obras/*` es el catalogo maestro, y pide `administrador` tambien para
LEER. No es un descuido: el catalogo es el cubo contra el que resuelve todo
el matching, y quien lo lista entero ve el repertorio completo de la
sociedad. Abrirlo a `auditor` —que tiene lectura de todo— o recortarlo para
`titular` con `SoloPropiasObras` (`OE-6`) son decisiones de los issues que
traigan esos paneles.

`/reportes/*` es la ingesta de reportes de uso —subida, listado de cargas y el
log de rechazos de cada carga— y pide `administrador` por lo mismo que el
catalogo: una entrega pondera el reparto de un periodo entero, y el listado
deja ver de que fuentes vive la sociedad. La pantalla de ingesta (#29) es solo
de administrador, y lo que la cierra de verdad es este grupo del servidor, no
el menu del cliente.

`SoloPropiasObras` no es un grupo de rutas: es el predicado que los
endpoints de datos aplican cuando el actor es titular. Se compara
`TitularID`, no el id de usuario.

`/procesos/*` es el flujo de aprobaciones de `RD 13.5` (#34), y se parte en
tres grupos por la misma razon que `/recaudo` y `/bolsas`: `administrador`
OPERA el pipeline -abre una corrida y avanza sus etapas-, y `distribucion`/
`contabilidad` son las dos firmas de sus compuertas, no quien lo opera. El
rol con el que se firma sale de la SESION del actor, nunca de un campo del
cuerpo: si el cliente pudiera elegirlo, un actor de `contabilidad` podria
firmar "como `distribucion`" y la doble firma dejaria de separar a dos
personas. La lectura la comparten los cuatro roles del modulo, `auditor`
incluido, porque leer en que etapa esta una corrida no mueve nada.
