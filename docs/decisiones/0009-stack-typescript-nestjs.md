# 0009 Stack de aplicacion: TypeScript y NestJS

Fecha: 2026-08-22
Estado: Vigente

## Contexto

`0002-arquitectura-hexagonal.md` aplazo la eleccion de tecnologia a proposito: mientras faltaran
respuestas del cliente —si Intela reemplaza a REDES-SYS y AVSYS o convive con ellos, que
coeficientes tiene la formula OTT, de donde sale el rating— cualquier decision que atara el
dominio a un stack se iba a pagar cuando esas respuestas llegaran.

Esa razon sigue en pie, y esta ADR no la contradice: el nucleo sigue sin nombrar tecnologia. Lo
que se decide aqui es la tecnologia de los **adaptadores** y el lenguaje en que se escribe todo.
Hacia falta hacerlo ahora por tres motivos concretos:

1. Los KR-1 a KR-3 tienen horizonte de sprint 2 a 5 y no arrancan sin stack.
2. Los ADR `0002` y `0003` nombran su mitigacion —"un test de arquitectura en CI que falle si el
   paquete del dominio importa cualquier cosa de infraestructura"— **sin nombrar herramienta**.
   Mientras no exista, las dos decisiones se degradan solas.
3. `0005` exige aritmetica decimal exacta y `0006` exige almacenamiento append-only forzado por el
   motor. Las dos son propiedades del stack, no del diseno.

## Decision

**TypeScript sobre Node, con NestJS confinado a la capa de adaptadores.** Un solo desplegable con
tres puntos de entrada desde el mismo build (`api`, `scheduler`, `worker`), como fija `0003`.

| Capa | Tecnologia | Restriccion que impone la arquitectura |
| ---- | ---------- | -------------------------------------- |
| `domain/` | TypeScript plano + `decimal.js` | **Cero imports de framework.** Ni `@nestjs/*`, ni el ORM, ni el cliente HTTP |
| `application/` | Casos de uso, puertos como `interface`, tokens de inyeccion | Declara los contratos; no sabe quien los implementa |
| `infrastructure/` | NestJS 11, Drizzle ORM, PostgreSQL 16 | Aqui viven los decoradores y el SQL |
| Cola | `pg-boss` sobre el mismo PostgreSQL | `0003` quiere **una transaccion local por etapa** de reparto |
| Planificador | `@nestjs/schedule` leyendo `CalendarioDeDistribucion` | `0004`: el planificador no es dueno de las fechas |
| Similitud | `pg_trgm` + `unaccent` detras de `PuertoMotorDeSimilitud` | `0007`: el escalon 3 se sustituye sin tocar la cascada |
| Objetos | MinIO en local con versionado y object-lock, S3 en produccion | `0005`: reportes crudos inmutables |
| Frontend | React + TypeScript + Vite | Tres portales con RBAC por roles del reglamento |
| Pruebas | Vitest, `fast-check`, Testcontainers | `0005`: el motor se prueba sin infraestructura |
| **Test de arquitectura** | **`dependency-cruiser`** en CI | La mitigacion que `0002` y `0003` pedian |
| Observabilidad | `pino` + OpenTelemetry | `0006`: separada de la bitacora, que es dominio |

Los scripts de `src/scripts/` **siguen en Python** con PEP 723. Son analisis y generacion de
documentacion, no stack de aplicacion, y `CLAUDE.md` ya lo contempla.

### Las tres consecuencias que hay que asumir explicitamente

**TypeScript no tiene decimal nativo.** `0005` exige aritmetica decimal exacta con regla de
redondeo declarada, porque el valor punto es una division (`RD 9.1.1`) y el residuo tiene que ser
reproducible. Se resuelve con `decimal.js` en el dominio y `NUMERIC(18,6)` en PostgreSQL, y con una
condicion que no es negociable: **el driver devuelve `numeric` como cadena, nunca como `number`**.
Un `parseFloat` en el camino convierte una cifra auditable en un artefacto de coma flotante, y el
sistema no se rompe: sigue produciendo numeros, solo que equivocados en el ultimo centavo. Es el
error mas silencioso de esta eleccion y por eso queda escrito aqui.

