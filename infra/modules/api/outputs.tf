output "function_url" {
  description = "Origin the edge forwards /api/* to."
  value       = module.function.function_url
}

output "function_name" {
  description = "Name of the API function."
  value       = module.function.function_name
}

output "function_arn" {
  description = "ARN of the API function."
  value       = module.function.function_arn
}
