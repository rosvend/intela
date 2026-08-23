# 0010 Stack de aplicacion: Go

Fecha: 2026-08-23
Estado: Vigente
Sustituye a: [0009 Stack de aplicacion: TypeScript y NestJS](0009-stack-typescript-nestjs.md)

## Contexto

`0009` eligio TypeScript y NestJS hace un dia. La razon principal era tener un solo lenguaje
entre la API y los tres portales, con un paquete de contratos compartido. Esa ADR dejo escritas
tres consecuencias que habia que asumir: sin decimal nativo, sin `rapidfuzz`, y los decoradores de
NestJS empujando hacia el dominio.

Dos hechos nuevos obligan a revisarla, y conviene revisarla **ahora**: todavia no hay una linea de
codigo de aplicacion escrita, asi que el cambio es gratis. En seis semanas no lo sera.

**El primero: uno de los cuatro desarrolladores ya trabaja en Go**, y es quien tiene asignados los
adaptadores de adquisicion, la normalizacion, la VM, la observabilidad, las pruebas automatizadas y
el despliegue. Es decir, es dueno de exactamente la parte donde Go es dificil —interfaces,
`context`, concurrencia, empaquetado— mientras que el motor de dominio, que lo escribe alguien que
aprenderia el lenguaje, es la parte donde Go es mas facil: `0005` ya obliga a que sea una funcion
pura, sin E/S, sin reloj y sin concurrencia. La curva de aprendizaje cae justo donde no hay riesgo.

**El segundo: `0003` pide un solo binario con tres puntos de entrada** (`api`, `scheduler`,
`worker`). En Node eso se simula con tres ficheros de arranque que seleccionan modulos distintos.
En Go es el idioma nativo del lenguaje: tres paquetes bajo `cmd/`, un binario cada uno, el mismo
nucleo compilado una vez.

Y hay un tercer hecho que no es nuevo pero que pesa distinto visto desde aqui. `0002` y `0003`
nombraron el mismo riesgo asumido: que la frontera se erosione sola, y que nadie lo note hasta la
auditoria. La mitigacion que `0009` eligio fue `dependency-cruiser`, un fichero de configuracion
que alguien puede debilitar a las dos de la manana antes de una entrega. En Go una parte de esa
garantia deja de ser configuracion y pasa a ser el compilador.

## Decision

**Go para todo el backend. React con TypeScript se queda en el frontend.**

| Capa | Tecnologia | Que gana respecto a `0009` |
| ---- | ---------- | -------------------------- |
| `internal/dominio/` | Go + `shopspring/decimal` | El compilador prohibe importar `internal/` desde fuera de su padre |
| `internal/aplicacion/` | Casos de uso; los puertos son `interface` declaradas aqui | El adaptador **satisface la interfaz sin importarla**: la inversion de dependencia es el comportamiento por defecto del lenguaje, no una convencion |
| `internal/infraestructura/` | `chi` o `net/http` de la biblioteca estandar | Sin framework que quiera ser dueno de los handlers. Desaparece el riesgo de los decoradores |
| `cmd/{api,scheduler,worker}/` | Un `main` por punto de entrada | Es literalmente lo que pide `0003` |
| Persistencia | `pgx` v5 + `sqlc`, PostgreSQL 16 | SQL primero, con tipos generados desde el SQL. `pgx` escanea `NUMERIC` directo a `decimal.Decimal` |
| Cola | `River` (respaldada por PostgreSQL) | Encolado transaccional: conserva la transaccion local por etapa que exige `0003` |
| Migraciones | `goose` | — |
| Planificador | Temporizador leyendo `CalendarioDeDistribucion` | `0004`: el planificador no es dueno de las fechas |
| Similitud | `pg_trgm` + `unaccent` tras `PuertoMotorDeSimilitud` | Igual que en `0009`. Sin cambio |
| Objetos | MinIO local con object-lock, S3 en produccion | Sin cambio |
| Frontend | React + TypeScript + Vite | Sin cambio |
| Contratos | Especificacion OpenAPI → `openapi-typescript` | **Peor que `0009`.** Ver consecuencias |
| Exportables | `excelize` (Excel), `maroto` o Chrome headless (PDF) | — |
| Pruebas | `testing` estandar, `testcontainers-go`, `rapid` para propiedades | `rapid` cubre lo que cubria `fast-check` |
| Frontera | El compilador, mas `depguard` dentro de `golangci-lint` | Los ciclos de importacion son **error de compilacion** |
| Observabilidad | `log/slog` de la biblioteca estandar + OpenTelemetry | Una dependencia menos |
| Despliegue | Binario estatico en imagen distroless, `docker compose` | Sin runtime ni `node_modules` en la imagen |

