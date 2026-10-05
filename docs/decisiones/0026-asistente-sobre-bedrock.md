# 0026 El asistente usa Claude Haiku 4.5 en Amazon Bedrock por un endpoint de VPC

Fecha: 2026-10-05
Estado: Vigente. La base legal de la transferencia internacional (Ley 1581 de 2012) esta
**pendiente de confirmar por el area juridica de REDES**.

> Numero: la revision de #216 pidio "ADR 0025", pero `0025` ya lo ocupa
> `0025-salir-sin-aviso-de-revocacion.md` en la rama de #215. Dos ADR con el mismo numero no
> chocan en git (ver `README.md`), asi que este toma el primero libre.

## Contexto

El asistente de solo lectura (#66) es la primera vez que Intela envia datos a un modelo de
lenguaje externo y la primera ruta que cuesta dinero por llamada. Tres hechos condicionan la
decision:

1. **La red.** La Lambda `intela-api` vive en subredes privadas sin Internet Gateway ni NAT;
   solo tiene el endpoint gateway de S3 (`infra/modules/network`). Desde ahi
   `api.anthropic.com` no es alcanzable: con `ANTHROPIC_API_KEY` cargada, cada pregunta
   esperaria el plazo (25 s) y responderia "no disponible", pagando ese tiempo de Lambda.
2. **Los datos.** Al modelo le llegan el texto de la conversacion y, desde #67/#68/#69, los
   resultados de las herramientas: fragmentos de reglamento, pero tambien titulares, obras y
   cifras de reparto.
3. **El dinero.** Cada pregunta son hasta `MaxTurnosAgente` = 5 llamadas pagas al modelo.

## Decision

### Proveedor y modelo

- **Produccion: Claude Haiku 4.5 en Amazon Bedrock**, por la API Converse, con el perfil de
  inferencia entre regiones `us.anthropic.claude-haiku-4-5-20251001-v1:0`
  (`internal/infraestructura/modelolenguaje/bedrock`). Haiku porque el asistente consulta y
  resume: no razona sobre el reparto, que calcula el motor. Es el modelo rapido y barato de la
  familia, y soporta tool use nativo.
- **La API directa de Anthropic queda soportada pero no es la de produccion**
  (`AGENTE_PROVEEDOR=anthropic`): desde la VPC no tiene red (punto 1). Sirve en local o en
  un despliegue con salida a internet.
- **`falso`** sigue siendo el de CI, E2E y desarrollo sin proveedor.

`AGENTE_PROVEEDOR` (`bedrock` | `anthropic` | `falso`) elige el adaptador en
`modelolenguaje.Elegir`; `AGENTE_MODELO` cambia el modelo. Un valor desconocido o una
configuracion que no carga dejan el asistente en "no disponible": el arranque nunca se cae.

**Como se cambia de proveedor.** El puerto `aplicacion.ModeloLenguaje` es neutral (mensajes,
esquemas de herramienta, llamadas y resultados). Otro proveedor es un paquete nuevo en
`internal/infraestructura/modelolenguaje/` y un `case` en `Elegir`; `aplicacion` no se toca.
Otro modelo de Bedrock es `AGENTE_MODELO` mas ampliar la politica IAM de `infra/envs/nheo`.

### Red: endpoint de interfaz, no NAT

Un endpoint de interfaz `bedrock-runtime` con DNS privado, en **una sola subred**, con un
security group que solo admite 443 desde el de las Lambdas. Las subredes siguen sin ruta a
internet.

| Opcion | Costo aprox. | Efecto |
| ------ | ------------ | ------ |
| Endpoint de interfaz, 1 AZ (elegida) | USD 7,20/mes + USD 0,01/GB | Trafico dentro de AWS, autenticado por IAM, sin secreto |
| Endpoint en las 2 AZ | USD 14,40/mes | Sobrevive a la caida de una AZ; no lo justifica un asistente |
| NAT Gateway | ~USD 32/mes + trafico | Rompe el presupuesto de USD 20 de `nheo` y abre salida a internet a toda la API |
| Agente en una Lambda fuera de la VPC | 0 | Deja de servir cuando las herramientas leen Postgres (#67, #68) |

Si cae la AZ del endpoint, cae el asistente; el resto de la API no depende de el.

### Permisos

Politica en linea en el rol de `intela-api`, solo `bedrock:InvokeModel` (Converse se autoriza con
esa accion; no se usa streaming):

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

Con Bedrock los datos **no salen de la cuenta de AWS de REDES ni llegan a Anthropic**: el
proveedor del modelo no ve las peticiones, y Bedrock no las usa para entrenar ni las guarda
(salvo que se active el registro de invocaciones, que no esta activo). Pero el perfil `us.`
procesa en **us-east-1, us-east-2 o us-west-2**, es decir, **fuera de Colombia**.

### Base legal (pregunta abierta)

Enviar a una region de AWS en Estados Unidos datos personales de titulares (nombres, cifras de
ingresos) puede ser una **transferencia internacional de datos personales** en el sentido de la
Ley 1581 de 2012 (art. 26) o, segun como se lea la relacion con AWS, una transmision a un
encargado. Este ADR **no concluye** cual aplica ni si hace falta autorizacion del titular,
clausula contractual o declaracion de conformidad.

- **Pregunta abierta:** ¿la ejecucion del modelo en regiones de EE. UU. por medio de AWS
  Bedrock tiene base legal suficiente bajo la Ley 1581 y la politica de tratamiento de REDES?
- **Duena:** area juridica de REDES SGC. Registrada como P-23 en
  `docs/dominio/preguntas-cliente.md`.
- **Mientras no se confirme:** el asistente puede operar con herramientas que no devuelven
  datos personales (reglamento, #67). Las herramientas que devuelven titulares o cifras
  (#68, #69) no se despliegan a produccion sin esa respuesta.

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
- **Anthropic directo:** si alguna vez se usa, tope de gasto mensual en la consola de Anthropic
  para la clave del despliegue.

## Consecuencias

- Produccion no necesita el secreto `ANTHROPIC_API_KEY`; sigue declarado para el proveedor
  opcional.
- Se paga un endpoint de interfaz (~USD 7,20/mes) dentro de un presupuesto de USD 20.
- Prerrequisito de cuenta: Bedrock exige enviar una vez el formulario de caso de uso de
  Anthropic antes de invocar sus modelos (ver `docs/cd.md`).
- El prompt cacheado que usa el adaptador de Anthropic no se replica en Bedrock: el prefijo de
  sistema es corto y no alcanza el minimo cacheable.
- Queda abierta la pregunta legal, con duena, y condiciona el despliegue de #68 y #69.
