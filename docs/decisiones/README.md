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
| [0011 La verificacion del diagrama avisa, no bloquea](0011-verificacion-del-diagrama-como-aviso.md) | Vigente |

El diagrama que materializa `0002`, `0003`, `0008` y `0010` es `docs/diagrams/PATIC2 - Arquitectura.drawio`.
Que el diagrama cumpla lo que `0002` y `0003` prometen se comprueba con
`uv run --script src/scripts/check_arquitectura.py`. Desde `0011` esa comprobacion **avisa pero no
bloquea** el merge, y se retira cuando `depguard` corra sobre codigo Go.
