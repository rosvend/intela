#!/usr/bin/env bash
#
# Prueba de humo del arranque. Contesta la pregunta que las etapas de Docker de
# CI no contestan: la imagen construye, pero >>arranca y sirve<<?
#
#   docker compose --profile demo up -d --build
#   deploy/smoke.sh
#
# Recorre la misma ruta que recorre una persona en la demo, y por la puerta por
# la que entra una persona: nginx en el puerto de entrada, no la API en el 8080.
# Un proxy mal configurado -la barra final de `proxy_pass`, el `upstream` que
# apunta a un servicio caido- es invisible si se golpea la API directamente, y
# es de lo mas facil de romper que hay aqui.
#
#   1. GET  /health              el proceso vive, sin tocar la base
#   2. GET  /ready               el proceso vive Y Postgres responde
#   3. GET  /                    nginx sirve el tablero, no su propio 404
#   4. POST /api/auth/session    el seed sembro los usuarios, y el prefijo /api/
#                                se recorta bien de camino a la API
#   5. GET  /api/obras           con el token: hay catalogo, y son las 4 del seed
#
# Cada paso se reintenta hasta SMOKE_TIMEOUT. Los pasos 4 y 5 lo necesitan de
# verdad: el seed corre EN PARALELO con la API -no hay `depends_on: seed`- asi
# que al terminar `up -d` es normal que las credenciales todavia no existan. Lo
# que no es normal es que sigan sin existir un minuto despues, y eso es lo que
# separa "todavia no" de "roto".
#
# La cuenta de obras se comprueba de verdad, contra SMOKE_OBRAS. Un `200 []` es
# el falso verde clasico de esta prueba: el stack responde, la ruta existe, la
# autorizacion pasa... y no hay datos. Un smoke test que solo mira el codigo de
# estado lo da por bueno.
#
# Salida 0 = todo verde. Cualquier otra cosa = fallo, con el motivo y el ultimo
# cuerpo recibido.

set -euo pipefail

# --- Configuracion ----------------------------------------------------------

# Puerto por defecto = el mismo que docker-compose.yml publica por defecto. Si
# se movio uno, se mueve el otro: `INTELA_PUERTO_HTTP=8088` sirve para ambos.
BASE_URL="${BASE_URL:-http://localhost:${INTELA_PUERTO_HTTP:-80}}"
SMOKE_EMAIL="${SMOKE_EMAIL:-admin@redes.co}"
# Tiene que ser el rol `administrador`: GET /obras exige ese rol y con cualquier
# otro la prueba fallaria con 403 sin que nada este roto.
SMOKE_CLAVE="${SMOKE_CLAVE:-${SEED_CLAVE_ADMIN:-admin-local}}"
SMOKE_OBRAS="${SMOKE_OBRAS:-4}"
SMOKE_TIMEOUT="${SMOKE_TIMEOUT:-180}"
SMOKE_INTERVALO="${SMOKE_INTERVALO:-2}"

# Estado de la ultima peticion. Global porque en bash una funcion solo devuelve
# un entero, y aqui hacen falta el codigo y el cuerpo a la vez.
CODIGO=""
CUERPO=""
MOTIVO=""

# --- Logica de decision -----------------------------------------------------
#
# Funciones puras: reciben texto, devuelven 0 o 1, no tocan la red. Es lo que
# `deploy/smoke_test.sh` prueba unitariamente. Estan separadas del transporte a
# proposito -es aqui donde se decide verde o rojo, y una decision que solo se
# puede ejercitar levantando Postgres no se ejercita nunca.

# codigo_es <obtenido> <esperado>
codigo_es() { [ "${1-}" = "${2-}" ]; }

