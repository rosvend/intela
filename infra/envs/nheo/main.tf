# THE COMPOSITION ROOT.
#
# This is the only file in the repository where modules are wired to each other.
# Every module below is a leaf: none of them names another, none of them looks a
# resource up by name, and everything they need arrives as an argument. That is
# the same rule the Go core follows when it takes PuertoReloj instead of calling
# time.Now() -- a component that discovers its own dependencies cannot be
# deployed twice or tested in isolation.

data "aws_caller_identity" "current" {}

# Read HERE, at the root, and passed down -- never inside modules/go-lambda.
# A data source inside a module that carries `depends_on` is deferred to apply
# time on every run where the dependency changes, which made the Lambda roles
# churn and the destroy guard refuse every deploy. See modules/go-lambda/main.tf.
data "aws_partition" "current" {}

locals {
  # Project last, so it wins over anything in var.tags. The cost boundary is not
  # something a caller gets to switch off.
  tags = merge(var.tags, {
    Project     = var.project_tag
    Environment = var.environment
    ManagedBy   = "terraform"
  })

  # The assistant's chat model, called through the cross-Region profile "us.",
  # which routes to these three regions (GetInferenceProfile). ADR 0025.
  haiku_model_id        = "anthropic.claude-haiku-4-5-20251001-v1:0"
  haiku_profile_arn     = "arn:${data.aws_partition.current.partition}:bedrock:${var.region}:${data.aws_caller_identity.current.account_id}:inference-profile/us.${local.haiku_model_id}"
  haiku_profile_regions = ["us-east-1", "us-east-2", "us-west-2"]

  repo_root        = "${path.root}/../../.."
  api_zip_path     = coalesce(var.api_zip_path, "${local.repo_root}/dist/lambda/api.zip")
  migrate_zip_path = coalesce(var.migrate_zip_path, "${local.repo_root}/dist/lambda/migrate.zip")
}

module "network" {
  source = "../../modules/network"

  name_prefix = var.name_prefix
}

module "database" {
  source = "../../modules/database"

  name_prefix         = var.name_prefix
  subnet_ids          = module.network.private_subnet_ids
  security_group_ids  = [module.network.database_security_group_id]
  deletion_protection = var.database_deletion_protection
}

module "storage" {
  source = "../../modules/storage"

  name_prefix = var.name_prefix
  # S3 bucket names are globally unique. The account id is read, never written
  # as a literal, so this is correct in whatever account it lands in.
  bucket_suffix = data.aws_caller_identity.current.account_id
}

module "migrations" {
  source = "../../modules/migrations"

  name_prefix        = var.name_prefix
  partition          = data.aws_partition.current.partition
  zip_path           = local.migrate_zip_path
  app_version        = var.app_version
  database_url       = module.database.database_url
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.lambda_security_group_id]
  vault_bucket_name  = module.storage.bucket_name
  vault_policy_json  = module.storage.adapter_policy_json
}

module "api" {
  source = "../../modules/api"

  name_prefix        = var.name_prefix
  partition          = data.aws_partition.current.partition
  zip_path           = local.api_zip_path
  database_url       = module.database.database_url
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.lambda_security_group_id]
  vault_bucket_name  = module.storage.bucket_name
  vault_policy_json  = module.storage.adapter_policy_json

  # CORS off: the SPA and the API share an origin, because the edge rewrites
  # /api/* to the Function URL rather than sending the browser somewhere else.
  cors_origins = ""

  # RD 13.8.4.3. Published by REDES SGC on https://redescritores.com/contacto/
  # (sede Torre REM, Bogota; correo general de la sociedad). Printed on the
  # public ONI listing, so they are not secrets and do not belong in tfvars.
  # No default in the module: compose's placeholders are for local only.
  oni_direccion_fisica      = "Carrera 14 No. 99-33, Oficina 602, Torre REM, Bogota D.C."
  oni_direccion_electronica = "redescritorescolombia@redescritores.com"

  # The assistant (#66, ADR 0025): Bedrock through the VPC endpoint. The
  # Anthropic key stays wired for agente_proveedor = "anthropic", which cannot
  # reach api.anthropic.com from these subnets.
  agente_proveedor    = var.agente_proveedor
  anthropic_api_key   = var.anthropic_api_key
  bedrock_policy_json = data.aws_iam_policy_document.bedrock.json

  # THE ORDERING GUARANTEE. docs/cd.md requires the schema to move before the
  # new code serves traffic, and ADR 0008 explains the cost of getting it wrong:
  # a reparto run in flight must not meet a schema its code does not know. If
  # goose fails, this never updates and the old code keeps serving.
  #
  # It lives here rather than inside a module so the guarantee is visible where
  # the system is assembled.
  depends_on = [module.migrations]
}

module "frontend" {
  source = "../../modules/frontend"

  name_prefix = var.name_prefix
  domain_name = var.domain_name

  # An opaque string. modules/frontend does not know there is a Lambda behind
  # this, which is what keeps swapping Amplify for CloudFront a local change.
  api_origin_url = module.api.function_url
}

module "budget" {
  source = "../../modules/budget"

  name_prefix         = var.name_prefix
  tag_key             = "Project"
  tag_value           = var.project_tag
  monthly_limit_usd   = var.monthly_budget_usd
  notification_emails = var.budget_notification_emails
}

# Least privilege for the assistant: the Haiku inference profile, the foundation
# model behind it only when called THROUGH that profile, and Titan embeddings
# (#67). Converse is authorized as bedrock:InvokeModel; no streaming is used.
data "aws_iam_policy_document" "bedrock" {
  statement {
    sid       = "HaikuInferenceProfile"
    actions   = ["bedrock:InvokeModel"]
    resources = [local.haiku_profile_arn]
  }

  statement {
    sid     = "HaikuThroughTheProfileOnly"
    actions = ["bedrock:InvokeModel"]
    resources = [
      for r in local.haiku_profile_regions :
      "arn:${data.aws_partition.current.partition}:bedrock:${r}::foundation-model/${local.haiku_model_id}"
    ]

    condition {
      test     = "StringEquals"
      variable = "bedrock:InferenceProfileArn"
      values   = [local.haiku_profile_arn]
    }
  }

  statement {
    sid       = "TitanEmbeddings"
    actions   = ["bedrock:InvokeModel"]
    resources = ["arn:${data.aws_partition.current.partition}:bedrock:${var.region}::foundation-model/amazon.titan-embed-text-v2:0"]
  }
}
