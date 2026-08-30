# Integracion continua

Un solo workflow con disparadores, [`.github/workflows/ci.yml`](../.github/workflows/ci.yml).
Todo lo demas son etapas reutilizables (`workflow_call`) que ese orquestador invoca. El pipeline
esta en ingles a proposito: es infraestructura, y el idioma del dominio (`obra`, `titular`,
`reparto`) se reserva para lo que modela el negocio. Por la misma razon `.golangci.yml` esta en
ingles: sus mensajes de `depguard` acaban en el log de CI.

El **despliegue** vive en el mismo `ci.yml`, al final, y solo corre en `push` a `main`. Se documenta
aparte, en [`docs/cd.md`](cd.md).

## La compuerta

El unico check obligatorio en la proteccion de `main` es el job agregador **`ci`**, al final de
`ci.yml`. Agrega el resultado de todos los demas:

```yaml
if: always()
# falla si algun `needs` acabo en 'failure' o 'cancelled'
```

`skipped` **no** bloquea; solo `failure` y `cancelled`. De ahi salen dos propiedades que importan:

- Anadir una etapa es anadir un job y meterlo en el `needs` de `ci`. La proteccion de rama no se
  toca nunca.
- Una etapa que no aplica se salta y la compuerta sigue en verde.

> El nombre del job `ci` es el contrato con la proteccion de rama. Renombrarlo deja `main`
> esperando un check que ya no existe, y no se puede mergear nada hasta arreglarlo.

## Etapas

| Etapa | Que corre | Cuando |
| ----- | --------- | ------ |
| `Branch naming convention` | El prefijo de la rama | En cada PR |
| `Lint (Python)` | `ruff check` y `ruff format --check` sobre `src/` | Hay `*.py` y el PR toca Python o `ruff.toml` |
| `Lint (workflows)` | `actionlint` con shellcheck sobre cada `run:` | El PR toca `.github/` |
| `Lint (Go)` | `go mod tidy` sin diff, `gofmt -l`, `go vet`, `go build`, `golangci-lint` | Hay `go.mod` y el PR toca Go |
| `Test (Go)` | `go test -race -count=1` con perfil de cobertura | Hay `go.mod` y el PR toca Go |
| `Architecture boundary` | `depguard` aislado, sobre los `import` reales | Hay `go.mod` y el PR toca Go |
| `Lint (frontend)` | `eslint`, `prettier --check`, `tsc --noEmit` | Hay `web/package.json` y el PR toca `web/` |
| `Test (frontend)` | `npm test` | Hay `web/package.json` y el PR toca `web/` |
| `Frontend build` | `npm ci` y `npm run build` (`tsc -b` + `vite build`) | Hay `web/package.json` y el PR toca `web/` |
| `Docker build (backend)` | Construye `Dockerfile`. Publica solo en `main` | Hay `Dockerfile` y el PR toca el contenedor |
| `Docker build (frontend)` | Construye `web/Dockerfile`. Publica solo en `main` | Hay `web/Dockerfile` y el PR toca `web/` |
| `Deploy (production)` | Andamio de release. No despliega | Solo en `push` a `main`, tras la compuerta |

**Hoy las tres capas ya estan en `main`** (`go.mod`, `Dockerfile` y `web/`), asi que las etapas de
Go y de frontend corren cuando el PR toca su capa, y los dos `Docker build` cuando toca los
contenedores. Cada etapa se enciende sola por la existencia de su capa: el commit que la introdujo
no toco el pipeline.

### Las etapas de frontend tienen fallback tolerante

`Lint (frontend)` y `Test (frontend)` estan escritas contra el `web/` que ya vive en `main`, que
declara `lint`, `typecheck`, `test` y `format:check`. Cada comprobacion busca el script que
necesita en `web/package.json` y, si esta, lo corre.

El fallback tolerante quedo como defensa: si un script desaparece, la etapa emite un `::notice::`
nombrando la herramienta que falta y **pasa** en vez de romper en rojo. Fue deliberado cuando
`web/` no existia — fallar habria obligado a que el commit que trae el frontend traiga ademas
configuracion de eslint, de prettier y un runner de tests, y asi es como alguien acaba borrando la
etapa en vez de completarla. Hoy solo deberia dispararse por una regresion, y el `::notice::` sale
en el resumen del job en cada corrida.

`tsc --noEmit` corre aqui aunque `npm run build` ya ejecute `tsc -b`. No es redundante: un error de
tipos lo tiene que reportar el check de lint, en segundos, no el final de un build de Vite.

### Por que hay dos etapas de lint de Go

`Lint (Go)` y `Architecture boundary` corren el mismo binario sobre el mismo codigo, pero
separados: la segunda usa `--enable-only=depguard`. Es deliberado. Cuando un check se
pone en rojo, su nombre tiene que decir si se rompio la **frontera** (`0002`, `0003`) o si sobra un
espacio. Mezclarlos convierte una violacion de arquitectura en un item mas de una lista de estilo.

