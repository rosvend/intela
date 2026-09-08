# Arranque local

## Quickstart: la demo en un comando

Desde un clon limpio, sin configurar nada:

```bash
docker compose --profile demo up --build
```

Eso construye las imagenes, arranca Postgres, aplica las migraciones, **siembra
el dataset de demo** y levanta API, worker, scheduler, tablero y nginx. Cuando
para de escupir logs:

| Que | Donde |
| --- | --- |
| Tablero | <http://localhost> |
| API por el proxy | <http://localhost/api> |
| Sondas | <http://localhost/health>, <http://localhost/ready> |
| Entrar | `admin@redes.co` / `admin-local` |

Comprobarlo sin abrir el navegador — recorre la misma ruta que una persona y
falla con un motivo si algo no esta:

```bash
deploy/smoke.sh
```

Cerrar:

```bash
docker compose --profile demo down       # apagar, conservando los datos
docker compose --profile demo down -v    # apagar y borrar la base y los objetos
```

**Requisitos:** Docker con Compose v2 (`docker compose version`), y `curl` y
`jq` para el smoke test. Nada mas: Go, Node y `goose` viven dentro de las
imagenes.

**El `--profile demo` es lo unico que siembra.** Un `docker compose up` pelado
levanta el sistema migrado pero **vacio**, y entrar al tablero da `credenciales
invalidas` porque la tabla `usuarios` no existe todavia. Es a proposito: ver
[Migraciones y datos](#migraciones-y-datos).

### Si el puerto 80 esta ocupado

Pasa siempre con Docker rootless, que no puede abrir puertos por debajo de
1024, y con cualquier maquina que ya tenga algo escuchando ahi. Los tres
puertos que publica el compose se mueven por entorno, sin editar ficheros:

```bash
export INTELA_PUERTO_HTTP=8088    # nginx      (por defecto 80)
export INTELA_PUERTO_API=18080    # API        (por defecto 8080)
export INTELA_PUERTO_PG=55432     # PostgreSQL (por defecto 5432)

docker compose --profile demo up --build
BASE_URL=http://localhost:8088 deploy/smoke.sh
```

`deploy/smoke.sh` lee `INTELA_PUERTO_HTTP` por su cuenta, asi que con el
`export` puesto basta con `deploy/smoke.sh`. `BASE_URL` esta para apuntarlo a
otro sitio — un stack en otra maquina, por ejemplo.

## El arranque sin demo

```bash
docker compose up -d --build   # Postgres, migraciones, API, worker, scheduler, tablero y nginx
make verificar                 # tidy, build, vet, gofmt y test - lo mismo que corre CI
```

UI: <http://localhost>
API: <http://localhost/api>

Comprobar que responde:

```bash
curl -fsS http://localhost/api/health   # el proceso vive
curl -fsS http://localhost/api/ready    # el proceso vive Y la base responde
```

**El `--build` no es opcional.** Sin el, compose reutiliza las imagenes locales
que ya esten construidas. Cuando esas imagenes son anteriores al ultimo cambio,
los contenedores arrancan sin quejarse y `/ready` responde `listo`, pero los
endpoints que no existian en esa version devuelven `404 ruta no encontrada`.
Parece un fallo del codigo y es una imagen vieja.

## Migraciones y datos

**Las migraciones si corren al arrancar**, como paso propio: el servicio
`migrate` de `docker-compose.yml` ejecuta `goose up`, termina, y solo entonces
arranca la API. Antes lo hacia la propia API al levantar, lo que significaba que
cada replica intentaba migrar en paralelo y que un fallo de migracion se
confundia con un fallo de arranque.

El seed **no corre en un `up` pelado**. Vive detras de un perfil, que es lo que
mantiene esa promesa: un arranque normal no toca los datos, y sembrar hay que
pedirlo. No hay siembra en produccion.

```bash
docker compose --profile demo up --build              # levanta Y siembra
docker compose run --rm seed                          # sembrar un stack ya arriba
SEED_RESET=true docker compose run --rm -e SEED_RESET=true seed
go run ./cmd/seed                                     # equivalente, con DATABASE_URL
```

`demo` y `seed` son **dos nombres del mismo servicio**. `--profile demo` se lee
al lado de `up` y dice lo que se quiere ("levantalo con datos"); `seed` nombra
el servicio y es lo que se escribe al lado de `run`. `docker compose run`
enciende solo el perfil del servicio que nombra, asi que no hace falta pasarlo.

Sembrar es **idempotente** cuando el dataset ya esta completo: repetir el
`--profile demo up` no duplica nada, el binario mira, ve que ya esta y sale con
0. Si la carga anterior quedo a medias, en cambio, **falla y pide
`SEED_RESET=true`** en vez de completar el hueco a ciegas.

Nada depende del seed: `api`, `worker` y `scheduler` esperan a `migrate`, no a
`seed`. Es deliberado — la API no necesita datos para arrancar, y encadenarla
convertiria un fallo de siembra en un stack que no levanta. La consecuencia es
que durante los primeros segundos del `--profile demo up` la API ya responde y
el login todavia no: por eso `deploy/smoke.sh` sondea en vez de asumir.

El binario del seed vive en **otra imagen** que la de la API: el `Dockerfile`
tiene una etapa `seed` y el servicio la pide con `target: seed`. La imagen que
publica CI y despliega el CD es la etapa `runtime`, y no lo contiene. El motivo
es lo que hace `SEED_RESET=true`: borra 21 tablas -`titulares`, `obras`,
`declaraciones`, `bolsas`, `usuarios`...- y reescribe las cuentas con las
claves de esta pagina. Que exista en la imagen de produccion es todo lo que
hace falta para que un DSN copiado de staging lo ejecute contra datos reales.

`SEED_RESET` tiene ademas su propia guarda: se **niega** si en la base hay
alguna obra o algun titular cuyo id no sea del dataset. La comprobacion
anterior solo miraba que la bitacora estuviera vacia, y el estado real de REDES
-catalogo y padron IPI cargados, ningun reparto asentado- la pasaba entera.

Cada rol tiene **su propia clave**, desde entorno. Una sola constante
compartida entre `distribucion` y `contabilidad` anula el control de doble
firma: una persona firmaba por ambos. El seed **rechaza** dos roles con la
misma clave: bcrypt lleva sal, asi que dos hashes distintos no delatan nada y
el control se perderia en silencio.

## Entrar al tablero

El seed crea los cinco usuarios. Correr **una vez** despues de un arranque
limpio o de un `docker compose down -v`; los usuarios sobreviven a `down` y a
reiniciar la maquina.

Claves por defecto (sobreescribibles con `SEED_CLAVE_*`):

| Correo | Rol | Clave | Que ve en el tablero |
| --- | --- | --- | --- |
| `admin@redes.co` | administrador | `admin-local` | Los nueve modulos |
| `distribucion@redes.co` | distribucion | `distribucion-local` | Ingesta, Catalogo, Distribucion, Anomalias |
| `contabilidad@redes.co` | contabilidad | `contabilidad-local` | Titulares y Reportes - no Distribucion |
| `auditor@redes.co` | auditor | `auditor-local` | Todo, en solo lectura |
| `ana@redes.co` | titular | `ana-local` | Solo Inicio, con su liquidacion |

`distribucion` y `contabilidad` **no se solapan** a proposito: son las dos firmas
del control de doble firma (ADR 0008, `RD 13.5`).

### Que muestra la demo

El tablero de hoy es el andamiaje del `#19`: casi todas las pantallas son
placeholders y las reemplaza el PR de cada modulo. Lo que si esta construido y
vale la pena ensenar es que **la autorizacion funciona**:

1. Entrar como `admin@redes.co` - el sidebar trae nueve items en dos secciones.
2. Salir y entrar como `ana@redes.co` - **el sidebar se reduce a uno** y el
   contenido de Inicio cambia a su liquidacion. Mismo codigo, distinta sesion.
3. Escribir `localhost/catalogo` estando como titular - responde
   **No autorizado**: no basta con esconder el enlace.
4. Ir a `localhost/estado` - dice `Backend: listo` porque consulta la API, que a
   su vez consulta Postgres. Es la prueba de que no es una maqueta.

El filtro del navegador es **cosmetico**, para no mostrar pantallas inutiles. La
autorizacion de verdad va en el servidor y es el `#17`.

### Si algo falla

| Sintoma | Causa | Arreglo |
| --- | --- | --- |
| `404 ruta no encontrada` al entrar | Imagenes viejas | `docker compose up -d --build` |
| `credenciales invalidas` | La tabla `usuarios` esta vacia | `docker compose run --rm seed` |
| La API se reinicia sola, `lookup postgres ... no such host` | Docker se reinicio y el contenedor quedo con una direccion vieja | `docker compose up -d --force-recreate api` |

### Modo desarrollo del frontend

Solo si se va a tocar codigo de `web/` y se quiere recarga automatica. Necesita
Node y son dos terminales; para **mostrar** el sistema conviene el arranque
normal, que tiene menos piezas que puedan fallar:

```bash
docker compose up -d --build postgres migrate api   # solo el backend
npm --prefix web run dev                            # http://localhost:5173
```

## Variables de entorno

| Variable | Por defecto | Para que |
| --- | --- | --- |
| `DATABASE_URL` | *(obligatoria)* | DSN de PostgreSQL. Sin ella el proceso no arranca |
| `ADDR` | `:8080` | Donde escucha la API |
| `CORS_ORIGENES` | *(vacio)* | Lista blanca separada por comas. Vacio = sin CORS. Nunca `*` |
| `OBJECT_DIR` | `./data/objetos` | Raiz del almacen de reportes crudos. Relativa a proposito: con una ruta absoluta, `go run ./cmd/seed` falla con EACCES. En contenedor la fija `docker-compose.yml` a `/objetos` |
| `LOG_FORMATO` | `json` | `texto` para desarrollo |
| `DEBUG` | `false` | Sube el nivel de log a debug |
| `SHUTDOWN_TIMEOUT` | `15s` | Margen para terminar las peticiones en vuelo |
| `WORKER_INTERVALO` | `5s` | Cada cuanto el worker mira la cola. En cada pasada la vacia entera, no toma un trabajo por tic |
| `WORKER_REINTENTOS` | `5` | Veces que se toma el mismo trabajo antes de darlo por fallido. `1` desactiva los reintentos |
| `WORKER_ESPERA_BASE` | `30s` | Espera tras el primer fallo. Se dobla en cada fallo siguiente |
| `WORKER_ESPERA_TECHO` | `10m` | Tope de esa espera. `0` significa sin tope |
| `SCHEDULER_INTERVALO` | `1m` | Cada cuanto el scheduler revisa el calendario |
| `SEED_TIMEOUT` | `2m` | Tope para la corrida entera del seed. Si expira, la carga se corta a medias y la siguiente pide `SEED_RESET=true` |
| `SEED_RESET` | `false` | Vaciar y recargar el dataset. Falla si hay asientos, y tambien si hay obras o titulares que no son del dataset |
| `SEED_CLAVE_ADMIN` | `admin-local` | Clave del usuario administrador del seed |
| `SEED_CLAVE_DISTRIBUCION` | `distribucion-local` | Clave del rol distribucion |
| `SEED_CLAVE_CONTABILIDAD` | `contabilidad-local` | Clave del rol contabilidad |
| `SEED_CLAVE_AUDITOR` | `auditor-local` | Clave del rol auditor |
| `SEED_CLAVE_TITULAR` | `ana-local` | Clave de Ana (`ana@redes.co`) |

Los cuatro valores del worker son **configuracion de operacion, no parametros normativos**: no
salen del reglamento y por eso no entran por la tabla `parametros` (ADR 0004). El detalle de por
que la cola es una tabla propia y no River esta en el
[ADR 0015](decisiones/0015-cola-de-trabajos-en-tabla-propia.md).

## Que es real y que es sintetico

Esta seccion existe para trazar esa linea. Confundirla es como se acaba
presentando una cifra inventada como si viniera del cliente.

**Real, del cliente** -incompleto, y por eso hay bloqueos abiertos:

- `data/files/CARACOL_REDES-SGC_(COLOMBIA)_20250202.xlsx` - parrilla de TV
- `data/files/Modulo identificación de Obras - Parrilla Netflix.xlsx` - catalogo OTT
- `data/IPI - form to report members to IPI 01-03-24.xls` - padron IPI

**Sintetico**, escrito a mano para poder ejecutar algo:

- `data/samples/{tv,cine,ott}.csv` - dos a cuatro filas cada uno, con titulos
  como "Pelicula X" y "Serie Y". Sirven para probar el parseo, no para
  sacar conclusiones.
- Declaraciones de obra (el export de REDES-SYS no llego)
- Bolsas de recaudo (no hay facturas)
- Rating de franja
- Coeficientes OTT `Wa/Wb/Wc`

Los parametros normativos sinteticos van etiquetados `RD-IX-seed-sintetico` en
su columna de procedencia (ADR 0004): un parametro sin vigencia y organo
aprobador no es un parametro, es una constante disfrazada.

Bloqueos y datos que faltan pedir: [`docs/dominio/fuentes-datos.md`](dominio/fuentes-datos.md).
