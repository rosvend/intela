locals {
  in_vpc = length(var.subnet_ids) > 0

  # AWSLambdaVPCAccessExecutionRole is a superset of the basic one: it carries
  # the CloudWatch Logs permissions too, so it is never both.
  execution_policy = local.in_vpc ? "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole" : "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# The partition is read rather than hardcoded as "aws" so the module also works
# in GovCloud or China without an edit. Same reason we never write an account id.
data "aws_partition" "current" {}

data "aws_iam_policy_document" "assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

# One role per function, not one shared role. They happen to need the same
# permissions today, but they are different blast radii: the migration runner
# will eventually need rights the request path must never have.
resource "aws_iam_role" "this" {
  name               = "${var.name}-exec"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.this.name
  policy_arn = local.execution_policy
}

# Created explicitly, ahead of the function, so Terraform owns the retention.
# If Lambda creates it implicitly on first invocation it never expires.
resource "aws_cloudwatch_log_group" "this" {
  name              = "/aws/lambda/${var.name}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "this" {
  function_name = var.name
  description   = var.description
  role          = aws_iam_role.this.arn

  # provided.al2023 is the custom-runtime family: the zip carries a single
  # executable named `bootstrap`, which is the Go binary. The handler field is
  # inert for this runtime but the API still requires a value.
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = [var.architecture]

  filename         = var.zip_path
  source_code_hash = filebase64sha256(var.zip_path)

  memory_size = var.memory_mb
  timeout     = var.timeout_s

  reserved_concurrent_executions = var.reserved_concurrency

  dynamic "environment" {
    for_each = length(var.environment) > 0 ? [1] : []
    content {
      variables = var.environment
    }
  }

  dynamic "vpc_config" {
    for_each = local.in_vpc ? [1] : []
    content {
      subnet_ids         = var.subnet_ids
      security_group_ids = var.security_group_ids
    }
  }

  # Without this the first invocation races Lambda's own log-group creation and
  # the retention setting can be lost.
  depends_on = [
    aws_cloudwatch_log_group.this,
    aws_iam_role_policy_attachment.execution,
  ]
}

resource "aws_lambda_function_url" "this" {
  count = var.create_function_url ? 1 : 0

  function_name      = aws_lambda_function.this.function_name
  authorization_type = var.function_url_auth_type
}
