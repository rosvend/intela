module "function" {
  source = "../go-lambda"

  partition = var.partition

  name        = "${var.name_prefix}-api"
  description = "Intela HTTP API (cmd/lambda)"
  zip_path    = var.zip_path

  memory_mb            = var.memory_mb
  timeout_s            = var.timeout_s
  reserved_concurrency = var.reserved_concurrency
  log_retention_days   = var.log_retention_days

  subnet_ids         = var.subnet_ids
  security_group_ids = var.security_group_ids

  create_function_url    = true
  function_url_auth_type = var.function_url_auth_type

  # ADDR, HTTP_*_TIMEOUT and SHUTDOWN_TIMEOUT are deliberately absent. Lambda
  # owns the socket and the lifecycle, so setting them would be configuration
  # that reads as meaningful and does nothing.
  environment = {
    DATABASE_URL              = var.database_url
    SESION_TTL                = var.session_ttl
    SESION_INACTIVIDAD        = var.session_idle_timeout
    CORS_ORIGENES             = var.cors_origins
    LOG_FORMATO               = var.log_format
    ONI_DIRECCION_FISICA      = var.oni_direccion_fisica
    ONI_DIRECCION_ELECTRONICA = var.oni_direccion_electronica
    OBJECT_BUCKET             = var.vault_bucket_name
    AGENTE_PROVEEDOR          = var.agente_proveedor
    ANTHROPIC_API_KEY         = var.anthropic_api_key
    AGENTE_MODELO             = var.agente_modelo
    AGENTE_PLAZO              = var.agente_plazo
  }
}

# Inline, not attached: the deploy role may only attach the two Lambda execution policies.
resource "aws_iam_role_policy" "vault" {
  name   = "${var.name_prefix}-api-vault"
  role   = module.function.role_name
  policy = var.vault_policy_json
}
