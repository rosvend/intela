# ONI tardios: publicaciones complementarias (#184)

`00023` dejo `UNIQUE (periodo)` en `oni_publicaciones`. Un uso que llegaba
despues (reporte tardio o uso reabierto) y quedaba en `escalon = 'oni'` no se
podia publicar: republicar el periodo respondia 409 y `publicado_en` seguia
en NULL, asi que el reloj de R-19 (RD 13.8.7) no arrancaba.

La migracion `00027_oni_publicaciones_complementarias.sql` quita
`UNIQUE (periodo)`, agrega `secuencia` con `UNIQUE (periodo, secuencia)` y un
unico por `oni_publicacion_items(uso_id)`. El down vuelve a `UNIQUE (periodo)`:
falla, a proposito, si ya hay una complementaria.

`BloquearPeriodoONI` toma `pg_advisory_xact_lock` por periodo, no un lock de
fila. Hay que serializar la publicacion entera (pendientes, secuencia e
items) para que dos POST concurrentes no lean los mismos pendientes.

Cada obra conserva el ancla de la publicacion que la incluyo
(`usos.publicado_en`, `oni_publico.fecha_proceso`, `obras[].fecha_proceso`).
La cabecera de `GET /publico/oni` es la secuencia mas reciente y no reescribe
las anteriores. `POST /oni/publicaciones` responde solo las obras de la
secuencia recien creada.

El 00027 es el primero libre por encima de `00026_correccion_de_anomalias.sql`
en `main`. goose corre con `allowMissing = false`: un numero por debajo de la
version ya aplicada, o dos ficheros con el mismo numero, tumban el despliegue.
El #199 tambien pide 00027 y no esta en `main`; el que entre segundo tiene que
volver a tomar el primero libre.
