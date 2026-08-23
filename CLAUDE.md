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
| `docs/context.md` | Planteamiento academico original. **Anterior al analisis de dominio**: donde diga que los ingresos se acumulan por fila, manda `docs/dominio/formulas.md` |

Convencion de citas: `RD 13.1.3` es la seccion 13.1.3 del Reglamento de Distribucion IX.
`RT` Tarifas VI, `RS` Socios, `RA` Anticipos.

Las skills de `.claude/skills/` cargan el dominio bajo demanda. Si la tarea toca reparto, matching,
recaudo, ingesta, aprobaciones, afiliacion o trazabilidad, **usa la skill antes de escribir codigo**.

## Comandos

```bash
uv run --script src/scripts/<archivo>.py     # todos los scripts, entorno efimero (PEP 723)
uvx "ruff@0.16.4" check src/                 # lint Python, igual que CI
uvx "ruff@0.16.4" format src/                # formato Python
```

| Script | Que hace |
| ------ | -------- |
| `sample.py` | Diagnostico de los archivos de muestra del cliente |
| `check_arquitectura.py` | Verifica la frontera sobre el `.drawio`. Avisa, no bloquea (ADR 0011) |
| `diagrama_arquitectura.py` | Regenera el diagrama del README. Necesita Graphviz |
| `diagrama_despliegue.py` | Regenera la topologia de despliegue. Necesita Graphviz |
| `convert_reglamentos.py` | Regenera `docs/reglamentos/` desde los PDF. Necesita `poppler-utils` |

CI: la compuerta obligatoria es el job `ci` de `.github/workflows/ci.yml`; las etapas se filtran
por ruta. Las ramas siguen `^(feature|fix|hotfix|docs|chore|refactor)/[a-z0-9._/-]+$` o el PR falla.

## Idioma

El dominio se nombra en **espanol**, igual que los reglamentos: `obra`, `titular`, `reparto`,
`recaudo`, `declaracion`. No traducir estos terminos en modelos, tablas ni variables. El resto del
codigo y la infraestructura van en ingles.

## Estilo

`ruff`, `gofmt` y `golangci-lint` ya corren en CI — no hace falta razonar sobre formato. Lo que
si hay que decidir a mano:

- Aplicar SOLID en logica de negocio, servicios y features grandes; no en un adaptador de 20 lineas.
- La solucion mas simple que cumpla el requisito (YAGNI, KISS). El sistema mueve dinero de
  terceros: la indireccion que no se justifica es deuda, no diseno.
- Inyectar dependencias siempre que haya E/S, para que el nucleo se pruebe sin infraestructura.

## Estado

Fase de analisis, cerrandose. El stack esta decidido en
[`docs/decisiones/0010-stack-go.md`](docs/decisiones/0010-stack-go.md) (Go + React) pero **todavia
no existe `go.mod`**: no hay codigo de aplicacion. Mientras tanto `.golangci.yml` esta escrito pero
inerte, y el job `codigo` de `arquitectura.yml` no corre.

Antes de construir el motor de matching o el de distribucion faltan datos del cliente: ver las
preguntas abiertas al final de `docs/dominio/reglas-negocio.md`.
