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
#   SMOKE_COMPOSE    Orden de compose. Por defecto "docker compose"
#   SMOKE_SIN_COMPOSE  =1 para omitir el estado de los contenedores (util
#                      cuando se apunta a un despliegue remoto)
#
# Sale 0 si el stack sirve, 1 si no, y en ese caso deja en el log el estado de
# los contenedores y la cola de sus logs: un fallo aqui tiene que ser
# diagnosticable sin volver a levantar nada.

set -euo pipefail

BASE="${SMOKE_BASE_URL:-http://localhost}"
BASE="${BASE%/}"
ESPERA="${SMOKE_ESPERA:-180}"
EMAIL="${SMOKE_EMAIL:-admin@redes.co}"
CLAVE="${SMOKE_CLAVE:-${SEED_CLAVE_ADMIN:-admin-local}}"
read -r -a COMPOSE <<<"${SMOKE_COMPOSE:-docker compose}"

# Los servicios que tienen que seguir en pie al final. `migrate` y `seed` no
# estan: son de una sola pasada y su exito lo demuestra el resto del script
# -si la base no estuviera migrada no habria /ready, y si no estuviera sembrada
# no habria con quien entrar-.
SERVICIOS=(postgres api worker scheduler web nginx)

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
    "${COMPOSE[@]}" ps --all >&2 2>&1 || true
    printf '\n--- ultimas 60 lineas de cada servicio ---\n' >&2
    "${COMPOSE[@]}" logs --tail 60 --no-color >&2 2>&1 || true
  fi

  exit 1
}

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

# ---------------------------------------------------------------------------
# Comprobaciones

for bin in curl jq; do
  command -v "$bin" >/dev/null 2>&1 ||
    fatal "hace falta '$bin' en el PATH para correr la prueba de humo"
done

printf 'Prueba de humo contra %s\n' "$BASE"

# 1. La sonda de disponibilidad. NO es /health: /health solo dice que el
#    proceso vive, /ready dice que ademas la base responde, que es lo que
#    distingue "arranco" de "sirve".
paso "1/6  la API responde a traves de nginx"
ready() { [ "$(codigo "$BASE/ready")" = "200" ]; }
esperar "GET $BASE/ready -> 200" ready

# El prefijo /api/ es un camino DISTINTO en nginx.conf, con su propia barra
# final en el proxy_pass. Que /ready funcione no dice nada sobre el, y es el
# que usa el tablero entero.
[ "$(codigo "$BASE/api/health")" = "200" ] ||
  fatal "GET $BASE/api/health no devolvio 200: revisa el proxy_pass de /api/ en deploy/nginx.conf"
ok "GET $BASE/api/health -> 200 (el prefijo /api/ enruta)"

# 2. El tablero, servido por el contenedor `web` a traves de nginx.
paso "2/6  el tablero web se sirve por nginx"
raiz=$(curl -fsS --max-time 10 "$BASE/") ||
  fatal "GET $BASE/ no respondio: nginx no esta sirviendo la SPA"
grep -q 'id="root"' <<<"$raiz" ||
  fatal "GET $BASE/ respondio, pero el cuerpo no es el index de la SPA (falta el div #root)"
ok "GET $BASE/ -> index de la SPA"

# El index es HTML estatico: se sirve igual aunque el build de Vite haya salido
# vacio y no haya aplicacion ninguna. El bundle es lo que distingue "nginx
# responde" de "el tablero carga", y ademas prueba el `location /assets/` de
# web/nginx.conf, que es otro camino distinto.
bundle=$(grep -o '/assets/[^"]*\.js' <<<"$raiz" | head -1) ||
  bundle=""
[ -n "$bundle" ] ||
  fatal "el index no referencia ningun bundle en /assets/: el build del tablero salio vacio"
[ "$(codigo "$BASE$bundle")" = "200" ] ||
  fatal "GET $BASE$bundle no devolvio 200: el tablero no puede cargar"
ok "GET $BASE$bundle -> 200 (el bundle del tablero carga)"

# 3. Una ruta protegida SIN credencial. Un 200 aqui seria un agujero, y un 404
#    querria decir que la ruta ni existe -las dos cosas se ven igual de bien
#    desde fuera si solo se mira que "algo responde"-.
paso "3/6  las rutas protegidas piden sesion"
sin_token=$(codigo "$BASE/api/obras")
[ "$sin_token" = "401" ] ||
  fatal "GET $BASE/api/obras sin token devolvio $sin_token, se esperaba 401"
ok "GET $BASE/api/obras sin token -> 401"

# 4. Entrar. Esto es lo que espera al seed: los usuarios los crea el, asi que
#    hasta que no termina no hay con quien iniciar sesion.
paso "4/6  se puede iniciar sesion con el usuario del seed"
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

# 5. Un endpoint de negocio de verdad, con datos de verdad. Que devuelva 200 no
#    basta: el catalogo vacio tambien es un 200, y es exactamente el sintoma de
#    "levanto pero nadie lo sembro", que es lo que este perfil existe para
#    evitar.
paso "5/6  el catalogo devuelve las obras sembradas"
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

# 6. Lo que HTTP no puede ver. `worker` y `scheduler` no publican puerto: si
#    uno de los dos esta reiniciandose en bucle, todo lo de arriba sigue en
#    verde y el sistema esta roto igual.
paso "6/6  todos los servicios siguen en pie"
if [ "${SMOKE_SIN_COMPOSE:-0}" = "1" ]; then
  ok "omitido (SMOKE_SIN_COMPOSE=1)"
elif ! command -v "${COMPOSE[0]}" >/dev/null 2>&1; then
  ok "omitido: '${COMPOSE[0]}' no esta en el PATH"
else
  corriendo=$("${COMPOSE[@]}" ps --status running --services 2>/dev/null) ||
    fatal "no se pudo consultar '${COMPOSE[*]} ps' (¿se corre desde la raiz del repositorio?)"
  for servicio in "${SERVICIOS[@]}"; do
    grep -qx "$servicio" <<<"$corriendo" ||
      fatal "el servicio '$servicio' no esta corriendo"
  done
  ok "en pie: ${SERVICIOS[*]}"
fi

printf '\n%sLa prueba de humo paso%s: el stack arranca y sirve.\n' "$verde" "$neutro"
