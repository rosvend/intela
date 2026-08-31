# Arquitectura de Intela

Este directorio es el nivel de detalle. El [README raiz](../../README.md) responde *que forma tiene
esto*; aqui esta el *por que* y el *donde va cada cosa*.

Nada se copia: las decisiones viven en [`docs/decisiones/`](../decisiones/) y el dominio en
[`docs/dominio/`](../dominio/). Esta pagina enlaza, no duplica. Ante discrepancia entre
cualquier cosa escrita aqui y un reglamento, **manda el reglamento**.

---

## La regla que lo gobierna todo

**Puertos y adaptadores con la regla de dependencia de Clean Architecture: nada del nucleo nombra
nada de afuera** ([ADR 0002](../decisiones/0002-arquitectura-hexagonal.md)).

Lo importante no es que sea hexagonal. Es que los cuatro invariantes del negocio dejan de ser
disciplina y pasan a ser **estructura**:

| Invariante | Como lo hace imposible el tipo |
| ---------- | ------------------------------ |
| El dinero no llega por fila | `ReporteDeUso` no tiene campo de importe. Sumar importes por fila **no compila**. |
| Los porcentajes solo salen de la Declaracion de Obra | Las columnas de credito de una parrilla entran por el puerto de identificacion tipadas como *evidencia de matching*. El puerto de titularidad es otro y solo acepta declaraciones. No hay camino de tipos de una parrilla a un porcentaje de pago. |
| Si no suman 100%, no se reparte nada | `declaracion_incompleta` es un estado de la obra, no un flag de error. El motor solo puede retener el total o repartir el total. |
| Solo se paga a personas naturales | `Titular` exige IPI de persona natural. Ninguna firma puede emitir una orden de pago a un productor. |

---

## Capas

Un solo desplegable, con los modulos aislados por dentro
([ADR 0003](../decisiones/0003-monolito-modular.md)). El perfil de carga es un pico anual, no
alto trafico.

| Directorio | Que vive aqui | Que **no** puede importar |
| ---------- | ------------- | ------------------------- |
| `internal/dominio/` | Entidades, invariantes, calculo puro. Sin E/S. | `aplicacion`, `infraestructura`, `net/http`, `database/sql`, `pgx`, `chi`, `river`, `os`, `time`, `math/rand` |
| `internal/aplicacion/` | Casos de uso, orquestacion, limites de transaccion. **Los puertos se declaran aqui como `interface`.** | `infraestructura`, `pgx`, `chi` |
| `internal/infraestructura/` | Adaptadores: HTTP, persistencia, clientes externos. | — |
| `cmd/{api,scheduler,worker}/` | Un `main` por punto de entrada. | — |

Estas reglas no son documentacion: estan escritas como `depguard` en
[`.golangci.yml`](../../.golangci.yml) y cada `deny` cita su ADR. Con Go, ademas, los ciclos de
importacion **no compilan** e `internal/` es una frontera que entiende el compilador
([ADR 0010](../decisiones/0010-stack-go.md)).

### Por que `time` esta denegado en el dominio

No es purismo. Las prescripciones de 3 y 10 anos (`R-19`, `R-20`) y las ventanas de 15 dias
(`R-10`, `R-22`) tienen que poder probarse sin esperar una decada, y
[ADR 0005](../decisiones/0005-reparto-determinista-y-reproducible.md) exige que una corrida se
reproduzca bit a bit anos despues. Un `time.Now()` suelto en el nucleo no falla: devuelve otro
numero. Los **tipos** (`time.Time`, `time.Duration`) se reciben como parametro; el **instante**
entra por `PuertoReloj`.

---

## Puertos

**De entrada:** son los casos de uso mismos. Los adaptadores que los invocan —portales, API,
cargador de archivos, planificador, webhooks, CLI— no saben nada mas del nucleo.

**De salida:** interfaces que el nucleo declara y el adaptador satisface **sin importarlas**.
Repositorios, catalogos externos, dispersion de pagos, contabilidad, notificaciones, almacen de
documentos, bitacora, parametros normativos, reloj, motor de similitud.

Dos son de **dominio, no de infraestructura**, y conviene entender por que:

- **`PuertoReloj`** — por lo de arriba.
- **`PuertoNotificaciones`** — devuelve un acuse persistible. Notificar no es un efecto secundario:
  es el hecho juridico que **arranca el reloj de la prescripcion** (`RD 13.8.8`). Si el acuse no se
  guarda, no hay forma de defender una prescripcion ante la DNDA.

---

## Modulos del dominio

Nueve contextos delimitados. Las dependencias permitidas son exactamente las de la pagina 2 del
diagrama; **la ausencia de flecha es tan informativa como su presencia**.

| Regla | Consecuencia |
| ----- | ------------ |
| `Reparto` lee de `Repertorio` y de `Recaudo` | Ninguno de los dos conoce a `Reparto` |
| `Recaudo` es el unico que conoce `Usuario`, `Convenio`, `Tarifa` | Aguas abajo solo circula una bolsa |
| `Identificacion` escribe alias y emite ONI | **No toca dinero** |
| Ningun modulo escribe en la trazabilidad de otro | La bitacora es de quien la genera |

