# Runbook: sembrar el dataset demo en produccion

Carga el dataset sintetico completo (`cmd/seed` /
`internal/infraestructura/semilla`) en una instalacion cuya base esta en
subred privada: titulares, obras, declaraciones, bolsas, reportes, usos
identificados y parametros.

Misma razon que [primer-administrador](primer-administrador.md): solo la
Lambda de migraciones alcanza Postgres.

## Cuando se usa

Cuando Amplify / la API estan arriba pero el catalogo esta vacio o sin
declaraciones, y hace falta el mismo juego que `docker compose --profile demo`
deja en local.

## Antes de empezar

- El codigo con la orden `sembrar-dataset` ya desplegado en `intela-migrate`
  (merge a `main` + Deploy, o `make aplicar`).
- Credenciales AWS y permiso `lambda:InvokeFunction` sobre `intela-migrate`.
- La base **sin obras ajenas al dataset**. Si ya hay obras creadas por la API
  con otros ids, el seed responde error (`semilla a medias` o
  `ErrDatosNoSinteticos` con `reset:true`). En ese caso hace falta una base
  limpia (nuevo entorno / recrear RDS), no un reset a medias.

El admin provisionado (`primer-administrador`) **se conserva**: el seed no
pisa su hash. Las otras cuentas demo se crean con las claves por defecto de
[`ARRANQUE.md`](../ARRANQUE.md).

## Pasos

### 1. Invocar

```bash
aws lambda invoke \
  --function-name intela-migrate \
  --profile <perfil> \
  --payload '{"orden":"sembrar-dataset"}' \
  --cli-binary-format raw-in-base64-out \
  /dev/stdout
```

Respuesta esperada:

```json
{"orden":"sembrar-dataset","estado":"cargado"}
```

Un reintento responde `{"estado":"ya sembrado"}`.

El alias `sembrar-titulares-demo` hace lo mismo (compatibilidad con el nombre
anterior).

### 2. Comprobar

```bash
TOKEN=$(curl -s -X POST https://<url>/api/auth/session \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@redes.co","clave":"<tu-clave-provisionada>"}' \
  | jq -r .token)

curl -s https://<url>/api/obras -H "Authorization: Bearer $TOKEN" | jq .
# 4 obras; Pelicula X y Minuto Comico con suma_porcentajes 100;
# Serie Y con 60 (incompleta a proposito, R-04).
```

## Respuestas posibles

| Respuesta | Que significa | Que hacer |
| --- | --- | --- |
| `{"estado":"cargado"}` | Dataset escrito. | Entrar al tablero. |
| `{"estado":"ya sembrado"}` | Ya estaba completo. | Nada. |
| `semilla a medias (...): pase SEED_RESET=true` | Hay obras/reportes a medias o ajenos. | Base limpia, o `{"orden":"sembrar-dataset","reset":true}` solo si **todo** lo que hay es del dataset. |
| `SEED_RESET rechazado: hay datos que no son del dataset sintetico` | Hay obras/titulares reales. | No uses reset; recrea el entorno. |
| `orden "…" no permitida` | Lambda vieja. | Desplegar `main` y reintentar. |

## Lo que este runbook NO cubre

- Correr el seed en cada `terraform apply` automaticamente (sigue siendo
  invocacion manual, como `primer-administrador`).
- Los bytes crudos de los reportes en S3: en Lambda caen en `/tmp` y no los
  comparte la API. El metadato y los usos en Postgres si quedan.
