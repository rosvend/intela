output "bucket_name" {
  description = "Name of the vault bucket."
  value       = aws_s3_bucket.vault.id
}

output "bucket_arn" {
  description = "ARN of the vault bucket, for the policy the S3 adapter will need."
  value       = aws_s3_bucket.vault.arn
}
