# Intela

Sistema de reconocimiento de obras y distribucion de ingresos por propiedad intelectual para
**REDES SGC**, la sociedad de gestion colectiva de los **escritores audiovisuales** de Colombia
(guionistas y libretistas).

## Lo que hay que entender antes de escribir codigo

Cuatro cosas que se contradicen con la intuicion y que causan errores caros:

1. **El dinero no llega por fila.** Ningun reporte de uso trae importes. REDES SGC cobra una
   bolsa a cada usuario segun el Reglamento de Tarifas, y los reportes de uso solo sirven para
   **ponderar** el reparto de esa bolsa. Es un asignador de bolsa, no un atribuidor de
   ingresos transaccionales.

2. **Los porcentajes de reparto solo salen de la Declaracion de Obra.** Nunca de los reportes
   de los canales ni de los contratos de escritura. Las columnas `Autor*` y `Guionista*` de
   una parrilla son pistas para identificar la obra, jamas insumo de pago.

3. **Si los porcentajes declarados no suman 100%, no se reparte nada de esa obra.** Se retiene
   el total en reserva. Nunca se reparte parcialmente. `declaracion_incompleta` es un estado
   valido del modelo, no un error.

4. **Solo se paga a escritores personas naturales.** Productores, directores, actores,
   ejecutivos de cadena y revisores no generan derecho de autor aqui, por mucho que aparezcan
   en los metadatos.

Toda cifra que el sistema produzca debe ser explicable hasta su origen: fuente, reporte, regla
aplicada. El reglamento lo exige y la auditoria lo revisa.

## La frontera

Puertos y adaptadores con la regla de dependencia de Clean Architecture: **nada del nucleo nombra
nada de afuera.** Es lo que mas se rompe por descuido.

| Directorio | Que vive aqui | No puede importar |
| ---------- | ------------- | ----------------- |
| `internal/dominio/` | Entidades, invariantes, calculo puro. Sin E/S. | `aplicacion`, `infraestructura`, `net/http`, `database/sql`, `pgx`, `chi`, `river`, `os`, `time`, `math/rand` |
| `internal/aplicacion/` | Casos de uso, orquestacion, transacciones. Los puertos se declaran aqui como `interface`. | `infraestructura`, `pgx`, `chi` |
| `internal/infraestructura/` | Adaptadores: HTTP, persistencia, clientes externos. | — |
| `cmd/{api,scheduler,worker}/` | Un `main` por punto de entrada. | — |

La lista completa, con la cita al ADR de cada `deny`, esta en [`.golangci.yml`](.golangci.yml).
No la dupliques: si cambia una regla, cambia ahi.

`time` esta denegado en el dominio a proposito: el instante entra por `PuertoReloj`. Los tipos
(`time.Time`) se reciben como parametro. Sin eso, las prescripciones de 3 y 10 anos no se pueden
probar y una corrida no se reproduce.

Antes de decidir en que capa va algo nuevo, lee
[`docs/architecture/README.md`](docs/architecture/README.md).

## Donde esta el contexto

Cargar solo lo que haga falta para la tarea.

| Archivo | Para que |
| ------- | -------- |
| `docs/architecture/README.md` | Capas, puertos, modulos, stack, diagramas. El detalle arquitectonico |
| `docs/dominio/glosario.md` | Lenguaje ubicuo. Que es obra, titular, ONI, recaudo, reparto, IPI, IDA |
| `docs/dominio/reglas-negocio.md` | Registro de reglas con cita al reglamento. Empezar aqui |
| `docs/dominio/formulas.md` | Modelos de calculo por tipo de usuario (TV, cine, OTT, hoteles) |
| `docs/dominio/identificadores.md` | Por que los IDs de fuente no cruzan y como resolver obras |
| `docs/dominio/fuentes-datos.md` | Perfil real de los archivos del cliente y que falta pedir |
| `docs/reglamentos/` | Texto verbatim de los reglamentos, citable por numeral |
| `docs/decisiones/` | Por que el sistema quedo modelado asi |
| `docs/ci.md` | Etapas de CI, la compuerta `ci`, filtrado por ruta y hooks locales |
| `docs/cd.md` | Despliegue: imagenes en GHCR, donde se inyecta el proveedor y los secretos |
| `docs/context.md` | Planteamiento academico original. **Anterior al analisis de dominio**: donde diga que los ingresos se acumulan por fila, manda `docs/dominio/formulas.md` |

