# The schema migration runner.
#
# Separate from modules/api even though the two share most of their
# configuration, because they have different reasons to change, different
# lifecycles and different failure modes. Migration strategy can be replaced
# here without opening the module that serves requests.
#
# It has to run inside the VPC: the database has no public endpoint, so a
# GitHub Actions runner cannot reach it. The migrations themselves are compiled
# into the binary (migrations/embed.go), so nothing is mounted or copied.
#
# ORDERING. docs/cd.md requires migrations to land before the new code serves
# traffic, and ADR 0008 explains why: a reparto run in flight during a deploy
# must not meet a schema its code does not know. That ordering is expressed in
# the root module as `depends_on = [module.migrations]` on module.api -- visible
# at the composition point rather than buried in here.

variable "name_prefix" {
  description = "Prefix for every resource name."
  type        = string
}

variable "zip_path" {
  description = "Deployment package built from cmd/lambda-migrate."
  type        = string
}

variable "database_url" {
  description = "PostgreSQL DSN."
  type        = string
  sensitive   = true
}

variable "subnet_ids" {
  description = "Private subnets the function attaches to."
  type        = list(string)
}

variable "security_group_ids" {
  description = "Security groups for the function's ENI."
  type        = list(string)
}

# This is the trigger. When it changes, the migration runs; when it does not,
# Terraform leaves the invocation alone and a re-apply is a no-op.
variable "app_version" {
  description = "Version being deployed, normally the commit SHA. Changing it is what causes migrations to run."
  type        = string
}

variable "memory_mb" {
  description = "Memory for the migration runner."
  type        = number
  default     = 512
}

variable "timeout_s" {
  description = "Lambda timeout. Must exceed MIGRATE_TIMEOUT so the process fails with a readable goose error instead of being killed mid-statement."
  type        = number
  default     = 300
}

variable "migrate_timeout" {
  description = "Value for MIGRATE_TIMEOUT. Go duration syntax. Kept below timeout_s on purpose."
  type        = string
  default     = "4m"
}

variable "log_format" {
  description = "Value for LOG_FORMATO."
  type        = string
  default     = "json"
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention."
  type        = number
  default     = 14
}
