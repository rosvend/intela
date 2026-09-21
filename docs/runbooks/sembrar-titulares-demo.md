# Runbook: sembrar el padron demo en produccion

Carga Ana, Beto y Carla en `titulares` para poder declarar splits en una
instalacion que **no** corre `cmd/seed` (la imagen de produccion no lo
incluye).

Misma razon que [primer-administrador](primer-administrador.md): la base esta
en subred privada y solo la alcanza la Lambda de migraciones.

## Cuando se usa

Cuando el catalogo ya tiene obras (via `POST /api/obras`) y
`estado_declaracion` queda en `incompleta` / `suma_porcentajes: 0` porque el
padron esta vacio: `POST /obras/{id}/declaracion` responde 400
("uno de los titulares indicados no existe").

**No siembra obras ni declaraciones.** Solo el padron. Los splits se dan de
alta despues por la API.

## Antes de empezar

- El codigo con la orden `sembrar-titulares-demo` ya desplegado en
  `intela-migrate` (push a `main` + Deploy, o `make aplicar` local).
- Credenciales AWS del entorno y permiso `lambda:InvokeFunction` sobre
  `intela-migrate`.

## Pasos

### 1. Confirmar que el padron esta vacio (opcional)

```bash
TOKEN=$(curl -s -X POST https://<url>/api/auth/session \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@redes.co","clave":"<clave>"}' \
  | jq -r .token)

curl -s https://<url>/api/titulares -H "Authorization: Bearer $TOKEN"
# []
```

### 2. Invocar la orden

```bash
aws lambda invoke \
  --function-name intela-migrate \
  --profile <perfil> \
  --payload '{"orden":"sembrar-titulares-demo"}' \
  --cli-binary-format raw-in-base64-out \
  /dev/stdout
```

Respuesta esperada:

```json
{"orden":"sembrar-titulares-demo","estado":"creados"}
```

Un reintento responde `{"estado":"ya sembrados"}` y no duplica filas.

### 3. Declarar splits por la API

```bash
curl -s -X POST https://<url>/api/obras/<obra-id>/declaracion \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '[
    {"titular_id":"tit-ana","ipi":"IPI-00000001","porcentaje":60},
    {"titular_id":"tit-beto","ipi":"IPI-00000002","porcentaje":40}
  ]'
```

Titulares sembrados:

| id | nombre | IPI |
| --- | --- | --- |
| `tit-ana` | Ana Escritora | `IPI-00000001` |
| `tit-beto` | Beto Libretista | `IPI-00000002` |
| `tit-carla` | Carla Guionista | `IPI-00000003` |

## Respuestas posibles

| Respuesta | Que significa | Que hacer |
| --- | --- | --- |
| `{"estado":"creados"}` | Habia huecos; se insertaron. | Declarar por la API. |
| `{"estado":"ya sembrados"}` | Los tres ids ya estaban. | Nada, o declarar. |
| `orden "…" no permitida` | La Lambda desplegada es anterior a esta orden. | Desplegar `main` y reintentar. |

## Lo que este runbook NO cubre

El dataset completo del seed (obras, reportes, bolsas, parametros sinteticos,
usuarios de demo). Eso sigue siendo solo local:
`docker compose --profile demo up`.
