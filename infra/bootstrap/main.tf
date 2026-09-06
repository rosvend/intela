data "aws_caller_identity" "current" {}

locals {
  tags = merge(var.tags, {
    Project   = var.project_tag
    ManagedBy = "terraform"
    Component = "bootstrap"
  })

  # Never a literal account id. Bucket names are globally unique, so the account
  # id is what keeps two deployments of this repo from colliding.
  state_bucket = "${var.name_prefix}-tfstate-${data.aws_caller_identity.current.account_id}"
}

resource "aws_s3_bucket" "state" {
  bucket = local.state_bucket

  lifecycle {
    # Losing this bucket means losing the map between the code and every
    # resource it manages.
    prevent_destroy = true
  }
}

# Not optional. Terraform state is the only record of what exists; versioning is
# what makes a corrupted or truncated write recoverable.
resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

# The state contains the database password, among other things.
resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

module "github_oidc" {
  source = "../modules/github-oidc"

  name_prefix   = var.name_prefix
  github_owner  = var.github_owner
  github_repo   = var.github_repo
  deploy_branch = var.deploy_branch

  create_oidc_provider       = var.create_oidc_provider
  existing_oidc_provider_arn = var.existing_oidc_provider_arn

  # Passed in, not derived inside the module: the module must not have an
  # opinion about how state buckets are named.
  state_bucket_arn = aws_s3_bucket.state.arn
}