## El filtrado por ruta va en el job, nunca en el disparador

Esta es la decision con mas consecuencias del pipeline. Las etapas se filtran con `if:` a nivel de
**job**; nunca con `on.pull_request.paths`.

Un workflow saltado por un filtro de disparador **no reporta ningun estado**. GitHub no lo marca
como saltado: lo deja en `Expected — waiting for status to be reported`. Si el check obligatorio
depende de eso, el PR no se puede mergear nunca y `main` queda bloqueada sin un error que lo
explique.

Con el filtro en el job, el workflow siempre arranca y siempre reporta; lo que se salta son los
jobs de dentro, y `skipped` no bloquea la compuerta.

El job `detect` calcula que aplica. Una etapa corre cuando se cumplen **las dos** condiciones:

- **existe** — la capa esta en el repositorio (`go.mod`, `Dockerfile`, `web/package.json`).
- **tocada** — el PR cambia algo que le pertenece.

En `push` a `main` y en `workflow_dispatch` no hay filtro por ruta: corre todo lo que existe. Son
eventos raros —merges y reejecuciones a mano— y ahi la red completa vale mas que el ahorro.

Cada patron de rutas se incluye a si mismo: `P_PYTHON` cubre `ruff.toml`, `P_GO` cubre
`.golangci.yml`, y todos cubren `ci.yml`. Aflojar una regla tiene que volver a disparar la etapa
que esa regla gobierna, en lugar de colarse sin verificar.

La lista de ficheros del PR se pide a la API (`gh api .../pulls/N/files`), no a `git diff`: asi no
depende de la profundidad del clon ni de que se haya traido la rama base.

## Nombres de rama

```text
^(feature|fix|hotfix|docs|chore|refactor)/[a-z0-9._/-]+$
```

`feat` **no** vale; el prefijo es `feature`. La rama llega por `github.head_ref`, que controla quien
abre el PR, asi que se pasa por `env:` y nunca se interpola dentro del `run:` — si no, un nombre de
rama es inyeccion de shell.

## Hooks locales

[`lefthook.yml`](../lefthook.yml) corre las comprobaciones rapidas antes del commit y las lentas
antes del push:

| Momento | Que corre |
| ------- | --------- |
| `pre-commit` | `gofmt` (reescribe y re-stagea), `ruff check`, `ruff format`, `actionlint` |
| `pre-push` | nombre de rama, `go test`, `golangci-lint`, build del frontend |

```bash
go install github.com/evilmartians/lefthook@latest   # o: brew install lefthook
lefthook install
```

**Todo lo que corre en un hook corre tambien en CI, nunca solo en el hook.** Un hook se salta con
`--no-verify` y solo existe en la maquina de quien ejecuto `lefthook install`; tratarlo como
comprobacion de verdad es dejar la frontera en manos de la buena voluntad. Los hooks estan para no
pelearse con CI por un espacio, no para sustituirlo.

Cada comando se salta solo si su herramienta no esta instalada, para que un toolchain incompleto
no bloquee un commit.

## Lo que tiene que traer cada capa

Las etapas estan escritas en estricto a proposito. Aflojarlas las pondria en verde escondiendo que
el build no es reproducible, que es justo lo contrario de lo que pide el
[ADR 0005](decisiones/0005-reparto-determinista-y-reproducible.md).

- **El PR que traiga Go** tiene que commitear **`go.sum`**. `Lint (Go)` exige que `go mod tidy` no
  produzca diff. Ojo tambien con `go mod download || true` en un `Dockerfile`: ese `|| true` se
  traga el fallo y construye la imagen con dependencias sin verificar.
- **El frontend ya vive en `main` con su `web/package-lock.json`**: `Frontend build` usa `npm ci`,
  que lo exige — un `npm install` en el `web/Dockerfile` re-resolveria el arbol en cada build y dos
  imagenes del mismo commit podrian llevar codigo distinto. La capa trajo eslint y tsc, y vitest y
  prettier llegaron despues: hoy `Lint (frontend)` y `Test (frontend)` comprueban de verdad.

## Lo que todavia no cubre

- **El despliegue es un andamio.** Las imagenes se publican de verdad en GHCR al mergear, pero
  `Deploy (production)` no despliega: falta decidir proveedor. Detalle en [`docs/cd.md`](cd.md).
- **Sin arranque de imagen.** Las etapas de Docker comprueban que la imagen *construye*, no que
  *arranca*.
- **Sin golden files del reparto.** Los tests unitarios son el suelo. El `ADR 0005` pide que una
  corrida sea reproducible bit a bit anos despues, y eso necesita casos construidos desde los
  ejemplos resueltos de los propios reglamentos.
