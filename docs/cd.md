# Despliegue continuo

El despliegue vive en el mismo `ci.yml` que la integracion, al final, y **solo corre en `push` a
`main`**. Hoy es un andamio: publica imagenes de verdad y no despliega nada, porque no hay proveedor
decidido —no existe ADR de infraestructura de ejecucion—.

Un andamio que no miente: cada paso imprime el comando que acabara ejecutando y el secreto que
necesitara. Un check verde aqui significa *se llego al camino de release*, nunca *se desplego*.

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
| `Deploy (production)` | Solo `push` a `main`, tras `ci` | Andamio. Imprime el plan, no despliega |

**Verificar y publicar son la misma etapa** (`container.yml`), con `push: false` en PR y `push: true`
en `main`. Separarlas en dos workflows las dejaria divergir, y construiria cada imagen dos veces en
`main`. La primera noticia de la divergencia seria una imagen que paso CI y falla al construir en el
camino de release.

## Las imagenes

Se publican en **GHCR**, que no necesita ningun secreto propio: `container.yml` se autentica con el
`GITHUB_TOKEN` del propio job, con permiso `packages: write`.

```
ghcr.io/rosvend/intela-api:sha-<sha completo>    inmutable, es lo que despliega el release
ghcr.io/rosvend/intela-api:main                  puntero movil, comodidad
ghcr.io/rosvend/intela-web:sha-<sha completo>
ghcr.io/rosvend/intela-web:main
```

Las etiquetas se calculan dentro de `container.yml` en vez de con `docker/metadata-action`, para que
la referencia que el workflow devuelve como `output` sea exactamente la que empujo. `deploy.yml`
despliega **siempre la etiqueta `sha-`**: es inmutable, y un rollback es volver a lanzar el job con
otro SHA.

> Mientras no exista ningun `Dockerfile` en el repositorio, las dos etapas de imagen se saltan y
> `Deploy (production)` corre igualmente, con las entradas vacias, y reporta que no habia nada que
> desplegar. Es la respuesta honesta y mantiene el camino de release ejercitado en vez de teorico.

## Donde se inyecta el proveedor

Cada `TODO(provider)` de [`.github/workflows/deploy.yml`](../.github/workflows/deploy.yml) es un
punto de inyeccion. Rellenarlo es sustituir el `echo` por el comando real y anadir el secreto; la
estructura de alrededor —orden, compuerta de entorno, concurrencia— no cambia.

El orden no es decorativo:

1. **Autenticar** contra el proveedor.
2. **Migrar** la base de datos, *antes* de que la imagen nueva sirva trafico, y de forma compatible
   hacia atras. El [ADR 0008](decisiones/0008-reparto-como-flujo-con-aprobaciones.md) hace del
   reparto un flujo de varias etapas con aprobaciones: una corrida en vuelo durante un despliegue no
   puede encontrarse un esquema que su codigo no conoce.
3. **Desplegar** la imagen ya publicada, por su etiqueta `sha-`. Este paso selecciona, nunca
   reconstruye.
4. **Verificar** salud, y fallar el job si no se recupera. Hasta que este paso exista de verdad, el
   rollback es manual.

## Donde van los secretos

En secretos de **entorno** (`production`), no de repositorio: quedan acotados al entorno y una
corrida que apunte a otro sitio no puede leerlos. Se declaran en el bloque `secrets:` de
`workflow_call` de `deploy.yml` —hoy comentado, porque declarar un secreto antes de que algo lo
consuma invita a pasar una credencial que el workflow no sabe usar— y el llamador los pasa.

Dos formas, por orden de preferencia:

- **OIDC**, si el proveedor lo soporta: no se guarda ninguna credencial. Necesita `id-token: write`
  en el job que llama. **Hoy no se concede a proposito**: un permiso de token que nadie usa es un
  pasivo permanente, asi que lo anade el commit que lo necesite.
- **Credencial estatica** en un secreto de entorno, para proveedores sin OIDC.

## La compuerta de aprobacion

`deploy.yml` corre con `environment: production`. Anadir *required reviewers* a ese entorno en los
ajustes del repositorio convierte el job en una aprobacion manual **sin tocar el workflow**.

El entorno todavia no existe; GitHub lo crea la primera vez que el job corre.

## Concurrencia

`ci.yml` cancela corridas en vuelo **solo en pull request**:

```yaml
cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

Y `deploy.yml` fija ademas la suya, `cancel-in-progress: false`. Es lo contrario de la politica de
CI, a proposito: cancelar un build desperdicia un runner, cancelar un despliegue deja el entorno a
medio migrar.

## Lo que todavia no cubre

- **No despliega.** Falta el proveedor. Todo lo de arriba es la forma, no el fondo.
- **Sin verificacion de salud real**, asi que el rollback es manual.
- **Sin smoke test de arranque.** Las etapas de Docker comprueban que la imagen *construye*, no que
  *arranca*. `load: true` en PR deja la imagen en el daemon local justo para poder encadenarlo aqui.
- **Un solo entorno.** No hay `staging`. Cuando lo haya, es otra invocacion de `deploy.yml` con otro
  `environment`, no otro workflow.
