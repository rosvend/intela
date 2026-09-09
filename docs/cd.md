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
- **El smoke test de arranque no toca lo desplegado.** Ya no falta —`Boot smoke test` levanta el
  stack de `docker-compose.yml` y lo recorre por HTTP en cada PR ([`docs/ci.md`](ci.md))— pero lo que
  arranca ahi son las **imagenes**, y desde el
  [ADR 0014](decisiones/0014-infraestructura-serverless-en-aws.md) la imagen no es lo que se
  despliega. Cubre el codigo y el cableado; no cubre la Lambda ni su entorno. Eso sigue siendo el
  paso de salud de este workflow.
- **`cmd/worker` y `cmd/scheduler` no se despliegan.** Sus cuerpos son `log.Debug(); return nil`. La
  forma que tomaran —EventBridge Scheduler contra una Lambda acotada— esta escrita en el ADR 0014.
- **La subida de parrillas no cabe por aqui.** `deploy/nginx.conf` admite 64 MB y una Function URL
  topa en 6 MB. Cuando llegue el endpoint de ingesta necesita un PUT prefirmado a S3.
