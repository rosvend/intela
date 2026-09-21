#!/usr/bin/env bash
#
# Prueba de la guarda de servicios de una sola pasada de deploy/smoke.sh.
#
# Por que existe: la prueba de humo de verdad tarda ocho minutos y necesita un
# stack levantado, asi que el caso que importa -el `seed` que falla sobre una
# base QUE YA TENIA DATOS- no se puede ejercer en cada PR: hace falta sembrar,
# romper el dataset a mano y volver a sembrar. Esta prueba ejerce la misma
# guarda sin levantar nada, sustituyendo `docker compose` por un doble que
# imprime el `ps --all --format json` del caso que se quiere probar.
#
# Lo que fija es justo lo que el verde falso necesitaba para colarse: que
# smoke.sh mire el CODIGO DE SALIDA del contenedor y no los efectos que dejo
# en la base. Si alguien quita la guarda, smoke.sh deja de fallar en la
# comprobacion 1/7 y pasa a fallar mas tarde por HTTP, con otro mensaje: los
# casos de abajo comparan el mensaje, no solo el codigo de salida, asi que la
# mutacion se ve.
#
# Uso: deploy/smoke_guardia_test.sh
# Sale 0 si todos los casos pasan, 1 si alguno falla.

set -Eeuo pipefail

AQUI=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
SMOKE="$AQUI/smoke.sh"

[ -x "$SMOKE" ] || {
  printf 'no encuentro %s o no es ejecutable\n' "$SMOKE" >&2
  exit 1
}

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fallos=0

# doble_compose escribe un ejecutable que se comporta como `docker compose`
# para lo unico que la guarda le pide: `ps --all --format json`. El resto de
# subordenes (las que usa fatal() para volcar diagnostico) responden vacio y
# con exito, para que el doble no sea quien haga fallar la prueba.
doble_compose() {
  local ruta="$1" json="$2"
  cat >"$ruta" <<DOBLE
#!/usr/bin/env bash
case "\$*" in
  *"--format json"*)
    cat <<'JSON'
$json
JSON
    ;;
  *) exit 0 ;;
esac
DOBLE
  chmod +x "$ruta"
}

# caso corre smoke.sh contra un doble y comprueba el codigo de salida y que la
# salida contenga el fragmento esperado.
#
# SMOKE_ESPERA=1 acota el caso en que la guarda NO salta: sin eso, un fallo de
# esta prueba tardaria SMOKE_ESPERA por defecto (180 s) en cada comprobacion
# HTTP contra un puerto donde no hay nadie.
caso() {
  local nombre="$1" json="$2" codigo_esperado="$3" fragmento="$4"
  local doble="$TMP/compose-$RANDOM" salida codigo

  doble_compose "$doble" "$json"

  set +e
  salida=$(
    SMOKE_COMPOSE="$doble" \
      SMOKE_ESPERA=1 \
      SMOKE_BASE_URL="http://127.0.0.1:1" \
      "$SMOKE" 2>&1
  )
  codigo=$?
  set -e

  if [ "$codigo" != "$codigo_esperado" ]; then
    printf 'FALLO  %s: salio %s, se esperaba %s\n' "$nombre" "$codigo" "$codigo_esperado" >&2
    printf '%s\n' "$salida" | sed 's/^/       | /' >&2
    fallos=$((fallos + 1))
    return
  fi

  if ! grep -qF -- "$fragmento" <<<"$salida"; then
    printf 'FALLO  %s: la salida no menciona %s\n' "$nombre" "$fragmento" >&2
    printf '%s\n' "$salida" | sed 's/^/       | /' >&2
    fallos=$((fallos + 1))
    return
  fi

  printf '   ok  %s\n' "$nombre"
}

printf 'Guarda de servicios de una sola pasada (deploy/smoke.sh)\n\n'

# 1. EL CASO DEL BLOQUEANTE. El seed fallo, pero la base ya tenia datos de una
#    corrida anterior, asi que login y /api/obras habrian pasado. La guarda
#    tiene que cortar antes, nombrando al servicio.
caso "seed fallido -> rojo nombrando 'seed'" \
  '[{"Service":"migrate","State":"exited","ExitCode":0},
    {"Service":"seed","State":"exited","ExitCode":1},
    {"Service":"api","State":"running","ExitCode":0}]' \
  1 "'seed' termino con codigo 1"

# 2. Lo mismo para migrate.
caso "migrate fallido -> rojo nombrando 'migrate'" \
  '[{"Service":"migrate","State":"exited","ExitCode":2},
    {"Service":"api","State":"running","ExitCode":0}]' \
  1 "'migrate' termino con codigo 2"

# 3. La forma de `ps --format json` no es estable entre versiones de Compose:
#    unas devuelven un array y otras una linea por contenedor. La guarda tiene
#    que ver el fallo en las dos, o se rompe segun en que runner corra.
caso "NDJSON (no array) -> rojo igual" \
  '{"Service":"migrate","State":"exited","ExitCode":0}
{"Service":"seed","State":"exited","ExitCode":1}' \
  1 "'seed' termino con codigo 1"

# 4. Un contenedor que no ha terminado lleva ExitCode 0. Sin mirar el estado,
#    "no ha terminado" se leeria como "termino bien".
caso "seed sin terminar -> rojo" \
  '[{"Service":"migrate","State":"exited","ExitCode":0},
    {"Service":"seed","State":"running","ExitCode":0}]' \
  1 "'seed' no termino"

# 5. Si SMOKE_COMPOSE no mira el proyecto que se levanto, no hay nada que
#    comprobar y decirlo es mejor que dar verde.
caso "sin migrate en el proyecto -> rojo" \
  '[{"Service":"api","State":"running","ExitCode":0}]' \
  1 "no aparece el contenedor de 'migrate'"

# 6. `up` PELADO, sin perfil: no hay contenedor de seed y eso es correcto -el
#    `required: false` de docker-compose.yml es deliberado-. La guarda no
#    puede exigirlo. Aqui smoke.sh tiene que PASAR la comprobacion 1/7 y morir
#    despues en la primera peticion HTTP, que es otro mensaje distinto.
caso "up sin perfil (sin seed) -> la guarda no se queja" \
  '[{"Service":"migrate","State":"exited","ExitCode":0},
    {"Service":"api","State":"running","ExitCode":0}]' \
  1 "GET http://127.0.0.1:1/ready"

# 7. Todo en orden: la guarda pasa y el fallo vuelve a ser el de HTTP.
caso "migrate y seed en 0 -> la guarda no se queja" \
  '[{"Service":"migrate","State":"exited","ExitCode":0},
    {"Service":"seed","State":"exited","ExitCode":0},
    {"Service":"api","State":"running","ExitCode":0}]' \
  1 "GET http://127.0.0.1:1/ready"

printf '\n'
if [ "$fallos" != "0" ]; then
  printf '%s caso(s) fallaron\n' "$fallos" >&2
  exit 1
fi
printf 'La guarda de una sola pasada se comporta en los %s casos.\n' 7
