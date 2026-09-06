# A plain string, not the instance object. Callers that only need "how do I
# connect" should not be able to reach into RDS-specific attributes -- that is
# what would make swapping the engine a cross-module change.
output "database_url" {
  description = "Full PostgreSQL DSN, with sslmode and pgxpool sizing already applied."
  value       = local.database_url
  sensitive   = true
}

output "endpoint" {
  description = "host:port of the instance."
  value       = aws_db_instance.this.endpoint
}

output "parameter_name" {
  description = "SSM parameter holding the DSN, for humans."
  value       = aws_ssm_parameter.database_url.name
}
