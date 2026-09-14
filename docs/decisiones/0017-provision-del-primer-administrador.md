# 0017 La provision del primer administrador es una orden de la Lambda de migraciones

Estado: Vigente
Fecha: 2026-09-07

## Contexto

Produccion quedo desplegada, migrada y sana, y **cerrada**: sin ningun usuario. Todas las rutas
menos `/health` y `/ready` piden sesion, y una sesion sale de una fila de `usuarios`. No habia
forma de crear la cuenta que crearia las demas.

No es un descuido, es la suma de tres decisiones correctas:

- Ninguna migracion inserta usuarios. Un credencial no es esquema: viajaria en el historial de
  `goose`, se aplicaria en todos los entornos y no se puede rotar con un `up`.
- `cmd/seed` no viaja en la imagen del servicio (ADR 0016 y la revision de #78), porque su
  `SEED_RESET` borra 21 tablas y su unica guarda mira la bitacora, no la procedencia de los datos.
- La base no tiene endpoint publico. La VPC no tiene IGW ni NAT -- solo un endpoint gateway de S3
  -- y el grupo de seguridad de la base solo acepta ingreso desde el de las Lambdas.

Las tres juntas dejan una sola conclusion: la cuenta inicial tiene que crearla algo que ya viva
**dentro** de la VPC.

## Alternativas descartadas

**Abrir la base al exterior** (`publicly_accessible`, o un IGW y una regla de grupo de seguridad).
Descartada: no hay IGW, asi que ni siquiera enrutaria sin cambiar la topologia de red de
produccion, y expone la base para una operacion que se hace una vez.

**Un bastion con SSM Session Manager.** Descartada para esto: sin NAT y sin endpoints de interfaz
para `ssm`, `ssmmessages` y `ec2messages`, el agente no alcanza el servicio. Serian tres endpoints
nuevos con coste mensual permanente, mas una instancia, para una operacion de un solo uso.

**Una Lambda propia (`cmd/lambda-bootstrap`) con su modulo de Terraform.** Es la forma mas limpia y
la que se defiende mejor en revision: la funcion de migraciones conserva su contrato. Descartada
por coste/beneficio: infraestructura nueva que hay que mantener para siempre por una operacion que
se corre una vez en la vida de una instalacion.

**Una ruta HTTP de registro inicial.** Descartada: una superficie publica que acuna la identidad
privilegiada es un blanco permanente, a cambio de ahorrar una invocacion.

## Decision

La orden `primer-administrador` de `cmd/lambda-migrate`, **fuera** de `ordenesPermitidas` para que
no pueda llegar a `goose`.

Reusa la unica funcion que ya vive en las subredes privadas con `DATABASE_URL`. No anade
infraestructura: ni modulo, ni funcion, ni variable de entorno.

### Propiedades que la hacen segura

**Una sola vez, y lo impone la base.** El `INSERT ... WHERE NOT EXISTS (SELECT 1 FROM usuarios)` va
dentro de una transaccion explicita que antes toma `pg_advisory_xact_lock`. El lock no es opcional:
bajo READ COMMITTED -- el nivel por defecto -- la subconsulta del `NOT EXISTS` no ve las filas
insertadas y aun no confirmadas por otra transaccion, asi que dos invocaciones simultaneas contra
una tabla vacia ven las dos una tabla vacia y las dos insertan. Medido: cuatro conexiones con
barrera, cuatro ganadoras y cuatro filas antes del lock.

Ni la clave primaria ni el `UNIQUE` del email sirven de guarda: una segunda cuenta llega con otro
id y otro correo. Y la condicion mira la tabla ENTERA, no el rol: una instalacion con titulares
cargados no esta vacia.

**La clave no viaja en claro.** Llega ya hasheada, calculada por el operador con
`cmd/hash-clave`, que usa el mismo `cripto.Bcrypt` que despues verifica el login -- mismo algoritmo
y mismo coste por construccion. La clave no entra en el evento, no queda en el registro de la
plataforma y el proceso que provisiona no la ve.

**El hash se comprueba por forma, no por longitud.** `Hasher.EsHash` lo contesta el adaptador,
usando `bcrypt.Cost`. La primera version solo miraba `len(hash) >= 20` y por ahi pasaba una clave
en claro de 20 caracteres o mas: se guardaba tal cual, el login fallaba despues con la clave
correcta, y **no habia arreglo** -- esta operacion se niega a correr dos veces y ninguna ruta HTTP
crea usuarios ni resetea claves. El nucleo no aprende que es bcrypt; se lo pregunta al puerto.

**Un reintento es inocuo.** La segunda invocacion responde `ya provisionada` sin error. Es de un
solo uso y ya se hizo; hacerla fallar convertiria el reintento de un operador que no sabe si la
primera llego -- o el reintento automatico de una invocacion asincrona -- en una alarma.

**Los errores no llevan el email.** Se registran y ademas se devuelven desde la Lambda, asi que la
plataforma los guarda una segunda vez por su cuenta. Nombran el campo, nunca su valor.

## Consecuencias

**La deuda, por escrito:** `cmd/lambda-migrate` ya no es solo goose. Si aparece una segunda
operacion de este tipo, las dos deben salir a su propia funcion; una tercera orden ajena a las
migraciones es la senal de que este ADR hay que sustituirlo.

**El radio de la funcion de migraciones crece.** Quien tenga `lambda:InvokeFunction` sobre
`intela-migrate` puede reclamar el primer administrador antes que el operador. Mitigado por IAM y
porque la ventana se cierra en el primer uso, pero es real y queda dicho en el runbook.

**Lo que esto NO resuelve, y es lo siguiente.** Crea UNA cuenta y despues se cierra. No hay forma
de crear los otros cuatro roles del reglamento, y un titular aprobado por la afiliacion (#50) sigue
sin credenciales con las que entrar a ver sus ingresos -- de lo que #42 depende en silencio. Hacen
falta dos issues: gestion de usuarios del personal, y credenciales del titular al aprobar su
afiliacion.

Procedimiento: [`docs/runbooks/primer-administrador.md`](../runbooks/primer-administrador.md).
