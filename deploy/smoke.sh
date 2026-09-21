#!/usr/bin/env bash
#
# Prueba de humo del stack de docker-compose.
#
# Responde a una pregunta que el resto del pipeline no responde: *el sistema
# arranca y sirve*. `go build` dice que compila, `docker build` dice que la
# imagen se construye, y con las dos en verde el stack puede seguir sin
# levantar -un DSN mal escrito, una migracion que no aplica, el `proxy_pass`
# sin barra final que manda /api/obras a la API como /api/obras y devuelve 404
# a todo-. Nada de eso se ve hasta que alguien abre el navegador.
#
# Se prueba lo que ve un usuario, a traves de nginx y en el puerto publicado,
# no contenedor por contenedor: el proxy es parte del sistema y es donde viven
# la mitad de los fallos de integracion.
#
# Uso:
#   deploy/smoke.sh                       contra http://localhost
#   SMOKE_BASE_URL=http://localhost:8088 deploy/smoke.sh
#
# Variables:
#   SMOKE_BASE_URL   Raiz publica, la de nginx. Por defecto http://localhost
#   SMOKE_ESPERA     Segundos de margen por comprobacion. Por defecto 180
#   SMOKE_EMAIL      Usuario con el que se entra. Por defecto admin@redes.co
#   SMOKE_CLAVE      Su clave. Por defecto SEED_CLAVE_ADMIN, o admin-local
#   SMOKE_COMPOSE    Orden de compose. Por defecto "docker compose --profile
#                    demo": sin el perfil, `logs` en fatal() no ve el
#                    contenedor del seed -Compose v2 lo excluye de ese
#                    comando en cuanto el servicio declara `profiles` y el
#                    perfil no esta activo- aunque `ps --all` si lo liste, y
#                    el seed es el fallo mas probable de toda la funcionalidad.
#   SMOKE_SIN_COMPOSE  =1 para omitir el estado de los contenedores (util
#                      cuando se apunta a un despliegue remoto)
#
# Sale 0 si el stack sirve, 1 si no, y en ese caso deja en el log el estado de
# los contenedores y la cola de sus logs: un fallo aqui tiene que ser
# diagnosticable sin volver a levantar nada.

set -Eeuo pipefail

BASE="${SMOKE_BASE_URL:-http://localhost}"
BASE="${BASE%/}"
ESPERA="${SMOKE_ESPERA:-180}"
EMAIL="${SMOKE_EMAIL:-admin@redes.co}"
CLAVE="${SMOKE_CLAVE:-${SEED_CLAVE_ADMIN:-admin-local}}"
read -r -a COMPOSE <<<"${SMOKE_COMPOSE:-docker compose --profile demo}"

# Los servicios que tienen que seguir en pie al final. `migrate` y `seed` no
# estan: son de una sola pasada y se comprueban aparte, por su codigo de
# salida (comprobacion 1/7 mas abajo), no por lo que el resto del script
# infiera de ellos.
SERVICIOS=(postgres api worker scheduler web nginx)

# Servicios de una sola pasada (comprobados en una_pasada_ok(), mas abajo).
# Antes de esto, el resto del script "demostraba" su exito por indirectas -si
# la base no estuviera migrada no habria /ready, y si no estuviera sembrada no
# habria con quien entrar-, y esas indirectas son falsas en cuanto la base YA
# tenia datos de una corrida anterior: un `seed` que falla a medias dice adios
# con un `warning` (`required: false` en docker-compose.yml, deliberado: es lo
# que permite `up` sin --profile demo) y los usuarios y obras de la corrida
# anterior siguen ahi para que el login y `/api/obras` pasen igual.
#
# `migrate` es obligatorio; `seed` solo existe si el perfil `seed` o `demo`
# esta activo, asi que su ausencia no es un fallo -es un `up` pelado-.
UNA_PASADA=(migrate seed)
UNA_PASADA_OBLIGATORIO=migrate

# ---------------------------------------------------------------------------
# Salida

rojo=""
verde=""
neutro=""
if [ -t 1 ]; then
  rojo=$'\033[31m'
  verde=$'\033[32m'
  neutro=$'\033[0m'
fi

paso() { printf '\n== %s\n' "$1"; }
ok() { printf '   %sok%s  %s\n' "$verde" "$neutro" "$1"; }

