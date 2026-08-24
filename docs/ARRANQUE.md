# Arranque local

```bash
docker compose up --build   # API, worker, scheduler, Postgres y el tablero
make verificar              # tidy, build, vet, gofmt y test - lo mismo que corre CI
```

UI: <http://localhost>
API: <http://localhost/api>

Comprobar que responde:

```bash
curl -fsS http://localhost/api/health   # el proceso vive
curl -fsS http://localhost/api/ready    # el proceso vive Y la base responde
```

## Migraciones y datos

**El arranque no aplica migraciones ni siembra nada.** Antes lo hacia: la API
leia el `.sql` entero y lo ejecutaba al levantar, y sembraba usuarios si la
tabla estaba vacia -tambien en produccion. Las migraciones pasan a `goose`
como paso propio del despliegue, y el seed a un comando explicito.

Los dos entran con el PR de persistencia. Hasta entonces esto levanta el
esqueleto: los procesos arrancan, se conectan y responden a `/health`.

Cuando llegue el seed, cada usuario tendra **su propia clave, desde entorno**.
La version anterior daba la misma constante conocida a `distribucion` y
`contabilidad`, que son justo los dos roles que constituyen el control de
doble firma: una sola persona con esa clave firmaba por ambos, y el control no
controlaba nada.

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
