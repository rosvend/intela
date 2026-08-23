# Intela

Sistema de reconocimiento de obras y distribucion de ingresos por propiedad intelectual para
**REDES SGC**, la sociedad de gestion colectiva de los escritores audiovisuales de Colombia
(guionistas y libretistas).
---

## Lo que hay que entender antes de escribir codigo

Cuatro cosas del dominio se contradicen con la intuicion, y las cuatro producen codigo plausible
y equivocado. Un error en cualquiera de ellas **no se manifiesta como una excepcion**: produce un
numero, el numero se paga, y aparece en una auditoria anos despues.

1. **El dinero no llega por fila.** Ningun reporte de uso trae importes. REDES SGC cobra una bolsa
   a cada usuario segun el Reglamento de Tarifas, y los reportes solo la **ponderan**. Es un
   asignador de bolsa, no un atribuidor de ingresos transaccionales.
2. **Los porcentajes salen solo de la Declaracion de Obra.** Nunca de los reportes de los canales
   ni de los contratos de escritura. Las columnas `Autor*` y `Guionista*` de una parrilla son
   pistas para **identificar** la obra, jamas insumo de pago.
3. **Si lo declarado no suma 100%, no se reparte nada de esa obra.** Se retiene el total en
   reserva. `declaracion_incompleta` es un estado valido del modelo, no un error.
4. **Solo se paga a escritores personas naturales.** Productores, directores, actores y revisores
   no generan derecho de autor aqui, por mucho que aparezcan en los metadatos.

Detalle y citas en [`CLAUDE.md`](CLAUDE.md) y [`docs/dominio/`](docs/dominio/).

---

## Arquitectura

**Puertos y adaptadores con la regla de dependencia de Clean Architecture: nada del nucleo nombra
nada de afuera.** Los cuatro invariantes de arriba dejan de ser disciplina y pasan a ser
estructura — el tipo `ReporteDeUso` no tiene campo de dinero, asi que la operacion de sumar
importes por fila sencillamente no existe.

Por dentro, **monolito modular**: los contextos delimitados tienen frontera real, pero no hay
limite de proceso entre ellos. El perfil de carga es un pico anual, no alto trafico.

Y el reparto es **un flujo con compuertas humanas**, no un job por lotes: `RD 13.5` define dos
procesos con etapas y dueno por etapa, y el dinero no sale con una sola firma.

### El diagrama

[`docs/PATIC2 - Arquitectura.drawio`](docs/PATIC2%20-%20Arquitectura.drawio), tres paginas:

| Pagina | Responde |
| ------ | -------- |
| 1 · Arquitectura Hexagonal | Que cruza la frontera y por que puerto. Adaptadores, casos de uso, dominio y los puertos de salida |
| 2 · Modulos del Dominio | Que modulo puede importar a cual. **La ausencia de flecha es tan informativa como su presencia** |
| 3 · Despliegue | Unica pagina con tecnologia concreta |

Convencion de lectura en la pagina 1: linea solida = *usa*; linea punteada con punta hueca =
*implementa*. Las del borde de salida van **adaptador → puerto**, porque el adaptador implementa
el contrato que el nucleo declara. Ninguna arista sale del nucleo.

Eso no es una promesa, se comprueba:

```
uv run --script src/scripts/check_arquitectura.py
```

Verifica la regla de dependencia de la pagina 1, que los modulos de la pagina 2 sean aciclicos, y
que ningun sistema externo de la pagina 3 quede dibujado sin que nadie lo consuma.

### Vista de despliegue generada

[`docs/despliegue.png`](docs/despliegue.png), regenerable con
`uv run --script src/scripts/diagrama_despliegue.py` (requiere Graphviz).

Reparto de responsabilidades, para que las dos vistas no se contradigan: **la pagina 3 del
`.drawio` manda** — lleva las citas al reglamento y las notas de decision. El PNG es la topologia
de infraestructura y sigue al `docker-compose`. Si discrepan, el `.drawio` es el correcto y el
script esta viejo.

---

## Decisiones

Una decision por archivo en [`docs/decisiones/`](docs/decisiones/). Nunca se borran: si una se
revierte se escribe otra que la reemplace. Sirve para que un cambio que parece una mejora obvia no
rompa algo que se decidio a proposito.

