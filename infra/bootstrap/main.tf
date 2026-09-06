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

# Versioning above keeps EVERY past state file, and each one carries the
# database password of its moment (ADR 0014 took that tradeoff knowingly, to
# avoid a $7.20/month Secrets Manager endpoint). Without expiry that is a
# growing pile of old credentials with no end date.
#
# 90 days is the recovery window: long enough to roll back to a state from
# before a bad apply, which is the whole reason versioning is on, and short
# enough that a password rotated today stops being retrievable this quarter
# rather than never. Deleted markers go sooner -- they recover nothing.
resource "aws_s3_bucket_lifecycle_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  # The lifecycle rules read the versioning configuration, so they must be
  # applied after it exists rather than racing it.
  depends_on = [aws_s3_bucket_versioning.state]

  rule {
    id     = "expire-noncurrent-state"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }

  rule {
    id     = "tidy-up"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }

    expiration {
      expired_object_delete_marker = true
    }
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
