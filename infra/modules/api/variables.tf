variable "partition" {
  description = "AWS partition (aws, aws-us-gov, aws-cn). Passed in, never looked up here: a data source in this module would be deferred by the caller's depends_on and churn the role on every deploy. See the note in main.tf."
  type        = string
}

# The HTTP entrypoint.
#
# This module owns exactly one thing the generic go-lambda block does not: how
# the API is reached from outside, which today is a Lambda Function URL. Running
# a Go binary as a Lambda is go-lambda's job and is not repeated here.
#
# It receives database_url as a plain string rather than anything RDS-shaped, so
# moving the database somewhere else never reaches this file.

variable "name_prefix" {
  description = "Prefix for every resource name."
  type        = string
}

variable "zip_path" {
  description = "Deployment package built from cmd/lambda."
  type        = string
}

variable "database_url" {
  description = "PostgreSQL DSN. Opaque here: this module does not care what produced it."
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

# 512 MB is not a round number picked at random. cripto.Bcrypt uses
# bcrypt.DefaultCost, and aplicacion.Autenticacion verifies a decoy hash even
# when the email is unknown, so a login costs roughly 60ms of CPU whether it
# succeeds or fails. Lambda scales CPU with memory; starving this makes every
# login slow.
variable "memory_mb" {
  description = "Memory, which on Lambda is also the CPU dial."
  type        = number
  default     = 512
}

variable "timeout_s" {
  description = "Request timeout."
  type        = number
  default     = 30
}

# Bounds the total number of database connections: this times pool_max_conns.
variable "reserved_concurrency" {
  description = "Cap on simultaneous executions, which is what keeps the connection count under the instance's limit."
  type        = number
  default     = 10
}

variable "function_url_auth_type" {
  description = "NONE while the edge is Amplify, whose rewrite proxy cannot sign SigV4. Switching the edge to CloudFront with OAC means setting this to AWS_IAM -- which is the whole reason it is a variable."
  type        = string
  default     = "NONE"
}

variable "session_ttl" {
  description = "Value for SESION_TTL. Go duration syntax."
  type        = string
  default     = "12h"
}

variable "cors_origins" {
  description = "Value for CORS_ORIGENES. Empty disables CORS entirely, which is correct while the SPA and the API share an origin. Never '*'."
  type        = string
  default     = ""
}

variable "log_format" {
  description = "Value for LOG_FORMATO. json feeds CloudWatch Logs Insights; texto is for humans."
  type        = string
  default     = "json"
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention."
  type        = number
  default     = 14
}

variable "wait_for" {
  description = "Opaque token from the migration run. Sequences this function after goose without a module-level depends_on."
  type        = string
  default     = ""
}
