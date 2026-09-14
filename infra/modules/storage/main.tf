resource "aws_s3_bucket" "vault" {
  bucket = "${var.name_prefix}-reportes-${var.bucket_suffix}"

  # Irreversible after creation, which is exactly why it is set now.
  object_lock_enabled = true
}

# Object Lock requires versioning, and versioning is what makes an overwrite
# recoverable rather than final.
resource "aws_s3_bucket_versioning" "vault" {
  bucket = aws_s3_bucket.vault.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "vault" {
  bucket = aws_s3_bucket.vault.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "vault" {
  bucket = aws_s3_bucket.vault.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