Convencion de citas: `RD 13.1.3` es la seccion 13.1.3 del Reglamento de Distribucion IX.
`RT` Tarifas VI, `RS` Socios, `RA` Anticipos.

Las skills de `.claude/skills/` cargan el dominio bajo demanda. Si la tarea toca reparto, matching,
recaudo, ingesta, aprobaciones, afiliacion o trazabilidad, **usa la skill antes de escribir codigo**.

## Skills

Usa las skills instaladas de golang y clean architecture para seguir las mejores prácticas de código e ingeniería de software. Para revisar documentación, usa el MCP de Context7 antes de cada implementación. 

## Comandos

CI: la compuerta obligatoria es el job `ci` de `.github/workflows/ci.yml`; las etapas se filtran
por ruta y se saltan solas mientras su capa no exista. Las ramas siguen
`^(feature|fix|hotfix|docs|chore|refactor)/[a-z0-9._/-]+$` o el PR falla. Los hooks locales estan en
`lefthook.yml` (`lefthook install`); todo lo que corre en un hook corre tambien en CI. Detalle en
`docs/ci.md`.

CD: el despliegue esta al final del mismo `ci.yml` y solo corre en `push` a `main`, tras la
compuerta. Las imagenes se publican en GHCR; el job de despliegue es un andamio sin proveedor
todavia. Detalle en `docs/cd.md`.

La frontera de arquitectura se verifica sobre los `import` con `depguard` ([ADR 0012](docs/decisiones/0012-la-frontera-se-verifica-sobre-el-codigo.md)),
no sobre el diagrama. Todo el proyecto debe seguir Clean Architecture.

## Idioma

El dominio se nombra en **espanol**, igual que los reglamentos: `obra`, `titular`, `reparto`,
`recaudo`, `declaracion`. No traducir estos terminos en modelos, tablas ni variables. El resto del
codigo y la infraestructura van en ingles.

## Estilo

`ruff`, `gofmt` y `golangci-lint` ya corren en CI — no hace falta razonar sobre formato. Lo que
si hay que decidir a mano:

- Aplicar SOLID en logica de negocio, servicios y features grandes; no en un adaptador de 20 lineas.
- La solucion mas simple que cumpla el requisito (YAGNI, KISS). El sistema mueve dinero de
  terceros: la indireccion que no se justifica es deuda, no diseno. No apliques sobreingeniería (overengineering).
- Inyectar dependencias siempre que haya E/S, para que el nucleo se pruebe sin infraestructura.
- Todo el código y arquitectura debe ser modular con componentes débilmente acoplados para fácil extensión, mantenimiento y escalabilidad.

## Estado

Stack vigente: [`docs/decisiones/0010-stack-go.md`](docs/decisiones/0010-stack-go.md) (Go + React).
Ya existe `go.mod`, asi que `.golangci.yml` deja de ser inerte y la etapa `Architecture boundary`
si corre: la frontera esta vigilada sobre los `import` con `depguard` (ADR 0012).

El andamiaje vive en `cmd/`, `internal/` y `web/`. Es andamiaje: fronteras y puntos de entrada,
no el motor. El reparto, la identificacion y la persistencia entran en PRs propios.

Antes de construir el motor de matching o el de distribucion faltan datos del cliente: ver las
preguntas abiertas al final de [`docs/dominio/reglas-negocio.md`](docs/dominio/reglas-negocio.md)
y [`docs/dominio/fuentes-datos.md`](docs/dominio/fuentes-datos.md). Y no tratar las cifras del
seed como datos reales.

El equivalente para Cursor esta en `AGENTS.md`.
