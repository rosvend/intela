# 0014 La infraestructura de ejecucion es AWS serverless, descrita en Terraform

Fecha: 2026-09-05
Estado: Vigente
Modifica a: [0003 Monolito modular, no microservicios](0003-monolito-modular.md)

## Contexto

`docs/cd.md` y `.github/workflows/deploy.yml` decian lo mismo desde que existen: la tuberia de
release esta completa de forma y vacia de fondo, porque **no hay ADR de infraestructura de
ejecucion**. Las imagenes se publican de verdad en GHCR en cada merge y nadie las lee. Este ADR es
lo que faltaba.

Lo que acota la decision no es tecnico, es economico y de contexto:

- El despliegue va a la cuenta de AWS de un empleador, que ademas es la **cuenta de gestion** de su
  organizacion. Lo de Intela tiene que quedar separado por etiqueta y por coste de trabajo ajeno que
  vive en la misma cuenta.
- **Menos de $20/mes**, y menos de diez usuarios. No es un sistema de trafico.
- Tiene que poder **mudarse a una cuenta personal sin avisar**. Mudarse debe ser cambiar variables y
  volver a correr `init` con otro backend, no reescribir.
- La `0003` ya midio la carga: un pico anual, no trafico sostenido. Esa medicion sigue siendo buena y
  es justo la que hace absurdo pagar por hora algo que esta parado.

Hay una tension que hay que nombrar antes de decidir, porque quien lea esto la va a ver: **la `0003`
descarto serverless**.

## Decision

**La API y las migraciones corren como funciones Lambda, el tablero en Amplify Hosting, y los datos
en una instancia RDS PostgreSQL; todo descrito en Terraform, con un modulo raiz por cuenta.**

Cinco partes:

1. **Lambda como runtime, no como arquitectura.** `cmd/lambda` es un adaptador primario mas, hermano
   de `cmd/api`. Los dos entregan el mismo `httpapi.(*API).Router()`. `internal/` no cambia ni una
   linea y no aprende que existe Lambda.
2. **Amplify Hosting sirve `web/dist`** y reescribe `/api/<*>` hacia la Function URL, de modo que
   tablero y API comparten origen. Es el mismo truco que hace hoy `deploy/nginx.conf` con
   `proxy_pass http://api/`: la barra final quita el prefijo, porque el router registra las rutas en
   la raiz.
3. **RDS PostgreSQL `db.t4g.micro` en subred privada**, sin endpoint publico, sin NAT Gateway y sin
   VPC interface endpoints. La Lambda la alcanza desde dentro de la VPC.
4. **El orden migrar-antes-de-servir lo impone el grafo de Terraform**, no un paso de shell:
   `module.api` declara `depends_on = [module.migrations]`. Si goose falla, el `apply` falla y la API
   se queda con el codigo anterior.
5. **GitHub Actions entra por OIDC**, con dos roles: uno de solo lectura para el `plan` de un pull
   request y otro para el `apply` de un push a `main`. No se guarda ninguna credencial.

### Que le modifica esto a la `0003`

La `0003` descarto **"serverless por caso de uso"**: una funcion por caso de uso, que habria partido
el dominio en trozos desplegables por separado. Ese descarte sigue vigente y esta decision no lo
toca: sigue habiendo **un solo desplegable** y **un `main` por punto de entrada**, que es literalmente
lo que la `0003` pide. Lambda aqui es donde se ejecuta el binario, no como se parte el sistema.

Lo que la `0003` acerto en senalar y **sigue siendo cierto**: un proceso por lotes no cabe en los
quince minutos de una Lambda. Cuando aterrice el motor de reparto, la corrida **no** va en la Lambda
de la API; va en una tarea puntual de Fargate o equivalente, y eso es un ADR propio. Esta decision
cubre el camino de peticion y la migracion, que es lo que hay hoy.

## Alternativas consideradas

**Aurora Serverless v2 con escalado a cero.** Es la que mas cerca estuvo, y la que mas caro habria
salido. La documentacion de AWS fija la condicion de pausa: *"Aurora begins pausing the instance when
the specified delay period passes with **no connections to the instance**"*. Una Lambda tibia
mantiene abierto el pool de `pgx`, y el recolector de conexiones ociosas de `pgxpool` es una gorrutina
de fondo que **no corre mientras Lambda tiene el entorno congelado**: la conexion sobrevive y el
cluster no pausa nunca. Abrir y cerrar en cada invocacion si funciona, y cuesta menos de lo que
parece —dentro de la VPC un saludo TCP+TLS+auth son 30-80 ms—, pero basta una conexion abierta en
cualquier sitio para pagar `0.5 ACU x 730 h x $0.12 = $43.80/mes` en silencio. Y hay un disparador a
mano: `GET /ready` consulta la base, asi que el dia que alguien le apunte un monitor de
disponibilidad, Aurora deja de dormir y nada avisa. La mitigacion habitual tampoco esta: la
documentacion confirma que **RDS Proxy mantiene una conexion abierta e impide la pausa**. Ahorro
maximo ~$8/mes contra el riesgo de triplicar la factura sin ruido en la cuenta de un empleador.

