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
en el handler.

| Prefijo | Roles |
| ------- | ----- |
| `/admin/*` | `administrador` |
| `/auditoria/*` | `auditor`, `administrador` |

`SoloPropiasObras` no es un grupo de rutas: es el predicado que los
endpoints de datos aplican cuando el actor es titular. Se compara
`TitularID`, no el id de usuario.