| ADR | Decide |
| --- | ------ |
| [0001](docs/decisiones/0001-base-de-conocimiento-en-el-repo.md) | La base de conocimiento vive en el repo, en markdown, en dos capas: reglamento verbatim y dominio destilado |
| [0002](docs/decisiones/0002-arquitectura-hexagonal.md) | Puertos y adaptadores; los cuatro invariantes se vuelven estructura, no disciplina |
| [0003](docs/decisiones/0003-monolito-modular.md) | Un desplegable con modulos aislados por dentro, no nueve microservicios |
| [0004](docs/decisiones/0004-parametros-normativos-como-dato.md) | Todo parametro normativo es una fila con vigencia y organo aprobador |
| [0005](docs/decisiones/0005-reparto-determinista-y-reproducible.md) | El motor es una funcion pura; cada corrida congela su snapshot |
| [0006](docs/decisiones/0006-trazabilidad-como-asiento-append-only.md) | La trazabilidad es un libro de asientos del dominio, no un log de aplicacion |
| [0007](docs/decisiones/0007-identificacion-en-cascada-con-cola-manual.md) | Cascada de cuatro escalones contra el catalogo maestro, con cola manual al final |
| [0008](docs/decisiones/0008-reparto-como-flujo-con-aprobaciones.md) | Dos maquinas de estado con compuertas de doble firma, no una con un `if` |
| [0009](docs/decisiones/0009-stack-typescript-nestjs.md) | TypeScript y NestJS, con el nucleo libre de framework |

---

## Stack

Decidido en [ADR 0009](docs/decisiones/0009-stack-typescript-nestjs.md). Un solo desplegable con
tres puntos de entrada (`api`, `scheduler`, `worker`) desde el mismo build.

| Capa | Tecnologia | Por que |
| ---- | ---------- | ------- |
| `domain/` | TypeScript + `decimal.js`, **sin framework** | La regla de dependencia de `0002` |
| `application/` | Casos de uso, puertos como `interface` | Declara contratos; no sabe quien los implementa |
| `infrastructure/` | NestJS 11, Drizzle ORM, PostgreSQL 16 | Los decoradores y el SQL viven aqui, no mas adentro |
| Cola | `pg-boss` sobre el mismo postgres | `0003` quiere una transaccion local por etapa |
| Planificador | `@nestjs/schedule` sobre `CalendarioDeDistribucion` | `0004`: no es dueno de las fechas |
| Similitud | `pg_trgm` + `unaccent` tras `PuertoMotorDeSimilitud` | `0007`: sustituible sin tocar la cascada |
| Objetos | MinIO local con object-lock, S3 en produccion | `0005`: reportes crudos inmutables |
| Frontend | React + TypeScript + Vite | Tres portales con RBAC por roles del reglamento |
| Exportables | `exceljs`, `@react-pdf/renderer` | OE-6 pide PDF y Excel |
| Pruebas | Vitest, `fast-check`, Testcontainers | El motor se prueba sin infraestructura |
| Arquitectura | **`dependency-cruiser`** en CI | La mitigacion que `0002` y `0003` exigian |
| Observabilidad | `pino` + OpenTelemetry | Separada de la bitacora, que es dominio |
| Despliegue | `docker compose` | Lo exige `docs/context.md` |

### Las tres consecuencias de elegir TypeScript

Estan razonadas en la ADR; en corto:

1. **No hay decimal nativo.** `decimal.js` y `NUMERIC(18,6)`, y el driver devuelve `numeric` como
   **cadena, nunca `number`**. Un `parseFloat` en el camino no rompe nada visible: solo hace que
   la cifra deje de ser reproducible.
2. **No hay `rapidfuzz`.** El matching arranca con `pg_trgm`. El escalon difuso esta detras de un
   puerto, asi que un servicio Python puede sustituirlo mas adelante.
3. **NestJS empuja hacia dentro.** Sus decoradores no pueden alcanzar `domain/` ni `application/`,
   y `dependency-cruiser` lo verifica en cada commit.

---

## Skills

`.claude/skills/` carga contexto de dominio solo cuando la tarea lo toca, para no diluir el
contexto util. Cada skill cita numeral de reglamento; ante discrepancia manda el reglamento.

