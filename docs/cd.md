# Despliegue continuo

El despliegue vive en el mismo `ci.yml` que la integracion, al final, y **solo corre en `push` a
`main`**. Despliega de verdad desde el [ADR 0014](decisiones/0014-infraestructura-serverless-en-aws.md),
que eligio el proveedor que faltaba: AWS serverless, descrito en Terraform bajo
[`infra/`](../infra/README.md).

Que sale por la puerta:

| Pieza | Donde acaba |
| ----- | ----------- |
| API (`cmd/lambda`) | Lambda `provided.al2023` arm64, con Function URL |
| Esquema (`cmd/lambda-migrate`) | Lambda dentro de la VPC, invocada por Terraform |
| Tablero (`web/dist`) | Amplify Hosting, que ademas reescribe `/api/*` hacia la Function URL |

Un check verde aqui significa que el release se aplico **y que `/api/health` y `/api/ready`
respondieron 200**. El ultimo paso falla el job si no se recuperan.

## Por que esta en `ci.yml` y no en su propio workflow

Porque el release no puede empezar antes de que la compuerta este verde, y `needs: [ci]` es la unica
forma directa de garantizarlo.

La alternativa habitual, `workflow_run`, tiene dos problemas que aqui pesan mas que la separacion de
ficheros: se resuelve contra la copia del workflow que hay en la **rama por defecto**, no contra la
del PR, y no aparece en el PR. Estarias revisando un camino de release que no es el que corre.

## Las etapas

| Job | Cuando | Que hace |
| --- | ------ | -------- |
| `Docker build (backend)` | PR y `main` | En PR construye y descarta. En `main` publica a GHCR |
| `Docker build (frontend)` | PR y `main` | Igual, para `web/Dockerfile` |
| `Infrastructure` | PR y `main` | Valida cada modulo aislado. En PR, ademas planifica y comenta |
| `Deploy (production)` | Solo `push` a `main`, tras `ci` | Aplica Terraform, sube el tablero y verifica salud |

**Verificar y publicar son la misma etapa** (`container.yml`), con `push: false` en PR y `push: true`
en `main`. Separarlas en dos workflows las dejaria divergir, y construiria cada imagen dos veces en
`main`. La primera noticia de la divergencia seria una imagen que paso CI y falla al construir en el
camino de release.

## Las imagenes

Se publican en **GHCR**, que no necesita ningun secreto propio: `container.yml` se autentica con el
`GITHUB_TOKEN` del propio job, con permiso `packages: write`.

```text
ghcr.io/rosvend/intela-api:sha-<sha completo>    inmutable, es la que se anota en el release
ghcr.io/rosvend/intela-api:main                  puntero movil, comodidad
ghcr.io/rosvend/intela-web:sha-<sha completo>
ghcr.io/rosvend/intela-web:main
```

Las etiquetas se calculan dentro de `container.yml` en vez de con `docker/metadata-action`, para que
la referencia que el workflow devuelve como `output` sea exactamente la que empujo.

> **Las imagenes no son lo que se despliega.** El release serverless empaqueta el mismo codigo como
> un zip de Lambda. GHCR se queda por dos razones: `docker compose up` es un entregable del proyecto
> (`docs/context.md`), y construir la imagen es como se verifica que sigue construyendo. `deploy.yml`
> recibe las dos referencias y las anota en el resumen, sin desplegarlas.

## El orden, y por que migrar no es un paso propio

El orden que este documento pedia sigue siendo el mismo, y sigue sin ser decorativo:

1. **Autenticar** contra AWS, por OIDC.
2. **Migrar** la base, *antes* de que el codigo nuevo sirva trafico, y de forma compatible hacia
   atras. El [ADR 0008](decisiones/0008-reparto-como-flujo-con-aprobaciones.md) hace del reparto un
   flujo de varias etapas con aprobaciones: una corrida en vuelo durante un despliegue no puede
   encontrarse un esquema que su codigo no conoce.
3. **Desplegar** el codigo ya construido y verificado. Este paso selecciona, nunca reconstruye.
4. **Verificar** salud, y fallar el job si no se recupera.

Lo que cambio es donde vive el paso 2. **Migrar y desplegar la API ocurren los dos dentro del mismo
`terraform apply`**, y el orden entre ellos lo impone el grafo de dependencias, no este fichero:

```hcl
module "api" {
  depends_on = [module.migrations]
}
```

Partirlos en dos pasos de shell haria que la garantia fuese una propiedad de un YAML que nadie lee
durante un incidente. Asi es una propiedad del grafo, y falla ruidosamente: si goose falla, el apply
falla, la funcion de la API no se actualiza y el codigo viejo sigue sirviendo el esquema viejo.

El tablero se sube despues, con `aws amplify create-deployment` y su `start-deployment`, y el job
sondea el trabajo hasta `SUCCEED`: sin eso, el paso quedaria verde en cuanto termina la subida, que
no dice nada.

## La guarda de destruccion

Antes de aplicar —y antes de eso, en el `plan` de cada PR— el pipeline lee el plan en JSON y **se
niega a continuar si algo se destruye**:

```bash
terraform show -json tfplan \
  | jq -r '.resource_changes[]? | select(.change.actions | index("delete")) | .address'
```

`index("delete")` atrapa tambien un reemplazo, que es un borrado y una creacion. Para pasar por
encima hay que lanzar CI a mano con `confirm_destroy: yes`.

Vigila **cualquier** borrado y no solo los etiquetados `Project=intela`: todo lo que hay en ese
estado lleva la etiqueta por construccion, via `default_tags`, asi que filtrar por etiqueta solo
anadiria formas de que se le escape algo. La base tiene ademas dos cinturones mas: `prevent_destroy`
en su ciclo de vida y `deletion_protection` en la instancia.

## Donde van los secretos

**No hay ninguna credencial guardada.** Se entra por OIDC, y lo que se almacena son ARN de roles,
que no son secretos utiles por si solos: solo sirven a quien ya puede presentar un token de este
repositorio.

Dos roles, porque los dos trabajos necesitan poderes distintos:

| Secreto de entorno | Rol | Confia en |
| ------------------ | --- | --------- |
| `AWS_PLAN_ROLE_ARN` | Solo lectura, mas el bloqueo del estado | `repo:<owner>/<repo>:pull_request` |
| `AWS_DEPLOY_ROLE_ARN` | Gestion de los recursos del proyecto | `repo:<owner>/<repo>:ref:refs/heads/main` |

Un pull request lleva un `sub` distinto, asi que **no puede asumir el rol de despliegue por mucho
que edite el workflow en ese mismo PR**. Eso es lo que compra separarlos.

Van en secretos de **entorno** (`production`), no de repositorio: quedan acotados al entorno y una
corrida que apunte a otro sitio no puede leerlos. Ademas hace falta la variable de repositorio
`TF_STATE_BUCKET`, que no es secreta y la imprime `infra/bootstrap/`.

`id-token: write` ya se concede, en los dos jobs que lo usan y en ninguno mas — que era exactamente
la condicion que este documento ponia.

### El modelo del asistente

