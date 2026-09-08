#!/usr/bin/env bash
#
# Prueba unitaria de la logica de decision de `smoke.sh`. No levanta nada, no
# abre un socket y corre en menos de un segundo:
#
#   deploy/smoke_test.sh
#
# Existe porque la unica parte de un smoke test que se puede equivocar en
# silencio es esta. Si el transporte se rompe, la prueba se cae y se ve. Si el
# que se rompe es el criterio -contar mal, aceptar un cuerpo que no toca, dar por
# bueno un 200 vacio- la prueba sigue verde y deja de significar nada. Una
# comprobacion que no puede fallar no es una comprobacion.
#
# De ahi que el grueso de los casos de abajo sean NEGATIVOS: cada uno es una
# forma real en la que este stack ha dado o puede dar un falso verde.

set -uo pipefail

DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# `smoke.sh` solo ejecuta `main` cuando se invoca directamente, asi que hacerle
# `source` trae las funciones sin disparar una sola peticion.
# shellcheck source-path=SCRIPTDIR
# shellcheck source=smoke.sh
source "$DIR/smoke.sh"

# `smoke.sh` trae `set -e`; aqui estorba, porque el arnes espera fallos.
set +e

PASADAS=0
FALLIDAS=0

reportar() {
  local veredicto=$1 desc=$2
  if [ "$veredicto" = ok ]; then
    PASADAS=$((PASADAS + 1))
    printf '  \033[32mok\033[0m   %s\n' "$desc"
  else
    FALLIDAS=$((FALLIDAS + 1))
    printf '  \033[31mFALLA\033[0m %s\n' "$desc" >&2
  fi
}

# acepta <descripcion> <funcion> [args...]
acepta() {
  local desc=$1
  shift
  if "$@"; then reportar ok "$desc"; else reportar no "$desc"; fi
}

# rechaza <descripcion> <funcion> [args...]
rechaza() {
  local desc=$1
  shift
  if "$@"; then reportar no "$desc"; else reportar ok "$desc"; fi
}

# devuelve <descripcion> <esperado> <funcion> [args...]
devuelve() {
  local desc=$1 esperado=$2
  shift 2
  local obtenido
  obtenido=$("$@")
  if [ "$obtenido" = "$esperado" ]; then
    reportar ok "$desc"
  else
    reportar no "$desc (esperado '$esperado', obtenido '$obtenido')"
  fi
}

printf '\nLogica de decision de deploy/smoke.sh\n\n'

# --- codigo_es --------------------------------------------------------------

acepta  "codigo_es: 200 es 200"                 codigo_es 200 200
rechaza "codigo_es: 503 no es 200"              codigo_es 503 200
rechaza "codigo_es: cadena vacia no es 200"     codigo_es "" 200

# --- estado_es --------------------------------------------------------------

acepta  "estado_es: /health sano"                          estado_es '{"estado":"ok"}' ok
acepta  "estado_es: /ready listo"                          estado_es '{"estado":"listo"}' listo
rechaza "estado_es: /ready degradado no es listo"          estado_es '{"estado":"degradado"}' listo
rechaza "estado_es: cuerpo vacio"                          estado_es '' ok
rechaza "estado_es: HTML no es JSON (nginx contestando)"   estado_es '<html>502 Bad Gateway</html>' ok
rechaza "estado_es: JSON sin el campo"                     estado_es '{"version":"1"}' ok
rechaza "estado_es: JSON valido que no es objeto"          estado_es '["ok"]' ok

# --- es_el_tablero ----------------------------------------------------------
#
# El caso que justifica la funcion: nginx devolviendo 200 con algo que NO es la
# SPA. Mirar solo el codigo de estado lo da por bueno.

acepta  "es_el_tablero: el index de la SPA" \
  es_el_tablero '<!doctype html><html><head><title>Intela · REDES SGC</title></head><body><div id="root"></div></body></html>'
rechaza "es_el_tablero: la pagina de bienvenida de nginx" \
  es_el_tablero '<html><head><title>Welcome to nginx!</title></head></html>'
rechaza "es_el_tablero: el 404 de nginx servido con 200" \
  es_el_tablero '<html><head><title>404 Not Found</title></head></html>'
rechaza "es_el_tablero: respuesta vacia"      es_el_tablero ''
rechaza "es_el_tablero: JSON de la API"       es_el_tablero '{"estado":"ok"}'

# --- token_de ---------------------------------------------------------------

devuelve "token_de: extrae el token" "abc.def.ghi" \
  token_de '{"token":"abc.def.ghi","expira":"2026-01-01T00:00:00Z"}'
rechaza "token_de: 200 con token vacio"                token_de '{"token":""}'
rechaza "token_de: 200 sin campo token"                token_de '{"usuario":{"rol":"administrador"}}'
rechaza "token_de: token que no es cadena"             token_de '{"token":null}'
rechaza "token_de: cuerpo de error"                    token_de '{"error":"credenciales invalidas"}'
rechaza "token_de: cuerpo que no es JSON"              token_de 'Unauthorized'

# --- contar_obras -----------------------------------------------------------
#
# `jq length` sobre un objeto devuelve el numero de claves, asi que contar sin
# mirar el tipo convierte {"error":...} en "1 obra" y la prueba pasa. De ahi el
# `if type == "array"`.

devuelve "contar_obras: las cuatro del seed" 4 contar_obras '[{"id":"a"},{"id":"b"},{"id":"c"},{"id":"d"}]'
devuelve "contar_obras: catalogo vacio es 0, no un fallo" 0 contar_obras '[]'
rechaza  "contar_obras: un objeto de error no se cuenta" contar_obras '{"error":"no autorizado"}'
rechaza  "contar_obras: null no se cuenta"               contar_obras 'null'
rechaza  "contar_obras: HTML no se cuenta"               contar_obras '<html>502</html>'
rechaza  "contar_obras: respuesta vacia"                 contar_obras ''

# --- cuenta_coincide --------------------------------------------------------
#
# El veredicto final de la prueba. El primer rechazo es el falso verde que este
# smoke test existe para cazar: stack arriba, ruta viva, autorizacion en orden y
# base sin sembrar.

acepta  "cuenta_coincide: 4 obras cuando se esperan 4" \
  cuenta_coincide '[{"id":"a"},{"id":"b"},{"id":"c"},{"id":"d"}]' 4
rechaza "cuenta_coincide: 200 [] cuando se esperan 4 (sin sembrar)" \
  cuenta_coincide '[]' 4
rechaza "cuenta_coincide: 3 obras cuando se esperan 4 (seed a medias)" \
  cuenta_coincide '[{"id":"a"},{"id":"b"},{"id":"c"}]' 4
rechaza "cuenta_coincide: 5 obras cuando se esperan 4 (sembrado dos veces)" \
  cuenta_coincide '[{},{},{},{},{}]' 4
rechaza "cuenta_coincide: un error con 4 claves no son 4 obras" \
  cuenta_coincide '{"a":1,"b":2,"c":3,"d":4}' 4

# --- recorte ----------------------------------------------------------------
#
# Solo tiene que no romperse y no vomitar un HTML entero en el log.

devuelve "recorte: deja el texto corto tal cual" "HTTP 500" recorte "HTTP 500"
devuelve "recorte: aplasta los saltos de linea" "a b c" recorte "$(printf 'a\nb\nc')"

# --- Veredicto --------------------------------------------------------------

printf '\n  %d pasadas, %d fallidas\n\n' "$PASADAS" "$FALLIDAS"
[ "$FALLIDAS" -eq 0 ] || exit 1
