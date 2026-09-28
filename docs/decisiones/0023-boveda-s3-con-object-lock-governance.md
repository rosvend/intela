# 0023 La boveda de produccion es S3 con Object Lock GOVERNANCE a diez anos

Fecha: 2026-09-28
Estado: Vigente, **provisional** en el modo de bloqueo (ver "Provisional")

## Contexto

El puerto `AlmacenObjetos` (`Poner`, `Obtener`, `Borrar`) solo tenia `objetos.Disco`. En Lambda el
unico sitio escribible es `/tmp`, que se recicla con el contenedor, asi que `cmd/lambda` dejaba
`Ingesta` y `Admision` sin cablear y `POST /api/reportes` respondia 503 en produccion (#182). El
[ADR 0006](0006-trazabilidad-como-asiento-append-only.md) pide copia cruda inmutable con retencion,
y el [ADR 0014](0014-infraestructura-serverless-en-aws.md) pone los objetos en el bucket de
`infra/modules/storage`, creado ya con Object Lock, versionado, SSE y bloqueo de acceso publico.

Habia dos preguntas abiertas en #182: que modo de Object Lock usar, y que hace `Borrar` frente a un
objeto bloqueado.

## Decision

**`objetos.S3` implementa `AlmacenObjetos` sobre ese bucket, con el mismo contrato que `Disco`**
(una sola bateria de pruebas para los dos, la de S3 contra LocalStack).

- **`Poner`** escribe con `If-None-Match: *`: una clave ocupada da 412 y sale como
  `ErrObjetoYaExiste`, igual que `Disco`, que es lo que `Ingesta.congelarEvidencia` necesita para
  completar una subida a medias. Cada objeto lleva **`GOVERNANCE` hasta dentro de diez anos**
  (R-13, `RD 13.4`) y checksum SHA-256 que S3 valida al recibirlo.
- **`Obtener`** lee la version vigente. Con la escritura condicional cada clave tiene una sola
  version con contenido, asi que la clave que guarda la base basta; no se guarda `VersionId`.
- **`Borrar`** hace un `DeleteObject` **sin** `VersionId`. En un bucket versionado eso solo pone un
  marcador de borrado: la clave deja de leerse y la version retenida **queda como huerfana inerte**
  hasta que vence su retencion. Es la opcion mas simple que es correcta: la compensacion de
  `Admision` (el unico que llama a `Borrar`) sigue siendo idempotente, y ninguna Lambda puede
  destruir una version retenida.
- **Cableado**: `OBJECT_BUCKET` elige S3 (`objetos.Boveda`); sin el, `Disco` en `OBJECT_DIR`, que es
  lo de desarrollo y `docker compose`. `cmd/lambda` **no arranca sin `OBJECT_BUCKET`** en vez de caer
  en `/tmp`. `cmd/lambda-migrate` usa lo mismo para `sembrar-dataset`.
- **IAM**: una politica inline por Lambda (api y migrate), escrita por `modules/storage`:
  `s3:PutObject`, `s3:PutObjectRetention` y `s3:GetObject` sobre `bucket/*`; `s3:DeleteObject` solo
  sobre `bucket/afiliaciones/*`; y `s3:ListBucket` sobre el bucket, sin el cual una clave inexistente
  da 403 en vez de 404 y `Obtener` no podria devolver `ErrNoEncontrado`. **Nunca**
  `s3:DeleteObjectVersion` ni `s3:BypassGovernanceRetention`. Inline y no adjunta porque el rol de
  despliegue solo puede adjuntar las dos politicas de ejecucion de Lambda (`modules/github-oidc`).

## Provisional

El equipo responde como proxy del cliente; estas dos respuestas no son de REDES SGC:

1. **`GOVERNANCE` y no `COMPLIANCE`.** Hoy los datos son sinteticos. `COMPLIANCE` no lo levanta ni
   la cuenta raiz durante diez anos, asi que un objeto de prueba mal subido seria permanente y el
   bucket no se podria vaciar. `GOVERNANCE` ya impide sobrescribir y borrar a todo el que no tenga
   `s3:BypassGovernanceRetention`, y ese permiso no lo tiene ninguna Lambda. Pasar a `COMPLIANCE`
   es cambiar una constante de `objetos/s3.go` cuando entren datos reales, y es una decision aparte.
2. **Diez anos contados desde la escritura**, no desde el cierre del periodo o del pago.

## Alternativas consideradas

**Escribir sin bloqueo y aplicarlo (`PutObjectRetention`) cuando la fila exista.** Permitiria borrar
de verdad en la compensacion, pero abre una ventana en la que la evidencia ya certificada no esta
bloqueada, y pide un segundo paso que el nucleo tendria que recordar llamar. Descartada.

**Bloqueo por defecto del bucket en vez de por objeto.** Mas simple en el codigo, pero bloquearia
tambien lo que se escriba para pruebas manuales o migraciones, y el modo quedaria en Terraform lejos
del adaptador que lo prueba. Descartada por ahora.

**MinIO como emulador de pruebas.** Es lo que nombraba el ADR 0010, pero `minio/minio` ya no se puede
descargar de Docker Hub. LocalStack aplica Object Lock y la escritura condicional, y las pruebas lo
demuestran fallando si se quitan.

## Consecuencias

Un RUT o certificado bancario de una solicitud cuyo `INSERT` perdio queda retenido diez anos sin
fila que lo nombre. Son bytes personales que nadie lee; se asume a sabiendas y se revisa al pasar a
`COMPLIANCE`.

Fuera de esta decision: las subidas de mas de 6 MB, que no caben en una Function URL, van por PUT
prefirmado (#191). Hasta entonces produccion solo acepta reportes de menos de 6 MB.