El asistente de solo lectura (#66) llama a un modelo de lenguaje. En produccion es **Claude
Haiku 4.5 en Amazon Bedrock** ([ADR 0026](decisiones/0026-asistente-sobre-bedrock.md)):
`infra/envs/nheo` fija `AGENTE_PROVEEDOR=bedrock` (variable `agente_proveedor`), la Lambda
llega a Bedrock por el endpoint de interfaz `bedrock-runtime` de `infra/modules/network`
(~USD 7,20/mes, una AZ) y se autentica con su rol: no hay secreto que cargar.

1. **Prerrequisito de cuenta, una sola vez.** Bedrock exige enviar el formulario de caso de uso
   de Anthropic (consola de Bedrock -> Model catalog -> Claude Haiku 4.5) antes de invocar sus
   modelos. Sin el, Converse responde `ResourceNotFoundException: Model use case details have
   not been submitted for this account` y el asistente contesta "no disponible". El 2026-10-05
   la cuenta de `nheo` todavia devolvia ese error.
2. **Permisos.** El rol de `intela-api` solo puede `bedrock:InvokeModel` sobre el perfil
   `us.anthropic.claude-haiku-4-5-20251001-v1:0`, el modelo base detras de el (us-east-1,
   us-east-2, us-west-2) y `amazon.titan-embed-text-v2:0`. Cambiar `AGENTE_MODELO` a otro
   modelo exige ampliar `data.aws_iam_policy_document.bedrock` en `infra/envs/nheo/main.tf`.
3. **Anthropic directo, opcional.** `ANTHROPIC_API_KEY` sigue declarado (repositorio ->
   `TF_VAR_anthropic_api_key`, `sensitive`) y se usa solo con `agente_proveedor = "anthropic"`.
   Desde estas subredes no hay ruta a `api.anthropic.com`, asi que en `nheo` no sirve. Si se
   usa en otro despliegue, poner un tope de gasto mensual a esa clave en la consola de Anthropic.
4. Sin proveedor que funcione el despliegue no falla: el asistente contesta "no disponible" y
   el resto de la API sirve igual.

#### Tope de gasto

- Cada pregunta son hasta 5 llamadas al modelo, de hasta 2048 tokens de salida cada una.
- El limite de 20 preguntas por usuario por minuto **es por instancia**: vive en la memoria del
  proceso, y en Lambda cada entorno caliente tiene el suyo. El tope real es 20 x instancias
  calientes, acotadas por `reserved_concurrency` (10). Un limite compartido es trabajo aparte.
- El presupuesto de `infra/modules/budget` filtra por la etiqueta `Project=intela`. El endpoint
  la lleva; **las invocaciones a Bedrock por un perfil del sistema no**, asi que ese
  presupuesto no ve el gasto del modelo. Crear a mano (o en un PR aparte) un presupuesto de
  cuenta filtrado por el servicio Amazon Bedrock, y vigilar las cuotas de servicio de Bedrock
  (tokens y peticiones por minuto del modelo), que son el tope duro.

`AGENTE_PLAZO` (25 s por defecto en el modulo) acota la pregunta entera por debajo del
`timeout_s` de la Lambda (30 s): al vencer, el usuario recibe un evento `error` en vez de un 502
mudo. Si en produccion se ven plazos vencidos, subir los dos a la vez (p. ej. 60 s y 55 s).

La Function URL esta en modo buffer: el cuerpo SSE de `/agente/consulta` llega entero al final, no
evento a evento. El panel lo pinta igual; para verlo en vivo habria que pasar la URL a
`RESPONSE_STREAM`, que hoy no soporta el adaptador de `cmd/lambda`.

## La compuerta de aprobacion

`deploy.yml` corre con `environment: production`. Anadir *required reviewers* a ese entorno en los
ajustes del repositorio convierte el job en una aprobacion manual **sin tocar el workflow**.

GitHub crea el entorno la primera vez que el job corre. Los secretos de arriba hay que cargarlos
ahi.

## Concurrencia

`ci.yml` cancela corridas en vuelo **solo en pull request**:

```yaml
cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

Y `deploy.yml` fija ademas la suya, `cancel-in-progress: false`. Es lo contrario de la politica de
CI, a proposito: cancelar un build desperdicia un runner, cancelar un despliegue deja el entorno a
medio migrar.

## Rollback

Volver a lanzar el workflow sobre un commit anterior: reconstruye ese codigo y lo aplica. **Las
migraciones no vuelven solas** — `goose down` no lo corre nadie automaticamente, y por eso el paso 2
exige compatibilidad hacia atras. Un esquema que solo anade es un esquema del que se puede volver.

## Lo que todavia no cubre

- **Sin `staging`.** Un solo entorno. Cuando haya otro es otra invocacion de `deploy.yml` con otro
  `environment` y otro `infra/envs/<nombre>/`, no otro workflow.
- **Sin rollback automatico.** El paso de salud falla el job, pero no revierte: avisa, no arregla.
- **Sin smoke test de arranque de la imagen.** Las etapas de Docker comprueban que la imagen
  *construye*, no que *arranca*. Menos grave desde que la imagen no es lo que se despliega.
- **`cmd/worker` y `cmd/scheduler` no se despliegan.** Sus cuerpos son `log.Debug(); return nil`. La
  forma que tomaran —EventBridge Scheduler contra una Lambda acotada— esta escrita en el ADR 0014.
- **La subida de parrillas no cabe por aqui.** `deploy/nginx.conf` admite 64 MB y una Function URL
  topa en 6 MB. Cuando llegue el endpoint de ingesta necesita un PUT prefirmado a S3.
