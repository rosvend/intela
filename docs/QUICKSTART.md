# Quickstart

De un clon recien hecho a un sistema que se puede ensenar, en **una orden**.

Esta pagina cubre solo eso: el arranque de demo y como comprobarlo. El detalle
-variables de entorno, que dato es real y cual sintetico, modo desarrollo del
frontend- esta en [`ARRANQUE.md`](ARRANQUE.md).

## Prerequisitos

| | |
| --- | --- |
| **Docker Engine** con **Compose v2 o posterior** | `docker compose version` tiene que responder. Lo que descarta es el guion (`docker-compose`, la v1 en Python): no entiende `--profile` y aqui hace falta. La prueba util es `docker compose --profile demo config`, que funciona o no; el numero exacto no dice tanto -con podman, `docker compose version` puede contestar con la version del proveedor de compose o con la de podman segun como este instalado- |
| **Nada mas** | No hace falta Go ni Node instalados: el backend y el tablero se compilan dentro de los contenedores |
| **Puertos 80, 8080 y 5432 libres** | nginx, la API y Postgres los publican en el host |
| **~3 GB de disco** | Contando las imagenes de compilacion (`golang`, `node`), que no viajan en las finales |

`podman` sirve igual de bien, con el socket compatible activo:

```bash
systemctl --user enable --now podman.socket
export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock
```

## La orden

```bash
docker compose --profile demo up --build
```

Eso es todo. En orden, compose: levanta Postgres y espera a que este sano,
aplica las migraciones (`migrate`, que corre y termina), **siembra el dataset
sintetico** (`seed`, que tambien corre y termina) y deja en pie la API, el
worker, el scheduler, el tablero y nginx.

El **perfil `demo` es lo que anade el `seed`**. Un `up` sin el deja la base
migrada y vacia: el sistema arranca, `/ready` responde, y el tablero pinta cero
obras.

Anadir `-d` lo deja en segundo plano. El `--build` no es opcional la primera
vez -no hay imagenes- ni despues de cambiar codigo: sin el, compose reutiliza
imagenes viejas y los endpoints nuevos responden `404 ruta no encontrada`, que
se lee como un fallo del codigo y es una imagen rancia.

Sembrar es **idempotente**: repetir el `up` con la base ya sembrada no duplica
nada, el sembrador detecta el dataset completo y sale con exito. Para recargar
de cero, `docker compose --profile demo down -v` y volver a levantar -no
`SEED_RESET=true`: en cuanto hay un solo asiento en la bitacora (basta con
haber tocado el tablero), el seed rechaza el reset con `ErrBitacoraNoVacia`
en vez de borrar el libro de auditoria. `SEED_RESET=true` solo recarga en la
ventana entre `migrate` y el primer asiento; pasada esa ventana, `down -v` es
el unico camino. Tambien es el unico que limpia `snapshots_parametros` -el
corte congelado de una corrida-: esa tabla es inmutable por trigger y el
sembrador no la toca, asi que un reset no la vacia.

## Que queda levantado

| URL | Que es |
| --- | --- |
| <http://localhost> | El tablero |
| <http://localhost/api/health> | La API, detras del mismo nginx -notese la barra final: `/api` a secas devuelve **301** a `/api/`, y `/api/` devuelve **404** `{"error":"ruta no encontrada"}`, que es la API contestando que la raiz no es una ruta suya. Ninguno de los dos sirve el tablero |
| <http://localhost/ready> | Sonda: el proceso vive **y** la base responde |
| <http://localhost/health> | Sonda: solo que el proceso vive |

