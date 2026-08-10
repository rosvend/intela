# 0001 Base de conocimiento en el repositorio

Fecha: 2026-08-10
Estado: Vigente

## Contexto

El proyecto depende de mucho contexto de negocio: cuatro reglamentos de REDES SGC, un marco
legal colombiano y andino, y formulas de reparto que no se deducen del codigo. Un desarrollador
o un agente que empiece a programar sin ese contexto produce codigo plausible y equivocado, por
ejemplo sumando importes que no existen o repartiendo parcialmente una obra con declaracion
incompleta.

Habia que decidir donde vive ese contexto y en que forma.

## Decision

La base de conocimiento vive **en el repositorio, en markdown, versionada con git**, en dos
capas:

**Capa 1, `docs/reglamentos/`.** Texto verbatim de los reglamentos, partido por seccion,
generado automaticamente desde los PDF por `src/scripts/convert_reglamentos.py`. Es la fuente
citable. Nadie la lee entera: se llega por cita o por `grep` de un numeral.

**Capa 2, `docs/dominio/`.** Conocimiento destilado y escrito a mano: glosario, registro de
reglas, formulas, identificadores, fuentes de datos. Cada afirmacion cita su seccion de la capa
1. Es lo que se lee antes de programar.

Encima, `CLAUDE.md` en la raiz con lo minimo que hay que saber siempre y punteros al resto, y
`.claude/skills/` para que el detalle se cargue solo cuando la tarea lo toca.

Los PDF originales se conservan en `docs/reglamentos/fuente/`.

## Alternativas consideradas

**Un wiki externo (Confluence, Notion).** Descartado: el contenido queda detras de una API, no
en el repositorio, los agentes no lo leen sin integracion adicional, y se desincroniza del
codigo sin que nadie lo note en un diff.

**Una boveda de Obsidian o una app tipo Tolaria fuera del repo.** Producen exactamente el mismo
artefacto que esta decision, markdown en git, pero fuera del arbol del proyecto. Se pierde la
propiedad de que el contexto viaje con el codigo y aparezca en los diffs. Se puede adoptar
cualquiera de esas herramientas **apuntando a `docs/`**, que es compatible con esta decision.

**Volcar todo en `CLAUDE.md`.** Descartado: se carga completo en cada sesion, diluye el
contexto util y degrada la calidad de las respuestas.

**Busqueda semantica o RAG sobre los PDF.** Descartado para reglamentos. Se necesita el numeral
exacto, no un fragmento parecido. Una tabla de contenidos y `grep` son mas precisos y
verificables que un indice vectorial.

**Transcribir los reglamentos a mano.** Descartado: introduce errores en lo que debe ser fuente
de verdad legal, y no se puede repetir cuando llegue una version nueva.

## Consecuencias

Positivas: el contexto viaja con el codigo, se revisa en pull requests, es auditable como exige
`RD 16`, y cualquier cifra del sistema se puede rastrear hasta un numeral del reglamento.

A cambio: hay que mantener la capa 2 sincronizada cuando cambie un reglamento. La mitigacion es
que la capa 1 se regenera con un comando y que cada afirmacion de la capa 2 lleva cita, asi que
el trabajo de revision esta acotado.

Riesgo asumido: la capa 2 puede quedar desactualizada y contradecir la capa 1. Por eso queda
escrito en `docs/reglamentos/README.md` y en la skill `consultar-reglamentos` que ante
discrepancia manda el reglamento.

La conversion automatica tiene un limite conocido: `pdftotext` no extrae objetos de ecuacion de
Word. La ecuacion de `RD 9.7` esta transcrita a mano en `formulas.md` con aviso de procedencia,
y cada indice generado declara las limitaciones de su documento.
