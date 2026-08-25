# Intela

Sistema de reconocimiento de obras y distribucion de ingresos por propiedad intelectual para
**REDES SGC**, la sociedad de gestion colectiva de los **escritores audiovisuales** de
Colombia (guionistas y libretistas).

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

## Donde esta el contexto

Cargar solo lo que haga falta para la tarea. Skills en `.claude/skills/`. Cursor las carga desde ahi via `.cursor/rules/*.mdc`; no hay copia aparte.

| Archivo | Para que |
| ------- | -------- |
| `docs/dominio/glosario.md` | Lenguaje ubicuo. Que es obra, titular, ONI, recaudo, reparto, IPI, IDA |
| `docs/dominio/reglas-negocio.md` | Registro de reglas con cita al reglamento. Empezar aqui |
| `docs/dominio/formulas.md` | Modelos de calculo por tipo de usuario (TV, cine, OTT, hoteles) |
| `docs/dominio/identificadores.md` | Por que los IDs de fuente no cruzan y como resolver obras |
| `docs/dominio/fuentes-datos.md` | Perfil real de los archivos del cliente y que falta pedir |
| `docs/reglamentos/` | Texto verbatim de los reglamentos, citable por numeral |
| `docs/decisiones/` | Por que el sistema quedo modelado asi |
| `docs/context.md` | Planteamiento academico original del proyecto |

Convencion de citas: `RD 13.1.3` es la seccion 13.1.3 del Reglamento de Distribucion IX.
`RT` Tarifas VI, `RS` Socios, `RA` Anticipos.

## Skills

Usa las skills instaladas de golang y clean architecture para seguir las mejores prácticas de código e ingeniería de software. Para revisar documentación, usa el MCP de Context7 antes de cada implementación. 

## Idioma

El dominio se nombra en **espanol**, igual que los reglamentos: `obra`, `titular`, `reparto`,
`recaudo`, `declaracion`. No traducir estos terminos en modelos, tablas ni variables. El
codigo de infraestructura puede ir en ingles.

## Stack (ADR 0010)

Go en el backend; React + TypeScript + Vite en el frontend. Un binario por `cmd/{api,scheduler,worker}`.

- Nucleo: `internal/dominio/` (puro) y `internal/aplicacion/` (casos de uso y puertos).
- Adaptadores: `internal/infraestructura/`.
- Persistencia: PostgreSQL 16, `pgx` + `sqlc`, migraciones `goose`.
- Cola: River sobre el mismo Postgres.
- Objetos: MinIO local / S3, reportes crudos inmutables.
- Contratos: `api/openapi.yaml` → tipos TS en el frontend.
- Frontera: el compilador + `depguard` en `.golangci.yml`.

Los scripts de `src/scripts/` siguen en Python (PEP 723). Todo el proyecto debe seguir Clean Architecture.

## Estructura

```
cmd/                 puntos de entrada
internal/dominio/    entidades e invariantes, sin E/S
internal/aplicacion/ casos de uso y puertos
internal/infraestructura/
web/                 portales React
api/openapi.yaml     contrato HTTP
docs/dominio/        conocimiento destilado, con citas
docs/reglamentos/    texto verbatim + PDF originales en fuente/
src/scripts/         scripts sueltos, cada uno con su entorno PEP 723
.claude/skills/      contexto de dominio cargado bajo demanda
.cursor/rules/       punteros para Cursor a las skills de arriba
```

## Idioma

El dominio se nombra en **español**, igual que los reglamentos: `obra`, `titular`, `reparto`,
`recaudo`, `declaracion`. No traducir estos terminos en modelos, tablas ni variables. El resto del
código, infraestructura e implementanciones van en inglés. 

## Estilo

`ruff`, `gofmt` y `golangci-lint` ya corren en CI — no hace falta razonar sobre formato. Lo que
si hay que decidir a mano:

- Aplicar SOLID en logica de negocio, servicios y features grandes; no en un adaptador de 20 lineas.
- La solucion mas simple que cumpla el requisito (YAGNI, KISS). El sistema mueve dinero de
  terceros: la indireccion que no se justifica es deuda, no diseno.
- Inyectar dependencias siempre que haya E/S, para que el nucleo se pruebe sin infraestructura.
- Todo el código y arquitectura debe ser modular con componentes débilmente acoplados para fácil mantenimiento, escalabilidad y extensión.


## Arranque local

```
docker compose up --build
```

Dashboard en `http://localhost/` (nginx). API en `http://localhost/api`.

El arranque no siembra usuarios ni aplica migraciones: las migraciones son de
`goose` y el seed es un comando aparte, los dos pendientes del PR de
persistencia. Detalle y variables de entorno en [`docs/ARRANQUE.md`](docs/ARRANQUE.md).