Sumideros del grafo: `Afiliacion`, `Ingesta`, `Recaudo`. Fuentes: `ONI`, `Reclamaciones`.

---

## El reparto es un flujo, no un job por lotes

`RD 13.5` define **dos** procesos con etapas y dueno por etapa, y el dinero no sale con una sola
firma ([ADR 0008](../decisiones/0008-reparto-como-flujo-con-aprobaciones.md)). Son dos
maquinas de estado distintas, no una con un `if`:

```text
Nacional        Recaudo → Deducciones → Importe de la Obra → Importe por Titular →
                Liq. Parcial → [Verificacion] → Liq. Final → [Pago y Registro] → Auditoria

Internacional   Recaudo → Deducciones → Liq. Parcial → [Verificacion] →
                Liq. Final → [Pago y Registro] → Fees in Error → Auditoria
```

`[...]` son compuertas humanas con doble firma. El internacional no tiene valorizacion por puntos
(`RD 7.4`), y `Fees in Error` no lleva deducciones (`R-16`).

---

## Stack

Decidido en [ADR 0010](../decisiones/0010-stack-go.md), que sustituye a
[0009](../decisiones/0009-stack-typescript-nestjs.md).

| Capa | Tecnologia | Por que |
| ---- | ---------- | ------- |
| Dominio | Go + `shopspring/decimal` | El compilador prohibe importar `internal/` desde fuera |
| Aplicacion | Casos de uso; puertos como `interface` | El adaptador satisface la interfaz **sin importarla** |
| Infraestructura | `chi` o `net/http` estandar | Sin framework que quiera ser dueno de los handlers |
| Puntos de entrada | Un `main` por `cmd/` | Es literalmente lo que pide `0003` |
| Persistencia | `pgx` v5 + `sqlc`, PostgreSQL 16 | SQL primero; `NUMERIC` escanea directo a `decimal.Decimal` |
| Cola | `River` sobre el mismo postgres | Encolado transaccional: una transaccion local por etapa |
| Migraciones | `goose` | — |
| Planificador | Temporizador sobre `CalendarioDeDistribucion` | `0004`: no es dueno de las fechas |
| Similitud | `pg_trgm` + `unaccent` tras `PuertoMotorDeSimilitud` | `0007`: sustituible sin tocar la cascada |
| Objetos | MinIO con object-lock local, S3 en produccion | `0005`: reportes crudos inmutables |
| Frontend | React + TypeScript + Vite | Tres portales con RBAC por roles del reglamento |
| Contratos | OpenAPI → `openapi-typescript` | El precio del cambio de stack: sin tipos compartidos |
| Exportables | `excelize`, `maroto` | OE-6 pide Excel y PDF |
| Pruebas | `testing`, `testcontainers-go`, `rapid` | El motor se prueba sin infraestructura |
| Frontera | El compilador, mas `depguard` | Los ciclos de importacion **no compilan** |
| Observabilidad | `log/slog` + OpenTelemetry | Separada de la bitacora, que es dominio |
| Despliegue | Binario estatico, imagen distroless, `docker compose` | Lo exige `docs/context.md` |

`src/scripts/` se queda en Python permanentemente (PEP 723), aunque el backend sea Go.

### Persistencia

El patron de los adaptadores —un fichero por puerto, la asercion de compilacion, el mapeo de
errores a los centinelas de `aplicacion`, y de quien son los limites de transaccion— esta escrito
al lado del codigo que describe:

```
go doc github.com/rosvend/intela/internal/infraestructura/postgres
```

Lo unico que no cabe en un doc de paquete, porque cruza dos capas: **`Obra.EstadoDecl` es un campo
derivado.** La tabla `obras` no tiene esa columna y no la va a tener. Sale de
`repertorio.Declaracion.Estado()`, porque `R-04` (`RD 13.1.3`) tiene tres clausulas —la suma da
100, cada porcentaje es positivo, ninguna parte va sin IPI— y un `SUM(porcentaje) = 100` en SQL
solo comprueba la primera.

Las pruebas de los adaptadores corren contra PostgreSQL real con `testcontainers-go`, asi que
`make verificar` y el hook de `pre-push` necesitan Docker. Para el bucle rapido sin contenedores:
`make prueba-rapida`.

---

## Decisiones

Una por archivo en [`docs/decisiones/`](../decisiones/). Nunca se borran: si una se revierte se
escribe otra que la reemplace. Sirve para que un cambio que parece una mejora obvia no rompa algo
que se decidio a proposito.

