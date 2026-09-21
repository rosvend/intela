output "function_name" {
  description = "Name of the migration function, for a manual `aws lambda invoke` during an incident."
  value       = module.function.function_name
}

output "result" {
  description = "What the migration run returned. Surfaced so a failed apply shows the goose error in the Terraform output."
  value       = aws_lambda_invocation.migrate.result
}
