output "url" {
  description = "Public address of the dashboard. /api/* on this host reaches the backend."
  value       = module.frontend.url
}

output "amplify_app_id" {
  description = "Needed by the deploy workflow for `aws amplify start-deployment`."
  value       = module.frontend.app_id
}

output "amplify_branch" {
  description = "Branch the deploy workflow uploads to."
  value       = module.frontend.branch_name
}

output "api_function_name" {
  description = "Name of the API Lambda, for logs and manual invocation."
  value       = module.api.function_name
}

output "migrate_function_name" {
  description = "Name of the migration Lambda, for a manual run during an incident."
  value       = module.migrations.function_name
}

output "database_endpoint" {
  description = "host:port of the database. Not reachable from outside the VPC."
  value       = module.database.endpoint
}

output "database_url_parameter" {
  description = "SSM parameter holding the DSN, for a human who needs it."
  value       = module.database.parameter_name
}

output "vault_bucket" {
  description = "Bucket for raw usage reports."
  value       = module.storage.bucket_name
}