**No hay `rapidfuzz` ni `splink`.** El ecosistema de resolucion de entidades de Python no tiene
equivalente en Node. Se usa `pg_trgm` con `unaccent` en la base, que es donde vive el catalogo, y
la normalizacion de `normalize_text` portada a TypeScript. `0007` ya previo que el escalon difuso
se sustituya cuando lleguen datos reales: al estar detras de `PuertoMotorDeSimilitud`, ese
reemplazo puede ser incluso un servicio Python, sin tocar la cascada ni el resto del sistema.

**NestJS empuja hacia dentro.** Es un framework de decoradores e inyeccion, y el camino de menor
resistencia es anotar las entidades del dominio con `@Injectable()` y devolver DTO de Nest desde
los casos de uso. Eso es exactamente la erosion que `0002` nombra como riesgo asumido. La defensa
es mecanica: `dependency-cruiser` prohibe `@nestjs/*` en `domain/` y en `application/`, y prohibe
que `domain/` importe nada fuera de si mismo.

## Alternativas consideradas

**Python 3.12 con FastAPI.** Es la opcion mas cercana al equipo —estudiantes de ingenieria en
ciencia de datos— y al trabajo ya hecho: los scripts de perfilado son Python con PEP 723, y
`rapidfuzz`, `splink` y `pandas` viven ahi. Ademas `decimal.Decimal` es de la biblioteca estandar,
asi que el requisito de `0005` no costaria una dependencia ni una disciplina extra. Descartada por
decision del equipo a favor de un solo lenguaje entre API y los tres portales. Es la alternativa a
la que volver si el matching resulta ser el cuello de botella.

**Java 21 con Spring Boot.** La respuesta convencional a "Clean Architecture empresarial", y la
que mejor cubre dos requisitos de esta ADR sin esfuerzo: `BigDecimal` en la biblioteca estandar y
**ArchUnit** como test de arquitectura maduro. Descartada por distancia al perfil del equipo: se
perderia el trabajo de perfilado ya hecho y la curva de Spring competiria con el tiempo de
entender el dominio, que es donde esta el riesgo real del proyecto.

**BullMQ con Redis para la cola.** Es la opcion por defecto en Node y esta bien documentada.
Descartada como recomendacion frente a `pg-boss` por una razon de dominio, no de gusto: `0003`
eligio monolito modular para que **una transaccion de base de datos cubra una etapa completa de
reparto, sin sagas**. Con Redis, encolar el siguiente paso queda fuera de esa transaccion y
reaparece el problema de la consistencia parcial que se quiso evitar. Sigue siendo aceptable si el
equipo prefiere Redis por familiaridad; la decision se revisa entonces con una ADR nueva.

**Un ORM con mas magia (TypeORM, Prisma).** Descartada a favor de Drizzle por dos motivos: es
SQL-primero, asi que lo que corre contra la base es legible y auditable —importa cuando hay que
explicarle una cifra a la DNDA—, y devuelve `numeric` como cadena, que es justo lo que el punto
del decimal exige.

## Consecuencias

Positivas: un solo lenguaje entre el nucleo, los adaptadores y los tres portales, lo que reduce el
coste de que cuatro personas se muevan entre partes del sistema. `pg-boss` evita operar un segundo
almacen. Y por primera vez la regla de dependencia deja de ser una convencion: `dependency-cruiser`
la comprueba en cada commit, igual que `src/scripts/check_arquitectura.py` la comprueba sobre el
diagrama.

A cambio: el decimal es una dependencia y una disciplina, no una propiedad del lenguaje; hay que
vigilar cada frontera por donde una cifra podria volverse `number`. Y el matching arranca con
`pg_trgm`, que es menos capaz que lo que el equipo sabe hacer en Python.

Riesgo asumido: que `dependency-cruiser` se configure de forma laxa —o se desactive cuando falle
en un momento inoportuno— y la frontera se erosione sin que nadie lo note, que es el mismo riesgo
que `0002` ya habia nombrado. La mitigacion es que sus contratos se traten como parte de la
definicion de hecho, y que el mismo criterio este escrito en tres sitios que se revisan: el
diagrama, esta ADR y la configuracion en CI.
