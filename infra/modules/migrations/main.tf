module "function" {
  source = "../go-lambda"

  partition = var.partition

  name        = "${var.name_prefix}-migrate"
  description = "Intela schema migrations, goose (cmd/lambda-migrate)"
  zip_path    = var.zip_path

  memory_mb          = var.memory_mb
  timeout_s          = var.timeout_s
  log_retention_days = var.log_retention_days

  subnet_ids         = var.subnet_ids
  security_group_ids = var.security_group_ids

  environment = {
    DATABASE_URL    = var.database_url
    MIGRATE_TIMEOUT = var.migrate_timeout
    LOG_FORMATO     = var.log_format
    OBJECT_BUCKET   = var.vault_bucket_name
  }
}

# sembrar-dataset writes the seed's raw reports to the vault; inline for the same reason as modules/api.
resource "aws_iam_role_policy" "vault" {
  name   = "${var.name_prefix}-migrate-vault"
  role   = module.function.role_name
  policy = var.vault_policy_json
}

# Running the migration IS the deploy step, so it belongs in the graph rather
# than in a shell step after the apply. If goose fails, the apply fails, and
# module.api never updates -- the old code keeps serving against the old schema,
# which is the correct outcome.
resource "aws_lambda_invocation" "migrate" {
  function_name = module.function.function_name
  input         = jsonencode({ orden = "up" })

  triggers = {
    version = var.app_version
  }
}
