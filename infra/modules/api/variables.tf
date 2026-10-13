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

variable "vault_bucket_name" {
  description = "Value for OBJECT_BUCKET: the Object Lock bucket that holds raw reports and affiliation documents."
  type        = string
}

variable "vault_policy_json" {
  description = "IAM policy the function needs over the vault bucket. Opaque here: modules/storage writes it."
  type        = string
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

variable "session_idle_timeout" {
  description = "Value for SESION_INACTIVIDAD (ASVS V3.3.2, ADR 0025). Go duration syntax."
  type        = string
  default     = "30m"
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

# RD 13.8.4.3. The public ONI listing has to name where documentation is sent.
# Empty is a production outage: PublicarListadoONI rejects it and POST
# /oni/publicaciones answers 500, so GET /publico/oni never has anything to
# show. There is no default: an invented notification address on a public page
# is worse than a plan that refuses to apply. The value lives in infra/envs.
variable "oni_direccion_fisica" {
  description = "Value for ONI_DIRECCION_FISICA. Physical address where ONI documentation is sent (RD 13.8.4.3). Not a secret: it is printed on a public page."
  type        = string

  validation {
    condition     = length(trimspace(var.oni_direccion_fisica)) > 0
    error_message = "oni_direccion_fisica must be non-empty. An empty value makes POST /oni/publicaciones return 500 and the public ONI listing stays empty."
  }
}

variable "oni_direccion_electronica" {
  description = "Value for ONI_DIRECCION_ELECTRONICA. Electronic address where ONI documentation is sent (RD 13.8.4.3). Not a secret: it is printed on a public page."
  type        = string

  validation {
    condition     = length(trimspace(var.oni_direccion_electronica)) > 0
    error_message = "oni_direccion_electronica must be non-empty. An empty value makes POST /oni/publicaciones return 500 and the public ONI listing stays empty."
  }
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention."
  type        = number
  default     = 14
}

# The in-app assistant (#66). No key means the assistant answers "not available"; the API still serves.
variable "agente_proveedor" {
  description = "Value for AGENTE_PROVEEDOR: bedrock, anthropic, falso, or empty (anthropic when a key is present)."
  type        = string
  default     = ""
}

variable "anthropic_api_key" {
  description = "Value for ANTHROPIC_API_KEY. Comes from the ANTHROPIC_API_KEY repository secret through TF_VAR_anthropic_api_key, never from a file."
  type        = string
  default     = ""
  sensitive   = true
}

variable "agente_modelo" {
  description = "Value for AGENTE_MODELO. Empty uses the adapter default (Haiku 4.5). Another Bedrock model also needs bedrock_policy_json widened."
  type        = string
  default     = ""
}

# Must stay below timeout_s: the assistant answers with an explicit error instead of the runtime killing the request.
variable "agente_plazo" {
  description = "Value for AGENTE_PLAZO, the whole-question deadline. Go duration syntax."
  type        = string
  default     = "25s"
}

variable "bedrock_policy_json" {
  description = "IAM policy that lets the function invoke the assistant's Bedrock models. Opaque here: the composition root writes it."
  type        = string
}