| Skill | Cuando se dispara |
| ----- | ----------------- |
| [`consultar-reglamentos`](.claude/skills/consultar-reglamentos/) | Hace falta el texto exacto o la cita de un numeral |
| [`ingesta-y-normalizacion`](.claude/skills/ingesta-y-normalizacion/) | Cargar o perfilar reportes, esquema canonico, boveda cruda, duplicados |
| [`matching-de-obras`](.claude/skills/matching-de-obras/) | Cruzar reportes contra el catalogo maestro, alias, difuso, ONI |
| [`recaudo-y-tarifas`](.claude/skills/recaudo-y-tarifas/) | Tarifas por categoria, convenios, facturacion, la bolsa a repartir |
| [`reparto-y-distribucion`](.claude/skills/reparto-y-distribucion/) | Valorizacion, valor punto, deducciones, splits, liquidaciones |
| [`proceso-y-aprobaciones`](.claude/skills/proceso-y-aprobaciones/) | Etapas del `RD 13.5`, compuertas, doble firma, calendario |
| [`afiliacion-y-anticipos`](.claude/skills/afiliacion-y-anticipos/) | Socio contra Titular Administrado, padron IPI, anticipos |
| [`trazabilidad-y-auditoria`](.claude/skills/trazabilidad-y-auditoria/) | Bitacora de asientos, linaje de una cifra, `ExplicarCifra`, retencion |

`clean-architecture` es una skill **vendorizada** desde `wondelai/skills`, fijada en
[`skills-lock.json`](skills-lock.json) y enlazada desde `.agents/skills/`. Es teoria general; las
ocho de arriba son especificas de Intela y de REDES SGC.

---

## Estructura

```
data/files/           muestras del cliente (59 y 49 filas)
data/                 padron IPI, sin perfilar todavia
docs/dominio/         conocimiento destilado, con citas
docs/reglamentos/     texto verbatim + PDF originales en fuente/
docs/decisiones/      ADR: por que el sistema quedo asi
src/scripts/          scripts sueltos, cada uno con su entorno PEP 723
.claude/skills/       contexto de dominio cargado bajo demanda
```

Convencion de citas: `RD 13.1.3` es la seccion 13.1.3 del Reglamento de Distribucion IX.
`RT` Tarifas VI, `RS` Socios, `RA` Anticipos.

El dominio se nombra en **espanol**, igual que los reglamentos: `obra`, `titular`, `reparto`,
`recaudo`, `declaracion`. No traducir estos terminos en modelos, tablas ni variables. El codigo de
infraestructura puede ir en ingles.

---

## Scripts

Declaran sus dependencias en linea (PEP 723) y resuelven su propio entorno efimero, para no
interferir con el stack de la aplicacion.

```
uv run --script src/scripts/sample.py                 # perfila los archivos de muestra
uv run --script src/scripts/check_arquitectura.py     # verifica el diagrama contra los ADR
uv run --script src/scripts/diagrama_despliegue.py    # regenera docs/despliegue.png
uv run --script src/scripts/convert_reglamentos.py    # regenera docs/reglamentos/ desde los PDF
```

`convert_reglamentos.py` requiere `pdftotext` (poppler-utils) y `diagrama_despliegue.py` requiere
Graphviz.

---

## Bloqueos

Antes de construir el motor de matching o el de distribucion faltan datos del cliente. En orden de
impacto, con el detalle en [`docs/dominio/fuentes-datos.md`](docs/dominio/fuentes-datos.md):

1. **Export de Declaraciones de Obra** desde REDES-SYS. Sin autores y porcentajes no hay reparto
   posible. Es el dato mas critico y no esta en ninguna muestra.
2. **Reportes de recaudo** por usuario y periodo. La bolsa. Ningun archivo actual trae importes.
3. **Feed de rating** por franja horaria. Bloquea la formula de television completa.
4. **Coeficientes `Wa`, `Wb`, `Wc`** de `RD 9.7`. Bloquean el calculo OTT; no estan publicados.

Y dos ambiguedades del propio reglamento que hay que resolver con el cliente, no interpretar:
la **base de calculo de salas de cine** (`T-02`, dos partes del `RT` se contradicen) y el
**hueco de la tabla hotelera** en el tramo de 71 a 100 habitaciones (`T-06`).

Lista completa al final de
[`docs/dominio/reglas-negocio.md`](docs/dominio/reglas-negocio.md).