Los scripts de `src/scripts/` **siguen en Python** con PEP 723, igual que en `0009`.

### Lo que la frontera gana en concreto

Tres cosas que en `0009` dependian de disciplina o de configuracion:

1. **Los ciclos de importacion no compilan.** Las reglas de `0003` —`Reparto` lee de `Repertorio` y
   de `Recaudo`, y ninguno de los dos conoce a `Reparto`— dejan de ser un contrato que se revisa y
   pasan a ser una condicion para que el programa exista.
2. **`internal/` es una frontera del compilador.** Un paquete bajo `internal/` no se puede importar
   desde fuera de su arbol. La frontera de modulo tiene respaldo del lenguaje.
3. **Los puertos no se importan para implementarlos.** El adaptador de PostgreSQL no menciona el
   paquete del nucleo: define los metodos y satisface la interfaz. Es la arista
   `adaptador → puerto` del diagrama, escrita como el lenguaje quiere que se escriba.

## Alternativas consideradas

**Quedarse en TypeScript y NestJS (`0009`).** Sigue siendo una opcion razonable y su argumento
principal —un lenguaje, contratos compartidos— es real y se pierde con este cambio. Se descarta
porque el desarrollador con experiencia en Go es dueno de los adaptadores, las pruebas y el
despliegue, que es donde Go rinde, y porque el motor de dominio que aprende el lenguaje es codigo
puro sin concurrencia. Si esa asignacion de roles cambia, esta ADR hay que revisarla.

**Java 21 con Spring Boot.** Sigue siendo la unica de las tres con `BigDecimal` nativo y con
ArchUnit, que es la herramienta de prueba de arquitectura mas madura que existe. Descartada otra
vez por la misma razon que en `0009`: con tres meses y cuatro personas, la curva de Spring compite
con el tiempo de entender el dominio, que es donde esta el riesgo real.

**Go en el backend y Go tambien en el frontend** con WASM o con plantillas del lado del servidor.
Descartada: OE-6 pide un panel con filtros por obra, fuente y periodo y exportables, y el
desarrollador de reporteria trabaja en React. Cambiar tambien el frontend no compra nada y cuesta
la unica parte del stack que ya estaba resuelta.

**Mantener NestJS para la API y escribir solo el motor de reparto en Go.** Descartada: dos runtimes,
dos cadenas de construccion y una frontera de proceso en medio de una transaccion que `0003` quiere
local. Se pagaria la complejidad de los microservicios sin ninguna de sus ventajas.

## Consecuencias

Positivas: la regla de dependencia de `0002` y las fronteras de modulo de `0003` pasan a estar
respaldadas por el compilador y no solo por un fichero de configuracion. `cmd/` da los tres puntos
de entrada de `0003` sin simularlos. El binario estatico simplifica la imagen y el
`docker compose` que pide `docs/context.md`. Y `pgx` escaneando `NUMERIC` a `decimal.Decimal`
elimina el punto mas silencioso de `0009`, donde una cifra podia volverse `number` sin que nada
fallara.

A cambio, y esto es el precio real del cambio: **se pierden los tipos compartidos entre la API y
los tres portales.** Ahora hace falta una especificacion OpenAPI y un paso de generacion hacia
TypeScript. Es friccion nueva en cada cambio de contrato, y es la razon por la que `0009` habia
elegido TypeScript. La mitigacion es que la generacion corra en CI y que un contrato desactualizado
rompa la construccion del frontend, no la ejecucion.

Ademas: `shopspring/decimal` es una dependencia igual que lo era `decimal.js`, asi que el decimal
exacto de `0005` sigue sin ser una propiedad del lenguaje. Y el matching difuso no mejora: sigue
siendo `pg_trgm` detras del puerto, con la puerta abierta a un servicio en Python cuando lleguen
datos reales (`0007`).

Riesgo asumido: que el motor de dominio lo escriba alguien mientras aprende el lenguaje. Se acepta
porque `0005` obliga a que ese motor sea una funcion pura —sin E/S, sin reloj, sin aleatoriedad y
sin concurrencia—, que es el Go mas sencillo que existe, y porque hay alguien en el equipo a quien
preguntarle. La mitigacion es que las dos primeras semanas el motor se escriba en pareja, y que las
pruebas de reproducibilidad de `0005`, con los ejemplos numericos del propio reglamento, esten
desde el primer commit del motor y no al final.

Riesgo secundario: que la especificacion OpenAPI se quede atras respecto al codigo y el frontend
consuma tipos que ya no existen. Por eso la generacion es un paso de CI y no un comando que alguien
recuerda correr.