**Aurora DSQL.** Descartada por incompatible, no por cara. `migrations/00001_init.sql` usa `pg_trgm`,
`btree_gist` con un `EXCLUDE USING gist` sobre `daterange`, `pgcrypto`, dos funciones plpgsql y cinco
triggers. DSQL no soporta nada de eso, y son justamente las invariantes que sostienen la bitacora
append-only de la `0006`.

**DynamoDB.** Serverless de verdad y con capa gratuita generosa, pero implica tirar el esquema entero
y con el las invariantes que el `0006` y el `0008` hacen cumplir en el motor. No es un cambio de
adaptador, es otro sistema.

**Amplify como plataforma completa.** Es lo que sugiere su nombre y no es lo que hace: Amplify Hosting
sirve frontales —React, Vue, Next, Nuxt— y su computo SSR es Node.js. El "fullstack" de Amplify Gen 2
genera un backend AppSync + DynamoDB + Cognito definido en TypeScript. No hay sitio para un binario
Go contra PostgreSQL. Amplify se queda con el papel que si sabe hacer: servir el SPA y hacer de proxy.

**ECS Fargate con las imagenes de GHCR.** Es la traduccion literal del `docker compose` que ya existe,
y por eso era tentadora. Se descarta por precio: una tarea Fargate minima facturada por hora
sostenida se come el presupuesto entero, y con menos de diez usuarios estaria parada casi siempre.
Ademas Fargate no tira de GHCR privado sin un secreto de registro, o sin espejar a ECR.

**Una VM con `docker compose` bajo systemd.** Es lo que asumia el documento de OKR del equipo, y
sigue siendo lo mas parecido al entorno local. Factura por hora este o no encendida, hay que
parchearla, y no tiene despliegue reproducible sin construir precisamente lo que este ADR construye.

**API Gateway delante de la Lambda.** Da etapas, throttling y logs de acceso, y cuesta $1 por millon
de peticiones, que aqui es nada. Se descarta porque no compra nada que Amplify no de ya —el rewrite
resuelve el mismo origen y el prefijo— y anade una segunda superficie de CORS que mantener.

## Consecuencias

Positivas: la factura queda en ~$15/mes, y **RDS es lo unico que factura sin trafico**. Sin NAT
Gateway, sin balanceador, sin VPC interface endpoints. Anadir un runtime entero costo un `main`
nuevo y cero cambios en `internal/`, que es el retorno de la `0002`. Mudar de cuenta es correr
`infra/bootstrap/` alli, escribir otro `backend.hcl` y cambiar dos secretos: `modules/` no se toca.

A cambio: hay un modelo de despliegue mas que aprender, que es exactamente el coste que la `0003`
anticipo. Se paga una vez y queda escrito aqui.

La contrasena de la base **queda en el estado de Terraform**, que vive en un bucket cifrado,
versionado y cerrado al publico. La alternativa —`manage_master_user_password` con Secrets Manager—
obligaria a la Lambda a llamar a Secrets Manager desde dentro de la VPC, y eso pide un interface
endpoint de $7.20/mes. Es un coste asumido a sabiendas, no un descuido.

El rol de despliegue tiene IAM acotado a `role/intela-*` y `policy/intela-*`, pero conserva `ec2:*`,
`rds:*`, `amplify:*` y `logs:*` sobre `*`, porque las llamadas de creacion de VPC no admiten permisos
a nivel de recurso. Lo que contiene el radio ahi es la etiqueta `Project`, el presupuesto que vigila
esa etiqueta, y la guarda de destruccion del pipeline. Estrecharlo mas pedria un esquema ABAC cuyo
modo de fallo es una pila a medio aplicar.

Riesgo asumido: **la reescritura de Amplify tiene que dejar pasar la cabecera `Authorization`**. Si
no lo hiciera, toda la autenticacion se cae, porque `httpapi` lee el token de ahi. Se verifica en el
primer despliegue con un login real, y la mitigacion esta contemplada en el diseno:
`modules/frontend/` recibe el origen como cadena opaca, asi que cambiarlo por S3 + CloudFront con
OAC —que ademas permitiria cerrar la Function URL con `AWS_IAM`— es un cambio local a ese modulo.

`cmd/worker` y `cmd/scheduler` **no se despliegan todavia**. Sus cuerpos son `log.Debug(); return nil`:
desplegarlos seria desplegar nada. Cuando tengan trabajo, la forma es EventBridge Scheduler
invocando una Lambda acotada en tiempo, que es otra llamada a `modules/go-lambda`. Queda escrito
aqui para que no haya que reconstruirlo.

Y una consecuencia que hay que recordar al escribir el endpoint de ingesta: `deploy/nginx.conf`
permite cuerpos de 64 MB porque las parrillas reales son de decenas de MB, y **una Function URL topa
en 6 MB**. Esa subida no puede ir por el camino de peticion: necesita un PUT prefirmado contra el
bucket de la boveda.