Para entrar al tablero, `admin@redes.co` / `admin-local`. Los otros cuatro roles
y sus claves estan en [`ARRANQUE.md`](ARRANQUE.md#entrar-al-tablero); cada uno ve
un tablero distinto, que es la parte de la demo que vale la pena ensenar.

## Comprobarlo

```bash
deploy/smoke.sh
```

Es exactamente el script que corre CI, sin una version paralela que pueda
pudrirse. Ojo con el "en cada pull request": la etapa se filtra por rutas
(`P_SMOKE` en `.github/workflows/ci.yml`), y ese patron **no incluye
`docs/**`**, asi que un PR que solo toque documentacion no la corre. Es
deliberado -es la etapa mas cara del pipeline-, pero significa que "CI ya lo
probo" no se puede dar por hecho en un PR de solo documentacion.

No se conforma con que los contenedores esten arriba: pasa por nginx como lo
haria un navegador y comprueba lo que tiene que ser cierto para que el sistema
*sirva*.

1. `migrate` y -si el perfil lo incluye- `seed` **terminaron con codigo 0**.
   Se le pregunta al contenedor, no a la base: con una base que ya tenia datos
   de ayer, un `seed` que falla hoy deja pasar las comprobaciones 5 y 6, que
   solo miran que haya datos.
2. `/ready` responde 200 -la API llega a Postgres- **y** `/api/health` responde
   200 -el prefijo `/api/` enruta; es otro camino en nginx y se rompe por su
   cuenta-.
3. La raiz devuelve el index del tablero **y su bundle de JavaScript carga**.
4. `/api/obras` sin token responde 401.
5. Se puede iniciar sesion con el usuario del seed.
6. `/api/obras` con token devuelve **al menos una obra**: es lo que distingue
   "levanto" de "levanto sembrado".
7. `worker` y `scheduler` siguen en pie, comprobado dos veces con 3 s entre
   medias. No publican puerto, asi que ninguna de las comprobaciones anteriores
   los mira. Atrapa el bucle de reinicio **rapido** -el que muere al arrancar y
   revive cada pocos segundos-; uno cuya ventana de "running" pase de esos 3 s
   puede salir en pie en las dos fotos. Contar reinicios cerraria el hueco,
   pero `RestartCount` no se lee igual en Docker que en podman.

Sale 0 si todo pasa. Si algo falla, sale 1 diciendo que comprobacion fue y
vuelca el estado de los contenedores y la cola de sus logs, para no tener que
reproducir el arranque entero.

Necesita `curl` y `jq`. Contra otro sitio -o con los puertos cambiados-:

```bash
SMOKE_BASE_URL=http://localhost:8088 deploy/smoke.sh
```

## Parar y limpiar

```bash
docker compose --profile demo stop      # pausar; los datos siguen ahi
docker compose --profile demo down      # quitar los contenedores; los datos siguen ahi
docker compose --profile demo down -v   # ademas borra los volumenes: base y reportes crudos
```

El `--profile demo` **tambien hace falta para bajar**: sin el, compose no sabe
que el contenedor del seed es suyo y lo deja atras.

Un `down -v` deja el siguiente arranque igual que el primero -Postgres nace
vacio, migraciones y seed vuelven a correr-. Es lo que hay que hacer antes de
grabar una demo, y lo que hay que hacer si la base quedo a medias.

Para recuperar tambien el disco de las imagenes:

```bash
docker compose --profile demo down -v --rmi local
```

## Si algo falla

| Sintoma | Causa | Arreglo |
| --- | --- | --- |
| `bind: address already in use` | Otro proceso tiene el 80, el 8080 o el 5432 | Liberarlo, o publicar otros puertos con un `compose.override.yml` |
| nginx muere en bucle con `open() ".../default.conf" failed (13: Permission denied)` | Host con SELinux y un montaje sin reetiquetar | Ya cubierto: el volumen de nginx lleva `:z`. Si sigue, `docker compose up --force-recreate nginx` |
| `rootlessport listen tcp 0.0.0.0:80: bind: permission denied` | podman sin privilegios no puede publicar puertos por debajo de `net.ipv4.ip_unprivileged_port_start` | `sudo sysctl net.ipv4.ip_unprivileged_port_start=80`, o publicar el tablero en otro puerto |
| `404 ruta no encontrada` en rutas que existen | Imagenes viejas | `docker compose --profile demo up -d --build` |
| `credenciales invalidas` | La base esta migrada pero sin sembrar (se arranco sin `--profile demo`) | `docker compose run --rm seed` |
| El seed dice `semilla a medias` | Una corrida anterior se corto por la mitad | `SEED_RESET=true docker compose --profile demo up`, y si lo rechaza con `ErrBitacoraNoVacia` (ya hay un asiento en la bitacora), `docker compose --profile demo down -v` en su lugar |

Mas sintomas y el detalle de las variables, en [`ARRANQUE.md`](ARRANQUE.md).
