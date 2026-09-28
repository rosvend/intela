output "bucket_name" {
  description = "Name of the vault bucket."
  value       = aws_s3_bucket.vault.id
}

output "bucket_arn" {
  description = "ARN of the vault bucket."
  value       = aws_s3_bucket.vault.arn
}

# Only what objetos/s3.go calls; no DeleteObjectVersion nor BypassGovernanceRetention (ADR 0023).
output "adapter_policy_json" {
  description = "IAM policy document for a role that runs the S3 object store adapter."
  value = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "ReadWriteRetainedObjects"
        Effect   = "Allow"
        Action   = ["s3:PutObject", "s3:PutObjectRetention", "s3:GetObject"]
        Resource = "${aws_s3_bucket.vault.arn}/*"
      },
      {
        Sid      = "CompensateAffiliationUploads"
        Effect   = "Allow"
        Action   = ["s3:DeleteObject"]
        Resource = "${aws_s3_bucket.vault.arn}/afiliaciones/*"
      },
      {
        Sid      = "MissingKeyIsNotFound"
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = aws_s3_bucket.vault.arn
      },
    ]
  })
}
