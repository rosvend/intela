# 0026 El asistente usa la API de Anthropic por un NAT Gateway; Bedrock es la alternativa prevista

Fecha: 2026-10-05
Estado: Vigente. Revisada el mismo dia: la primera version elegia Bedrock; la cuenta no ha enviado
el formulario de caso de uso de Anthropic, asi que produccion usa la API de Anthropic. La base
legal de la transferencia internacional (Ley 1581 de 2012) esta **pendiente de confirmar por el
area juridica de REDES (P-23) y ahora es mas urgente**: los datos salen de AWS.

> El nombre del fichero conserva "bedrock" para no romper enlaces.

> Numero: la revision de #216 pidio "ADR 0025", pero `0025` ya lo ocupa
> `0025-salir-sin-aviso-de-revocacion.md` en la rama de #215. Dos ADR con el mismo numero no
> chocan en git (ver `README.md`), asi que este toma el primero libre.

## Contexto

El asistente de solo lectura (#66) es la primera vez que Intela envia datos a un modelo de
lenguaje externo y la primera ruta que cuesta dinero por llamada. Tres hechos condicionan la
decision:

1. **La red.** La Lambda `intela-api` vive en subredes privadas; antes de esta decision no
   tenian Internet Gateway ni NAT, solo el endpoint gateway de S3 (`infra/modules/network`), asi
   que `api.anthropic.com` no era alcanzable sin salida a internet.
2. **Los datos.** Al modelo le llegan el texto de la conversacion y, desde #67/#68/#69, los
   resultados de las herramientas: fragmentos de reglamento, pero tambien titulares, obras y
   cifras de reparto.
3. **El dinero.** Cada pregunta son hasta `MaxTurnosAgente` = 5 llamadas pagas al modelo.

## Decision

### Proveedor y modelo

- **Produccion: Claude Haiku 4.5 por la API directa de Anthropic**
  (`AGENTE_PROVEEDOR=anthropic`, secreto `ANTHROPIC_API_KEY`), saliendo por un NAT Gateway
  (siguiente seccion). Haiku porque el asistente consulta y resume: no razona sobre el reparto,
  que calcula el motor. Es el modelo rapido y barato de la familia y soporta tool use nativo.
- **Bedrock queda como alternativa prevista** (`AGENTE_PROVEEDOR=bedrock`, perfil
  `us.anthropic.claude-haiku-4-5-20251001-v1:0`, `modelolenguaje/bedrock`). Hoy esta bloqueado:
  la cuenta 635867291313 no ha enviado el formulario de caso de uso de Anthropic y Converse
  responde `ResourceNotFoundException`. La politica IAM de Haiku se conserva para el cambio.
- **`falso`** sigue siendo el de CI, E2E y desarrollo sin proveedor.

`AGENTE_PROVEEDOR` (`bedrock` | `anthropic` | `falso`) elige el adaptador en
`modelolenguaje.Elegir`; `AGENTE_MODELO` cambia el modelo. Un valor desconocido o una
configuracion que no carga dejan el asistente en "no disponible": el arranque nunca se cae.

**Como se cambia de proveedor.** El puerto `aplicacion.ModeloLenguaje` es neutral (mensajes,
esquemas de herramienta, llamadas y resultados). Otro proveedor es un paquete nuevo en
`internal/infraestructura/modelolenguaje/` y un `case` en `Elegir`; `aplicacion` no se toca.
Otro modelo de Bedrock es `AGENTE_MODELO` mas ampliar la politica IAM de `infra/envs/nheo`.

**Como se vuelve a Bedrock.** Enviar el formulario de caso de uso (consola de Bedrock -> Model
catalog -> Claude Haiku 4.5), poner `agente_proveedor = "bedrock"`, comprobar una pregunta real y
entonces `enable_nat = false`. Si se apaga el NAT, Titan (#67) necesita volver a tener salida: o
se deja el NAT, o se reintroduce un endpoint de interfaz `bedrock-runtime` (~USD 7,20/mes/AZ).

### Red: NAT Gateway

Las subredes privadas de la Lambda no tenian ruta a internet. `infra/modules/network` crea,
con `enable_nat` (por defecto `true` en `nheo`): una subred publica, un Internet Gateway, **un**
NAT Gateway en una sola AZ con su IP elastica y la ruta `0.0.0.0/0` de la tabla privada. El
security group de las Lambdas pasa de "todo" a 443 hacia cualquier destino y 5432 hacia el
security group de la base. RDS y la Lambda de migraciones usan rutas internas de la VPC
(Postgres) y el endpoint gateway de S3; el NAT no interviene en ellas. El endpoint de interfaz
`bedrock-runtime` de la primera version se elimino: con el NAT es redundante.

| Opcion | Costo aprox. | Efecto |
| ------ | ------------ | ------ |
| NAT Gateway, 1 AZ (elegida) | ~USD 32/mes + USD 0,045/GB | Salida a internet para el asistente (Anthropic) y Titan; si cae su AZ, cae el asistente |
| Endpoint de interfaz `bedrock-runtime` | USD 7,20/mes/AZ | Solo sirve con Bedrock, hoy bloqueado |
| Agente en una Lambda fuera de la VPC | 0 | Deja de servir cuando las herramientas leen Postgres (#67, #68) |

**Costo.** El NAT suma ~USD 32/mes mas trafico a los ~USD 14 de RDS: el total esperado pasa de
~USD 15 a ~USD 45-50/mes. Supera el presupuesto de USD 20 de `nheo`; **la persona responsable del
proyecto aprobo el sobrecosto de forma explicita**. El ejemplo de `monthly_budget_usd` sube a 45.
Ademas, el NAT abre salida a internet (solo 443) a toda la Lambda, no solo al asistente.

### Permisos

Politica en linea en el rol de `intela-api`, solo `bedrock:InvokeModel` (Converse se autoriza con
esa accion; no se usa streaming). Con Anthropic directo solo se usa la ultima entrada (Titan, que no
exige formulario y sale por el NAT); las de Haiku se conservan para volver a Bedrock:

- el perfil de inferencia `us.anthropic.claude-haiku-4-5-20251001-v1:0` de la cuenta;
- el modelo base `anthropic.claude-haiku-4-5-20251001-v1:0` en `us-east-1`, `us-east-2` y
  `us-west-2` (las regiones a las que enruta el perfil, segun `GetInferenceProfile`), **solo**
  si la llamada llega por ese perfil (`bedrock:InferenceProfileArn`);
- `amazon.titan-embed-text-v2:0` en `us-east-1`, para los embeddings de #67.

### Que datos salen y a donde

| Dato | Sale | Nota |
| ---- | ---- | ---- |
| Pregunta e historial que reenvia el navegador | Si | Texto libre: puede traer lo que el usuario escriba |
| Resultados de herramientas (#67/#68/#69) | Si | Pueden incluir nombres de titulares, obras y cifras |
| Rol de quien pregunta | Si | Es lo unico del actor que se manda |
| Nombre, correo, id del usuario | **No** | `contextoDeActor` manda solo el rol; una prueba lo fija |
| Argumentos de herramienta en el log | **No** | El log guarda nombre de la herramienta y tamano de los argumentos |

**Con la API de Anthropic, el texto de la conversacion y los resultados de las herramientas
salen de AWS y llegan a Anthropic, un tercero fuera de AWS y fuera de Colombia.** No quedan dentro
de la cuenta de REDES. Anthropic los procesa bajo sus condiciones comerciales de API (consultar
su politica vigente de retencion y de uso para entrenamiento antes de produccion). La clave viaja
en una cabecera HTTPS; no se registra en el log.

Con Bedrock (la alternativa) los datos no habrian salido de la cuenta de AWS ni llegado a
Anthropic, aunque el perfil `us.` procesa en us-east-1, us-east-2 o us-west-2, fuera de Colombia.

### Base legal (pregunta abierta)

Enviar a Anthropic (un tercero, en Estados Unidos) datos personales de titulares (nombres, cifras
de ingresos) es, con alta probabilidad, una **transferencia o transmision internacional de datos
personales** en el sentido de la Ley 1581 de 2012 (art. 26), y con un tercero distinto de AWS. Este ADR **no concluye** cual aplica ni si hace falta autorizacion del titular,
clausula contractual o declaracion de conformidad.

- **Pregunta abierta:** ¿enviar a Anthropic (API directa, EE. UU.) la conversacion y resultados
  de herramientas tiene base legal suficiente bajo la Ley 1581 y la politica de tratamiento de
  REDES?
- **Duena:** area juridica de REDES SGC. Registrada como P-23 en
  `docs/dominio/preguntas-cliente.md`.
- **Mientras no se confirme:** el asistente puede operar con herramientas que no devuelven
  datos personales (reglamento, #67). **Recomendacion:** las herramientas que devuelven titulares
  o cifras (#68, #69) no se habilitan en produccion hasta responder P-23, salvo que REDES asuma el
  riesgo de forma explicita y por escrito. Con Bedrock el riesgo era menor; con Anthropic directo
  es mayor, por eso la pregunta es ahora mas urgente.

### El agente nunca autoriza ni escribe

El agente es de solo lectura: no firma compuertas ni mueve dinero (`RD 13.5`). Cada herramienta
envuelve un caso de uso de `aplicacion`, que es donde vive el RBAC; la regla depguard
`herramientas-sin-dominio` (`.golangci.yml`) impide que una herramienta lea el dominio
directamente y se salte ese control. Un `ErrNoAutorizado` corta el bucle sin devolverle nada
al modelo.

### Control de costo

- **Por pregunta:** como mucho 5 llamadas al modelo y 2048 tokens de salida por llamada; un
  plazo (`AGENTE_PLAZO`) para la pregunta entera.
- **Por usuario:** 20 preguntas por minuto. **Es por instancia**: el limite vive en la memoria
  de cada proceso, y en Lambda cada entorno caliente tiene el suyo, asi que el tope real es
  20 x instancias calientes (acotadas por `reserved_concurrency`, 10). Un limite compartido
  (tabla en Postgres) es trabajo aparte.
- **Presupuesto:** `infra/modules/budget` alerta sobre el gasto con la etiqueta
  `Project=intela`. El endpoint lleva la etiqueta; **las invocaciones a Bedrock por un perfil
  del sistema no**, asi que ese presupuesto no las ve. Hace falta, aparte, un presupuesto de
  cuenta filtrado por servicio Amazon Bedrock (o un perfil de inferencia de aplicacion
  etiquetado). Las cuotas de servicio de Bedrock (tokens y peticiones por minuto) son el tope
  duro. Ver `docs/cd.md`.
- **Anthropic directo (produccion):** poner un tope de gasto mensual en la consola de Anthropic
  para la clave del despliegue. Es el tope duro del gasto del modelo.
- **NAT:** ~USD 32/mes fijos mas trafico, etiquetado `Project=intela`: el presupuesto de
  `infra/modules/budget` si lo ve (ejemplo: USD 45/mes).

## Consecuencias

- Produccion necesita el secreto `ANTHROPIC_API_KEY` (ya cableado hasta la Lambda como variable
  de entorno `sensitive` en Terraform).
- Se paga un NAT Gateway (~USD 32/mes + trafico), por encima del presupuesto original de USD 20;
  el sobrecosto esta aprobado.
- La Lambda tiene salida a internet por 443; los datos del asistente salen de AWS.
- El prompt cacheado del adaptador de Anthropic si aplica en esta ruta.
- Queda abierta la pregunta legal (P-23), con duena, y condiciona el despliegue de #68 y #69.
- Volver a Bedrock exige el formulario de caso de uso y luego apagar el NAT (ver arriba).