# estado_es <cuerpo> <estado>  -- {"estado":"ok"} y {"estado":"listo"}
estado_es() {
  local visto
  visto=$(printf '%s' "${1-}" | jq -r '
    if type == "object" and (.estado | type) == "string" then .estado else empty end
  ' 2>/dev/null) || return 1
  [ "$visto" = "${2-}" ]
}

# es_el_tablero <html>
#
# Busca el <title> del index de la SPA. No vale comprobar solo que la respuesta
# no este vacia: el 404 de nginx tambien es HTML no vacio y devuelto con 200 por
# el fallback de SPA mal puesto, que es justo el fallo que hay que cazar.
es_el_tablero() { printf '%s' "${1-}" | grep -q '<title>Intela'; }

# token_de <cuerpo>  -- imprime el token; falla si no lo hay o esta vacio
token_de() {
  local t
  t=$(printf '%s' "${1-}" | jq -r '
    if type == "object" and (.token | type) == "string" then .token else empty end
  ' 2>/dev/null) || return 1
  [ -n "$t" ] || return 1
  printf '%s' "$t"
}

# contar_obras <cuerpo>  -- imprime cuantas; falla si la respuesta no es una lista
#
# El "falla si no es una lista" importa: un {"error":...} tiene longitud 1 para
# `jq length`, asi que contarlo sin mirar el tipo convierte un error en un exito.
contar_obras() {
  local n
  n=$(printf '%s' "${1-}" | jq -r '
    if type == "array" then length else empty end
  ' 2>/dev/null) || return 1
  [ -n "$n" ] || return 1
  printf '%s' "$n"
}

# cuenta_coincide <cuerpo> <esperadas>
cuenta_coincide() {
  local n
  n=$(contar_obras "${1-}") || return 1
  [ "$n" = "${2-}" ]
}

# --- Transporte -------------------------------------------------------------

# pedir <metodo> <ruta> [cuerpo-json] [cabecera]
pedir() {
  local metodo=$1 ruta=$2 datos=${3-} cabecera=${4-}
  local tmp
  tmp=$(mktemp)
  local -a args=(--silent --show-error --max-time 10 -X "$metodo"
    -o "$tmp" -w '%{http_code}')
  if [ -n "$datos" ]; then
    args+=(-H 'Content-Type: application/json' --data "$datos")
  fi
  if [ -n "$cabecera" ]; then
    args+=(-H "$cabecera")
  fi

  CODIGO=$(curl "${args[@]}" "${BASE_URL}${ruta}" 2>/dev/null) || {
    CUERPO=""
    CODIGO=""
    rm -f "$tmp"
    return 1
  }
  CUERPO=$(cat "$tmp")
  rm -f "$tmp"
}

# recorte <texto> -- para que un HTML entero no inunde el log del fallo
recorte() { printf '%s' "${1-}" | tr '\n' ' ' | cut -c1-200; }

# --- Comprobaciones ---------------------------------------------------------

comprobar_health() {
  pedir GET /health || { MOTIVO="sin respuesta: nadie escucha en $BASE_URL"; return 1; }
  codigo_es "$CODIGO" 200 || { MOTIVO="HTTP $CODIGO"; return 1; }
  estado_es "$CUERPO" ok || { MOTIVO="cuerpo inesperado: $(recorte "$CUERPO")"; return 1; }
}

comprobar_ready() {
  pedir GET /ready || { MOTIVO="sin respuesta"; return 1; }
  # El 503 aqui es el caso interesante: el proceso vive y la base no. Se nombra
  # aparte para no leerlo como "la API esta caida", que es lo contrario.
  if ! codigo_es "$CODIGO" 200; then
    MOTIVO="HTTP $CODIGO ($(recorte "$CUERPO"))"
    if [ "$CODIGO" = "503" ]; then
      MOTIVO="$MOTIVO - la API responde pero Postgres no"
    fi
    return 1
  fi
  estado_es "$CUERPO" listo || { MOTIVO="cuerpo inesperado: $(recorte "$CUERPO")"; return 1; }
}

comprobar_tablero() {
  pedir GET / || { MOTIVO="sin respuesta"; return 1; }
  codigo_es "$CODIGO" 200 || { MOTIVO="HTTP $CODIGO - el servicio web o su upstream"; return 1; }
  es_el_tablero "$CUERPO" || {
    MOTIVO="200 pero no es el tablero: $(recorte "$CUERPO")"
    return 1
  }
}

TOKEN=""
comprobar_sesion() {
  local carga
  carga=$(jq -nc --arg e "$SMOKE_EMAIL" --arg c "$SMOKE_CLAVE" '{email:$e, clave:$c}')
  pedir POST /api/auth/session "$carga" || { MOTIVO="sin respuesta"; return 1; }
  if ! codigo_es "$CODIGO" 200; then
    MOTIVO="HTTP $CODIGO ($(recorte "$CUERPO"))"
    case "$CODIGO" in
      401) MOTIVO="$MOTIVO - usuarios sin sembrar todavia, o clave distinta" ;;
      404) MOTIVO="$MOTIVO - nginx no esta recortando el prefijo /api/" ;;
    esac
    return 1
  fi
  TOKEN=$(token_de "$CUERPO") || { MOTIVO="200 sin token: $(recorte "$CUERPO")"; return 1; }
}

comprobar_obras() {
  pedir GET /api/obras "" "Authorization: Bearer $TOKEN" || { MOTIVO="sin respuesta"; return 1; }
  if ! codigo_es "$CODIGO" 200; then
    MOTIVO="HTTP $CODIGO ($(recorte "$CUERPO"))"
    if [ "$CODIGO" = "403" ]; then
      MOTIVO="$MOTIVO - $SMOKE_EMAIL no tiene el rol administrador"
    fi
    return 1
  fi
  if ! cuenta_coincide "$CUERPO" "$SMOKE_OBRAS"; then
    local vistas
    vistas=$(contar_obras "$CUERPO") || vistas="(la respuesta no es una lista)"
    MOTIVO="se esperaban $SMOKE_OBRAS obras y llegaron $vistas - falta el seed? (--profile demo)"
    return 1
  fi
}

# --- Ejecucion --------------------------------------------------------------

paso() { printf '  \033[32mOK\033[0m   %s\n' "$1"; }

fallo() {
  printf '\n  \033[31mFALLO\033[0m  %s\n' "$1" >&2
  printf '\n         Que mirar: docker compose --profile demo logs --tail=100\n' >&2
  exit 1
}

# reintentar <descripcion> <comprobacion>
#
# Un fallo tiene que distinguirse de un "todavia no". Reintentar hasta el tope y
# entonces gritar es lo que hace esa distincion; abortar al primer intento
# convertiria cada arranque frio en rojo, y no reintentar nunca es como se acaba
# metiendo un `sleep 30` que unas veces sobra y otras no llega.
reintentar() {
  local desc=$1 comprobacion=$2
  local fin=$(( $(date +%s) + SMOKE_TIMEOUT ))
  local intentos=0
  MOTIVO="sin intentar"

  while :; do
    intentos=$((intentos + 1))
    if "$comprobacion"; then
      if [ "$intentos" -gt 1 ]; then
        paso "$desc  (tras $intentos intentos)"
      else
        paso "$desc"
      fi
      return 0
    fi
    if [ "$(date +%s)" -ge "$fin" ]; then
      fallo "$desc
         Motivo:   $MOTIVO
         Intentos: $intentos en ${SMOKE_TIMEOUT}s"
    fi
    sleep "$SMOKE_INTERVALO"
  done
}

requisitos() {
  local falta=""
  command -v curl >/dev/null || falta="$falta curl"
  command -v jq >/dev/null || falta="$falta jq"
  [ -z "$falta" ] || fallo "faltan herramientas:$falta"
}

main() {
  requisitos
  printf '\nPrueba de humo contra %s\n\n' "$BASE_URL"

  reintentar "GET  /health            200 {\"estado\":\"ok\"}"        comprobar_health
  reintentar "GET  /ready             200 {\"estado\":\"listo\"}"     comprobar_ready
  reintentar "GET  /                  200 con el tablero"           comprobar_tablero
  reintentar "POST /api/auth/session  200 con token ($SMOKE_EMAIL)" comprobar_sesion
  reintentar "GET  /api/obras         200 con $SMOKE_OBRAS obras"   comprobar_obras

  printf '\n\033[32mPrueba de humo verde.\033[0m El stack arranca y sirve.\n\n'
}

# El guard es lo que deja que `deploy/smoke_test.sh` haga `source` de este
# fichero para probar las funciones de arriba sin disparar ninguna peticion.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
  main "$@"
fi
