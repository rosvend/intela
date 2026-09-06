# Arranque local

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

**El seed no existe todavia** (`#22`). No hay `cmd/seed`, asi que tras un
arranque limpio la tabla `usuarios` esta **vacia** y no se puede entrar al
tablero. Para trabajar o para una demo, ver la seccion siguiente.

Cuando llegue el seed, cada usuario tendra **su propia clave, desde entorno**.
La version anterior daba la misma constante conocida a `distribucion` y
`contabilidad`, que son justo los dos roles que constituyen el control de
doble firma: una sola persona con esa clave firmaba por ambos, y el control no
controlaba nada.

## Entrar al tablero

Mientras `#22` no exista, los usuarios se crean a mano. **Esto es solo para
desarrollo**: no se commitea como script para no pisar el trabajo de ese issue,
y la clave es la misma para los cinco a proposito -es un juego de prueba, no un
esquema de credenciales.

Correr **una vez** despues de un arranque limpio o de un `docker compose down -v`;
los usuarios sobreviven a `down` y a reiniciar la maquina.

```bash
docker compose exec -T postgres psql -U intela -d intela <<'SQL'
INSERT INTO titulares (id, nombre, ipi, persona_natural, clase, email)
VALUES ('tit-ana', 'Ana Escritora', 'IPI-00000001', TRUE, 'socio', 'ana@redes.co')
ON CONFLICT (id) DO NOTHING;

INSERT INTO usuarios (id, email, nombre, rol, titular_id, password_hash) VALUES
  ('usr-admin', 'admin@redes.co', 'Admin Intela',   'administrador', NULL,      '$2a$10$8HlgoFDxy5G6rLR8ewFRMesmXVIUkgb3etEMOoYqsPT778JCAbH5q'),
  ('usr-dist',  'dist@redes.co',  'Distribucion',   'distribucion',  NULL,      '$2a$10$8HlgoFDxy5G6rLR8ewFRMesmXVIUkgb3etEMOoYqsPT778JCAbH5q'),
  ('usr-conta', 'conta@redes.co', 'Contabilidad',   'contabilidad',  NULL,      '$2a$10$8HlgoFDxy5G6rLR8ewFRMesmXVIUkgb3etEMOoYqsPT778JCAbH5q'),
  ('usr-audit', 'audit@redes.co', 'Revisor Fiscal', 'auditor',       NULL,      '$2a$10$8HlgoFDxy5G6rLR8ewFRMesmXVIUkgb3etEMOoYqsPT778JCAbH5q'),
  ('usr-ana',   'ana@redes.co',   'Ana Escritora',  'titular',       'tit-ana', '$2a$10$8HlgoFDxy5G6rLR8ewFRMesmXVIUkgb3etEMOoYqsPT778JCAbH5q')
ON CONFLICT (id) DO NOTHING;
SQL
```

Los cinco usan la clave `intela-dev`:

| Correo | Rol | Que ve en el tablero |
| --- | --- | --- |
| `admin@redes.co` | administrador | Los nueve modulos |
| `dist@redes.co` | distribucion | Ingesta, Catalogo, Distribucion, Anomalias |
| `conta@redes.co` | contabilidad | Titulares y Reportes - no Distribucion |
| `audit@redes.co` | auditor | Todo, en solo lectura |
| `ana@redes.co` | titular | Solo Inicio, con su liquidacion |

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
| `credenciales invalidas` | La tabla `usuarios` esta vacia | Repetir el bloque de arriba |
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
| `OBJECT_DIR` | `/data/objetos` | Raiz del almacen de reportes crudos |
| `LOG_FORMATO` | `json` | `texto` para desarrollo |
| `DEBUG` | `false` | Sube el nivel de log a debug |
| `SHUTDOWN_TIMEOUT` | `15s` | Margen para terminar las peticiones en vuelo |
| `WORKER_INTERVALO` | `5s` | Cada cuanto el worker mira la cola |
| `SCHEDULER_INTERVALO` | `1m` | Cada cuanto el scheduler revisa el calendario |

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
