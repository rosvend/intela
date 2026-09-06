# Decisiones de arquitectura

Registro de por que el sistema quedo como quedo. Una decision por archivo,
`NNNN-titulo-corto.md`, numeracion incremental, nunca se borran: si una decision se revierte
se escribe una nueva que la reemplace y se marca la anterior como sustituida.

Estructura de cada archivo: contexto, decision, alternativas consideradas, consecuencias.

Sirve para que nadie, humano o agente, tenga que reconstruir el razonamiento leyendo el
codigo, y para que un cambio que parece una mejora obvia no rompa algo que se decidio a
proposito.

| Decision | Estado |
| -------- | ------ |
| [0001 Base de conocimiento en el repositorio](0001-base-de-conocimiento-en-el-repo.md) | Vigente |
| [0002 Arquitectura hexagonal con frontera Clean](0002-arquitectura-hexagonal.md) | Vigente |
| [0003 Monolito modular, no microservicios](0003-monolito-modular.md) | Vigente |
| [0004 Los parametros normativos son dato versionado](0004-parametros-normativos-como-dato.md) | Vigente |
| [0005 El calculo del reparto es una funcion pura y reproducible](0005-reparto-determinista-y-reproducible.md) | Vigente |
| [0006 La trazabilidad es un asiento append-only](0006-trazabilidad-como-asiento-append-only.md) | Vigente |
| [0007 Identificacion de obras en cascada, con cola manual](0007-identificacion-en-cascada-con-cola-manual.md) | Vigente |
| [0008 El reparto es un flujo con compuertas humanas](0008-reparto-como-flujo-con-aprobaciones.md) | Vigente |
| [0009 Stack de aplicacion: TypeScript y NestJS](0009-stack-typescript-nestjs.md) | Sustituida por 0010 |
| [0010 Stack de aplicacion: Go](0010-stack-go.md) | Vigente |
| [0011 La verificacion del diagrama avisa, no bloquea](0011-verificacion-del-diagrama-como-aviso.md) | Sustituida por 0012 |
| [0012 La frontera se verifica sobre el codigo, no sobre el diagrama](0012-la-frontera-se-verifica-sobre-el-codigo.md) | Vigente |
| [0013 La sesion es un token opaco en tabla, no un JWT](0013-sesiones-opacas-en-tabla.md) | Vigente |
| [0014 La infraestructura de ejecucion es AWS serverless, descrita en Terraform](0014-infraestructura-serverless-en-aws.md) | Vigente |
| [0016 El log de rechazos de la ingesta vive en una tabla aparte](0016-log-de-rechazos-en-tabla-aparte.md) | Vigente |

El `0015` falta a proposito: es `0015-cola-de-trabajos-en-tabla-propia.md`, que llega con la PR #85
y ocupa su fila cuando esa PR entre. Un hueco no rompe nada; un numero repetido si.

Y por eso, para elegir el numero de un ADR nuevo **no basta con mirar `main`**. Dos ADR con el
mismo numero y nombre de archivo distinto **no chocan en git**: el merge pasa limpio, nadie avisa,
y el unico sintoma es esta tabla. Es peor que un conflicto normal, que al menos bloquea. El numero
libre se busca sobre `main` y sobre las ramas abiertas:

```
git fetch origin --prune
for b in $(git branch -r --format='%(refname:short)' | grep -v HEAD); do
  git ls-tree "$b" --name-only -- docs/decisiones/
done | sed 's|.*/||' | sort -u
```

Es la misma politica que la de las migraciones de `goose` (cabecera de
`migrations/00002_catalogo_obras.sql`), con la diferencia de que alli la version repetida es un
error duro en cada arranque y aqui no salta nada.

El diagrama que materializa `0002`, `0003`, `0008` y `0010` es `docs/diagrams/PATIC2 - Arquitectura.drawio`.
Documenta la intencion; lo que `0002` y `0003` prometen se hace cumplir sobre el codigo con
`depguard` (reglas en [`.golangci.yml`](../../.golangci.yml), etapa `Architecture boundary` de CI).
Mientras no exista `go.mod` esa etapa se salta y no hay comprobacion de frontera que bloquee: es
la consecuencia que `0012` asume por escrito.