| ADR | Decide |
| --- | ------ |
| [0001](../decisiones/0001-base-de-conocimiento-en-el-repo.md) | La base de conocimiento vive en el repo, en markdown, en dos capas |
| [0002](../decisiones/0002-arquitectura-hexagonal.md) | Puertos y adaptadores; los invariantes se vuelven estructura |
| [0003](../decisiones/0003-monolito-modular.md) | Un desplegable con modulos aislados, no nueve microservicios |
| [0004](../decisiones/0004-parametros-normativos-como-dato.md) | Todo parametro normativo es una fila con vigencia y organo aprobador |
| [0005](../decisiones/0005-reparto-determinista-y-reproducible.md) | El motor es una funcion pura; cada corrida congela su snapshot |
| [0006](../decisiones/0006-trazabilidad-como-asiento-append-only.md) | La trazabilidad es un libro de asientos del dominio, no un log |
| [0007](../decisiones/0007-identificacion-en-cascada-con-cola-manual.md) | Cascada de cuatro escalones, con cola manual al final |
| [0008](../decisiones/0008-reparto-como-flujo-con-aprobaciones.md) | Dos maquinas de estado con doble firma, no una con un `if` |
| [0009](../decisiones/0009-stack-typescript-nestjs.md) | ~~TypeScript y NestJS~~ — **sustituida por 0010** |
| [0010](../decisiones/0010-stack-go.md) | Go en el backend; la regla de dependencia pasa a ser cosa del compilador |
| [0011](../decisiones/0011-verificacion-del-diagrama-como-aviso.md) | ~~La verificacion del diagrama avisa, no bloquea~~ — **sustituida por 0012** |
| [0012](../decisiones/0012-la-frontera-se-verifica-sobre-el-codigo.md) | La frontera se verifica sobre los `import` con `depguard`, no sobre el diagrama |

---

## Diagramas

### El diagrama completo

[`docs/diagrams/PATIC2 - Arquitectura.drawio`](../diagrams/PATIC2%20-%20Arquitectura.drawio),
tres paginas. Es la vista que manda.

| Pagina | Responde |
| ------ | -------- |
| 1 · Arquitectura Hexagonal | Que cruza la frontera y por que puerto |
| 2 · Modulos del Dominio | Que modulo puede importar a cual |
| 3 · Despliegue | Unica pagina con tecnologia concreta |

Convencion de lectura en la pagina 1: linea solida = *usa*; linea punteada con punta hueca =
*implementa*. Las del borde de salida van **adaptador → puerto**, porque el adaptador implementa el
contrato que el nucleo declara. **Ninguna arista sale del nucleo.**

### Los generados

| Archivo | Script |
| ------- | ------ |
| `docs/diagrams/arquitectura-{light,dark}.png` | `src/scripts/diagrama_arquitectura.py` |
| `docs/diagrams/despliegue.png` | `src/scripts/diagrama_despliegue.py` |

```bash
uv run --script src/scripts/diagrama_arquitectura.py
uv run --script src/scripts/diagrama_despliegue.py
```

Si un generado y el `.drawio` discrepan, **el `.drawio` es el correcto** y el script esta viejo.

### La verificacion

El diagrama **no se verifica**: documenta la intencion. Quien hace cumplir la frontera es el
codigo, en la etapa `Architecture boundary` de CI:

```bash
golangci-lint run --enable-only=depguard ./...
```

Corre aislada del lint de estilo a proposito, para que un check en rojo diga cual de las dos cosas
se rompio. Las reglas son la tabla de arriba, escritas como `deny` en
[`.golangci.yml`](../../.golangci.yml) con la cita al ADR de cada una.

> Hubo una comprobacion que recorria el `.drawio` y validaba las flechas. Se retiro en el
> [ADR 0012](../decisiones/0012-la-frontera-se-verifica-sobre-el-codigo.md): validaba un dibujo, no
> el sistema —el diagrama puede estar impecable mientras el codigo viola todas las reglas— y se
> rompia por ediciones cosmeticas de draw.io. El [ADR 0011](../decisiones/0011-verificacion-del-diagrama-como-aviso.md),
> que la habia degradado a aviso, queda sustituido.

**Consecuencia que hay que tener presente:** `depguard` solo corre cuando existe `go.mod`. Mientras
el modulo no aterrice en `main`, ninguna comprobacion de frontera bloquea un merge y la
mitigacion que `0002` y `0003` nombran depende de la revision humana. Es la ventana que `0012`
asume por escrito.

---

## Dominio

Cargar solo lo que haga falta para la tarea.

| Archivo | Para que |
| ------- | -------- |
| [`roles.md`](roles.md) | Matriz `aplicacion.Rol` → rol del reglamento y capacidades. **Leer antes de anadir una ruta** |
| [`glosario.md`](../dominio/glosario.md) | Lenguaje ubicuo: obra, titular, ONI, recaudo, reparto, IPI, IDA |
| [`reglas-negocio.md`](../dominio/reglas-negocio.md) | Registro de reglas con cita al reglamento. **Empezar aqui** |
| [`formulas.md`](../dominio/formulas.md) | Modelos de calculo por tipo de usuario (TV, cine, OTT, hoteles) |
| [`identificadores.md`](../dominio/identificadores.md) | Por que los IDs de fuente no cruzan y como resolver obras |
| [`fuentes-datos.md`](../dominio/fuentes-datos.md) | Perfil real de los archivos del cliente y que falta pedir |

Texto verbatim de los reglamentos, citable por numeral, en
[`docs/reglamentos/`](../reglamentos/). Convencion de citas: `RD 13.1.3` es la seccion 13.1.3
del Reglamento de Distribucion IX. `RT` Tarifas VI, `RS` Socios, `RA` Anticipos.