# fatal imprime el motivo y VUELCA EL ESTADO antes de salir.
#
# Sin el volcado, un fallo en CI deja "la peticion no respondio" y nada mas: hay
# que reproducir el arranque entero en local para enterarse de que lo que pasaba
# era que la migracion 00011 no aplicaba. Los logs ya estan ahi en el momento
# del fallo; no recogerlos es tirarlos.
fatal() {
  printf '\n%sFALLO%s  %s\n' "$rojo" "$neutro" "$1" >&2

  if [ "${SMOKE_SIN_COMPOSE:-0}" != "1" ] && command -v "${COMPOSE[0]}" >/dev/null 2>&1; then
    printf '\n--- estado de los contenedores ---\n' >&2
    "${COMPOSE[@]}" ps --all >&2 || true
    printf '\n--- ultimas 60 lineas de cada servicio ---\n' >&2
    "${COMPOSE[@]}" logs --tail 60 --no-color >&2 || true
  fi

  exit 1
}

# Red de seguridad para lo que "set -e" por si solo no verbaliza: una
# asignacion desnuda como `x=$(curl ...)` aborta el script sin pasar por
# fatal() si curl falla en el transporte (timeout, conexion rechazada) en vez
# de responder con un codigo HTTP. El trap convierte ese abort silencioso en
# el mismo volcado de diagnostico que ya usan las comprobaciones explicitas.
trap 'fatal "fallo inesperado en la linea $LINENO: revisa la salida de arriba"' ERR

# esperar repite una comprobacion hasta que pasa o hasta agotar SMOKE_ESPERA.
#
# Existe porque `up -d` devuelve cuando los contenedores ARRANCARON, no cuando
# el sistema esta listo: Postgres todavia acepta conexiones a medias, la API
# aun no abrio el puerto y el seed puede seguir insertando. Un smoke test sin
# espera no prueba el stack, prueba la velocidad de la maquina.
esperar() {
  local que="$1"
  shift
  local fin=$((SECONDS + ESPERA))
  until "$@"; do
    if ((SECONDS >= fin)); then
      fatal "$que: seguia sin cumplirse tras ${ESPERA}s"
    fi
    sleep 2
  done
  ok "$que"
}

# codigo devuelve solo el codigo HTTP. Sin -f: aqui un 401 es una respuesta
# valida que hay que poder comparar, no un error de curl.
codigo() { curl -s -o /dev/null -w '%{http_code}' --max-time 10 "$@"; }

