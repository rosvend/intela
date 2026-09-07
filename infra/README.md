# Infraestructura

AWS serverless descrito en Terraform. La decision y sus alternativas estan en
[ADR 0014](../docs/decisiones/0014-infraestructura-serverless-en-aws.md); el camino de release, en
[`docs/cd.md`](../docs/cd.md). Aqui va solo como se opera.

```
infra/
├── bootstrap/     Se corre UNA vez por cuenta, a mano. Estado local
├── modules/       Bloques. Ninguno llama a otro salvo go-lambda
└── envs/nheo/     El punto de composicion. Un directorio por cuenta
```

## Que hay desplegado

| Pieza | Servicio | Coste |
| ----- | -------- | ----- |
| API (`cmd/lambda`) | Lambda `provided.al2023` arm64, con Function URL | capa gratuita |
| Migraciones (`cmd/lambda-migrate`) | Lambda en la VPC, invocada por Terraform | capa gratuita |
| Tablero (`web/dist`) | Amplify Hosting, con rewrite de `/api/*` | ~$0 |
| Base | RDS PostgreSQL `db.t4g.micro`, subred privada | **~$14/mes** |
| Boveda de reportes | S3 con Object Lock habilitado, sin retencion por defecto | < $0.50 |
| Red | VPC, dos subredes privadas, endpoint S3 gateway | $0 |
| Vigilancia de gasto | AWS Budgets filtrado por `Project=intela` | $0 |

**RDS es el unico coste fijo mensual.** S3 tambien cobra sin trafico, pero por almacenamiento, asi
que empieza en centimos y crece con las parrillas guardadas, no con el calendario. Sin NAT Gateway,
sin balanceador y sin VPC interface endpoints, que costarian $7.20/mes cada uno.

## Arrancar en una cuenta nueva

```bash
# 1. Bucket de estado y roles de OIDC. Una sola vez, con credenciales de admin.
cd infra/bootstrap
cp terraform.tfvars.example terraform.tfvars   # editar
terraform init
AWS_PROFILE=nheo-roy terraform apply
```

Imprime tres valores: el bucket de estado y los ARN de los dos roles. Cargarlos en GitHub asi
—**el sitio importa, y no es el mismo para los dos roles**:

| Valor | Donde va | Por que ahi |
| ----- | -------- | ----------- |
| bucket de estado | Variable de **repositorio** `TF_STATE_BUCKET` | La leen el plan y el despliegue |
| ARN del rol de plan | Secreto de **repositorio** `AWS_PLAN_ROLE_ARN` | Un secreto de entorno solo lo ve un job que declara ese entorno, y el job del plan no puede declarar `production` sin dejar cada PR esperando la aprobacion de despliegue. El rol es de solo lectura y su confianza OIDC solo admite `…:pull_request` |
| ARN del rol de despliegue | Secreto del **entorno `production`** `AWS_DEPLOY_ROLE_ARN` | Es el que escribe en la cuenta, asi que va detras de la compuerta. Nadie se lo pasa: `deploy.yml` declara `environment: production` y **lo resuelve el propio job**, que es la unica forma —un job que llama a un workflow reutilizable no puede declarar `environment:`, asi que el llamante ni siquiera puede leerlo |
| Emails del presupuesto | Secreto de **repositorio** `BUDGET_NOTIFICATION_EMAILS` | Separados por coma. Secreto y no variable **porque este repositorio es publico** y `terraform.yml` publica el plan como comentario de PR: una variable normal publicaria la direccion. La variable de Terraform va ademas marcada `sensitive`, asi que el plan la imprime como `(sensitive value)` |

Poner el rol de plan como secreto de entorno lo deja invisible y el plan se salta reportando verde,
que es peor que fallar.

**Hasta que estos tres valores existan, el job de despliegue de `main` sale en rojo en cada push.**
Es deliberado: `deploy.yml` falla en vez de apartarse, porque un despliegue que no hace nada y
reporta verde es justo el check que se lee como prueba. El mensaje dice que falta y remite aqui.

Y un paso manual que no sale del `apply`:

**Consola de Billing -> Cost allocation tags -> activar `Project`.** Solo se puede desde la cuenta
de gestion, y hasta que se haga, el presupuesto no mide nada.

```bash
# El entorno.
cd infra/envs/nheo
cp backend.hcl.example backend.hcl             # bucket del paso 1
cp terraform.tfvars.example terraform.tfvars   # editar
terraform init -backend-config=backend.hcl
```

A partir de aqui, desde la raiz del repositorio:

```bash
make plan       # construye los zips y planifica
make aplicar    # construye, planifica y aplica
```

`plan` y `aplicar` dependen de `make lambda` porque `modules/go-lambda` calcula el hash del zip: sin
artefactos el plan ni siquiera evalua.

## Mudar de cuenta

Es el caso que el diseno tiene que soportar sin reescritura:

1. `cp -r infra/envs/nheo infra/envs/personal` y editar sus dos ficheros de ejemplo.
2. Correr `infra/bootstrap/` en la cuenta nueva, con el otro perfil.
3. `terraform init -backend-config=backend.hcl` en el directorio nuevo.
4. Actualizar `TF_STATE_BUCKET` y los dos ARN en GitHub.

**`modules/` no se toca.** Lo que lo permite: ningun ID de cuenta ni ARN literal en el codigo
—se leen con `aws_caller_identity`, `aws_region` y `aws_partition`—, todo nombre derivado de
`name_prefix`, y ningun modulo que busque un recurso preexistente por nombre.

## Las reglas de acoplamiento

Son las de `CLAUDE.md`, aplicadas a la infraestructura. CI las comprueba en la etapa
`Infrastructure`, y se pueden correr a mano:

```bash
# Un modulo no llama a otro modulo. Solo go-lambda se compone.
grep -rn 'source *= *"\.\./' infra/modules/

# Un modulo no descubre recursos: solo lee cuenta, region, particion y AZ.
grep -rhoE '^data "aws_[a-z_]+"' infra/modules/ | sort -u

# Nunca un ID de cuenta literal.
grep -rnE '[0-9]{12}' infra/ --include='*.tf'
```

Y la prueba de fondo: **cada modulo valida aislado**. Si uno solo valida como parte del raiz, ha
cogido una dependencia que no declara.

```bash
for d in infra/modules/*/ infra/envs/*/ infra/bootstrap/; do
  terraform -chdir="$d" init -backend=false && terraform -chdir="$d" validate
done
```

## Cosas que sorprenden

**`prevent_destroy` en la base y en el bucket de estado.** `terraform destroy` falla a proposito.
Quitarlo es una edicion deliberada de dos pasos, que es justo lo que se quiere que sea.

**Object Lock esta activado en la boveda, sin regla de retencion.** Solo se puede activar al crear
el bucket, asi que dejarlo para luego significaria recrearlo. Sin regla por defecto nada queda
inmutable todavia, asi que el bucket aun se puede borrar; la retencion se activa con el adaptador S3.

**Las migraciones corren dentro del `apply`, no en un paso del workflow.** El orden lo impone
`depends_on` en `envs/nheo/main.tf`. Si goose falla, el apply falla y la API se queda con el codigo
anterior — que es lo que exige el [ADR 0008](../docs/decisiones/0008-reparto-como-flujo-con-aprobaciones.md).

**El tamano del pool va en el DSN.** `pool_max_conns=2` lo lee `pgxpool.ParseConfig`, asi que limitar
conexiones no necesita ni una linea de Go. El otro lado del limite es la concurrencia reservada de la
funcion: 10 x 2 = 20 conexiones, contra ~112 que da una `t4g.micro`.
