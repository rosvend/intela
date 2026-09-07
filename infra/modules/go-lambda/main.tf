locals {
  in_vpc = length(var.subnet_ids) > 0

  # AWSLambdaVPCAccessExecutionRole is a superset of the basic one: it carries
  # the CloudWatch Logs permissions too, so it is never both.
  execution_policy = local.in_vpc ? "arn:${var.partition}:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole" : "arn:${var.partition}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"

  # NO `data` BLOCKS IN THIS MODULE, and that is a hard requirement rather than
  # a style preference.
  #
  # A module carrying `depends_on` has EVERY data source inside it deferred to
  # apply time whenever the thing it depends on has a pending change --
  # Terraform names the reason `read_because_dependency_pending`. envs/*/main.tf
  # gives module.api a `depends_on = [module.migrations]` to keep "migrate
  # before the API serves" in the graph, and module.migrations replaces its
  # aws_lambda_invocation on EVERY push, because the commit SHA is its trigger.
  #
  # So on every push these values became unknown at plan time, which forced the
  # role to be updated and its policy attachment to be REPLACED -- policy_arn
  # is ForceNew. A replacement is a delete, so the destroy guard refused the
  # plan, and the deploy blocked itself. Exempting aws_lambda_invocation from
  # the guard was necessary but not sufficient: its replacement cascades.
  #
  # Both values are static, so neither needs a lookup. The partition arrives as
  # a variable, read once at the environment root where nothing defers it, and
  # the trust policy is written out directly. Adding a `data` block back here
  # reintroduces the block on every deploy.
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

# One role per function, not one shared role. They happen to need the same
# permissions today, but they are different blast radii: the migration runner
# will eventually need rights the request path must never have.
resource "aws_iam_role" "this" {
  name               = "${var.name}-exec"
  assume_role_policy = local.assume_role_policy
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

  lifecycle {
    precondition {
      # The comparison is the graph edge: this function is not updated until
      # wait_for is known. The right-hand side is a character that no Lambda
      # invocation id (and no empty default) will ever equal.
      condition     = var.wait_for != "\t"
      error_message = "wait_for is a graph token; a tab is not a valid value."
    }
  }
}

resource "aws_lambda_function_url" "this" {
  count = var.create_function_url ? 1 : 0

  function_name      = aws_lambda_function.this.function_name
  authorization_type = var.function_url_auth_type
}