# una_pasada_ok exige que los servicios de una sola pasada hayan TERMINADO y
# hayan terminado BIEN, preguntandoselo al contenedor (`ps --all`), no
# deduciendolo de que la base tenga datos -que es de donde salia el verde
# falso: los datos podian ser de la corrida de ayer-.
#
# Se exige `exited` ademas de `ExitCode == 0` porque un contenedor que todavia
# corre, o que se quedo en `created` y nunca arranco, tambien lleva `ExitCode`
# 0 en esta salida: sin mirar el estado, "no ha terminado" se leeria como
# "termino bien", que es la misma clase de fallo que esto viene a cerrar.
#
# La forma de `ps --format json` NO es estable entre versiones de Compose: unas
# devuelven un array JSON y otras una linea por contenedor (NDJSON). `-s` mas
# `flatten(1)` acepta las dos. Importa mas de lo que parece: si la guarda se
# rompiera por la version de Compose, se romperia en CI -que es el unico sitio
# donde nadie la esta mirando-.
una_pasada_ok() {
  local crudo estado servicio situacion codigo_salida visto_obligatorio=0
  local nombres_json

  crudo=$("${COMPOSE[@]}" ps --all --format json 2>/dev/null) ||
    fatal "no se pudo consultar '${COMPOSE[*]} ps --all --format json'"

  nombres_json=$(printf '%s\n' "${UNA_PASADA[@]}" |
    jq -R -s -c 'split("\n") | map(select(length > 0))')

  estado=$(jq -r -s --argjson nombres "$nombres_json" \
    'flatten(1)[]
     | select(.Service as $s | $nombres | index($s))
     | "\(.Service) \(.State) \(.ExitCode)"' <<<"$crudo") ||
    fatal "no se pudo interpretar la salida de '${COMPOSE[*]} ps --all --format json'"

  while read -r servicio situacion codigo_salida; do
    [ -n "$servicio" ] || continue
    if [ "$servicio" = "$UNA_PASADA_OBLIGATORIO" ]; then
      visto_obligatorio=1
    fi
    if [ "$situacion" != "exited" ]; then
      fatal "el servicio de una sola pasada '$servicio' no termino: sigue en '$situacion'"
    fi
    if [ "$codigo_salida" != "0" ]; then
      fatal "el servicio de una sola pasada '$servicio' termino con codigo $codigo_salida: revisa sus logs abajo"
    fi
  done <<<"$estado"

  # Sin `migrate` en la salida, `SMOKE_COMPOSE` no esta mirando el proyecto que
  # se levanto, y entonces ni esta comprobacion ni la 7/7 significan nada.
  if [ "$visto_obligatorio" != "1" ]; then
    fatal "no aparece el contenedor de '$UNA_PASADA_OBLIGATORIO' en '${COMPOSE[*]} ps --all': revisa SMOKE_COMPOSE"
  fi
}

# ---------------------------------------------------------------------------
# Comprobaciones

for bin in curl jq; do
  command -v "$bin" >/dev/null 2>&1 ||
    fatal "hace falta '$bin' en el PATH para correr la prueba de humo"
done

printf 'Prueba de humo contra %s\n' "$BASE"

# 1. Los pasos de una sola pasada (migrate, seed) salieron con 0. Sin esto,
#    una base ya sembrada de una corrida anterior deja pasar un `seed` roto:
#    las comprobaciones 5 y 6 de mas abajo (login y catalogo) no distinguen
#    "sembrado" de "sembrado a medias por la corrida de HOY": solo miran
#    que haya datos, y los datos pueden ser de ayer.
paso "1/7  migrate y seed (si esta en el perfil) terminaron con exito"
if [ "${SMOKE_SIN_COMPOSE:-0}" = "1" ]; then
  ok "omitido (SMOKE_SIN_COMPOSE=1)"
elif ! command -v "${COMPOSE[0]}" >/dev/null 2>&1; then
  ok "omitido: '${COMPOSE[0]}' no esta en el PATH"
else
  una_pasada_ok
  ok "servicios de una sola pasada en 0"
fi

# 2. La sonda de disponibilidad. NO es /health: /health solo dice que el
#    proceso vive, /ready dice que ademas la base responde, que es lo que
#    distingue "arranco" de "sirve".
paso "2/7  la API responde a traves de nginx"
ready() { [ "$(codigo "$BASE/ready")" = "200" ]; }
esperar "GET $BASE/ready -> 200" ready

# El prefijo /api/ es un camino DISTINTO en nginx.conf, con su propia barra
# final en el proxy_pass. Que /ready funcione no dice nada sobre el, y es el
# que usa el tablero entero.
[ "$(codigo "$BASE/api/health")" = "200" ] ||
  fatal "GET $BASE/api/health no devolvio 200: revisa el proxy_pass de /api/ en deploy/nginx.conf"
ok "GET $BASE/api/health -> 200 (el prefijo /api/ enruta)"

# 3. El tablero, servido por el contenedor `web` a traves de nginx.
paso "3/7  el tablero web se sirve por nginx"
raiz=$(curl -fsS --max-time 10 "$BASE/") ||
  fatal "GET $BASE/ no respondio: nginx no esta sirviendo la SPA"
grep -q 'id="root"' <<<"$raiz" ||
  fatal "GET $BASE/ respondio, pero el cuerpo no es el index de la SPA (falta el div #root)"
ok "GET $BASE/ -> index de la SPA"

# El index es HTML estatico: se sirve igual aunque el build de Vite haya salido
# vacio y no haya aplicacion ninguna. El bundle es lo que distingue "nginx
# responde" de "el tablero carga", y ademas prueba el `location /assets/` de
# web/nginx.conf, que es otro camino distinto.
# awk con match()+exit, no grep+head. `grep -m1 -o` para en la primera LINEA
# que casa, pero -o sigue imprimiendo TODAS las coincidencias de esa linea: si
# el index trae mas de un bundle en la misma linea (un <script> y un
# modulepreload, por ejemplo), la salida tiene varias rutas y `codigo
# "$BASE$bundle"` pide una URL rota. Encadenar `| head -n1` detras arregla eso
# pero reintroduce el problema original para una linea larga: `head` puede
# seguir cerrando la tuberia a mitad de la escritura de `grep`, SIGPIPE, y con
# `pipefail` el `|| bundle=""` de abajo pisaria un valor que ya se habia
# capturado bien. match()+exit es un solo proceso: sin tuberia, no hay lector
# que pueda matar al escritor a mitad de nada, sea la linea del largo que sea.
bundle=$(awk 'match($0, /\/assets\/[^"]*\.js/) { print substr($0, RSTART, RLENGTH); exit }' <<<"$raiz") ||
  bundle=""
[ -n "$bundle" ] ||
  fatal "el index no referencia ningun bundle en /assets/: el build del tablero salio vacio"
[ "$(codigo "$BASE$bundle")" = "200" ] ||
  fatal "GET $BASE$bundle no devolvio 200: el tablero no puede cargar"
ok "GET $BASE$bundle -> 200 (el bundle del tablero carga)"

# 4. Una ruta protegida SIN credencial. Un 200 aqui seria un agujero, y un 404
#    querria decir que la ruta ni existe -las dos cosas se ven igual de bien
#    desde fuera si solo se mira que "algo responde"-.
paso "4/7  las rutas protegidas piden sesion"
sin_token=$(codigo "$BASE/api/obras")
[ "$sin_token" = "401" ] ||
  fatal "GET $BASE/api/obras sin token devolvio $sin_token, se esperaba 401"
ok "GET $BASE/api/obras sin token -> 401"

# 5. Entrar. Esto es lo que espera al seed: los usuarios los crea el, asi que
#    hasta que no termina no hay con quien iniciar sesion.
paso "5/7  se puede iniciar sesion con el usuario del seed"
sesion=""
login() {
  sesion=$(curl -fsS --max-time 10 -X POST "$BASE/api/auth/session" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"clave\":\"$CLAVE\"}" 2>/dev/null) || return 1
  jq -e '.token | strings and (length > 0)' >/dev/null 2>&1 <<<"$sesion"
}
esperar "POST $BASE/api/auth/session como $EMAIL" login
token=$(jq -r '.token' <<<"$sesion")
rol=$(jq -r '.usuario.rol' <<<"$sesion")
ok "sesion abierta con rol '$rol'"

# 6. Un endpoint de negocio de verdad, con datos de verdad. Que devuelva 200 no
#    basta: el catalogo vacio tambien es un 200, y es exactamente el sintoma de
#    "levanto pero nadie lo sembro", que es lo que este perfil existe para
#    evitar.
paso "6/7  el catalogo devuelve las obras sembradas"
obras=""
catalogo() {
  obras=$(curl -fsS --max-time 10 -H "Authorization: Bearer $token" \
    "$BASE/api/obras") || return 1
  [ "$(jq 'if type == "array" then length else 0 end' <<<"$obras")" -ge 1 ]
}
esperar "GET $BASE/api/obras -> al menos una obra" catalogo
printf '   %d obras en el catalogo: %s\n' \
  "$(jq 'length' <<<"$obras")" \
  "$(jq -r '[.[].titulo] | join(", ")' <<<"$obras")"

# 7. Lo que HTTP no puede ver. `worker` y `scheduler` no publican puerto: si
#    uno de los dos esta reiniciandose en bucle, todo lo de arriba sigue en
#    verde y el sistema esta roto igual.
#
# `ps --status running` es una foto: un contenedor con `restart:
# unless-stopped` que muere y revive cada par de segundos esta "running" en
# casi cualquier instante en que se le mire, asi que una sola foto no
# distingue un bucle de reinicio de un servicio sano. Se pide la foto dos
# veces con margen entre medias -sin fiarse de RestartCount, que podman no
# expone igual que Docker- para que un servicio que muere en el hueco salga
# en al menos una de las dos como no corriendo.
#
# COBERTURA PARCIAL, a proposito y declarada: esto atrapa el bucle rapido -el
# que revive cada pocos segundos, que es la forma que toma un servicio que
# muere al arrancar-, no cualquier bucle. Uno cuya ventana de "running" pase
# de los 3 s de margen puede salir "en pie" en las dos fotos. Cerrarlo de
# verdad pide contar reinicios, y `RestartCount` no se lee igual en Docker
# que en podman, que son los dos entornos donde esto tiene que correr. Lo que
# NO se puede hacer es presentarlo como cubierto: un bucle lento pasa.
en_pie() {
  local corriendo
  corriendo=$("${COMPOSE[@]}" ps --status running --services 2>/dev/null) || return 1
  local servicio
  for servicio in "${SERVICIOS[@]}"; do
    grep -qx "$servicio" <<<"$corriendo" || return 1
  done
}

paso "7/7  todos los servicios siguen en pie"
if [ "${SMOKE_SIN_COMPOSE:-0}" = "1" ]; then
  ok "omitido (SMOKE_SIN_COMPOSE=1)"
elif ! command -v "${COMPOSE[0]}" >/dev/null 2>&1; then
  ok "omitido: '${COMPOSE[0]}' no esta en el PATH"
else
  en_pie ||
    fatal "no se pudo consultar '${COMPOSE[*]} ps', o algun servicio de ${SERVICIOS[*]} no esta corriendo"
  sleep 3
  en_pie ||
    fatal "algun servicio de ${SERVICIOS[*]} dejo de estar en pie entre dos comprobaciones: parece un bucle de reinicio"
  ok "en pie: ${SERVICIOS[*]} (comprobado dos veces, con 3s de margen)"
fi

printf '\n%sLa prueba de humo paso%s: el stack arranca y sirve.\n' "$verde" "$neutro"
