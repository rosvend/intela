# THE COMPOSITION ROOT.
#
# This is the only file in the repository where modules are wired to each other.
# Every module below is a leaf: none of them names another, none of them looks a
# resource up by name, and everything they need arrives as an argument. That is
# the same rule the Go core follows when it takes PuertoReloj instead of calling
# time.Now() -- a component that discovers its own dependencies cannot be
# deployed twice or tested in isolation.

data "aws_caller_identity" "current" {}

locals {
  # Project last, so it wins over anything in var.tags. The cost boundary is not
  # something a caller gets to switch off.
  tags = merge(var.tags, {
    Project     = var.project_tag
    Environment = var.environment
    ManagedBy   = "terraform"
  })

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
  zip_path           = local.migrate_zip_path
  app_version        = var.app_version
  database_url       = module.database.database_url
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.lambda_security_group_id]
}

module "api" {
  source = "../../modules/api"

  name_prefix        = var.name_prefix
  zip_path           = local.api_zip_path
  database_url       = module.database.database_url
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.lambda_security_group_id]

  # CORS off: the SPA and the API share an origin, because the edge rewrites
  # /api/* to the Function URL rather than sending the browser somewhere else.
  cors_origins = ""

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
