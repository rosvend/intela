output "function_name" {
  description = "Name of the created function."
  value       = aws_lambda_function.this.function_name
}

output "function_arn" {
  description = "ARN of the created function."
  value       = aws_lambda_function.this.arn
}

output "role_name" {
  description = "Name of the execution role, so callers can attach extra policies without this module knowing about them."
  value       = aws_iam_role.this.name
}

output "role_arn" {
  description = "ARN of the execution role."
  value       = aws_iam_role.this.arn
}

output "function_url" {
  description = "The Function URL, or null when create_function_url is false."
  value       = var.create_function_url ? aws_lambda_function_url.this[0].function_url : null
}
