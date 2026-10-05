variable "region" {
  description = "AWS region for everything in this environment."
  type        = string
  default     = "us-east-1"
}

variable "name_prefix" {
  description = "Prefix for every resource name. Change it and a second, non-colliding copy of the whole stack can live in the same account."
  type        = string
  default     = "intela"
}

variable "environment" {
  description = "Environment name, used only as a tag. There is one environment today; docs/cd.md says a second is another deploy.yml invocation, not another workflow."
  type        = string
  default     = "production"
}

variable "tags" {
  description = "Extra tags applied to every resource. Project is forced on top of whatever is passed here and cannot be overridden."
  type        = map(string)
  default     = {}
}

variable "project_tag" {
  description = "Value of the Project tag. The boundary between this deployment and unrelated work in the same account."
  type        = string
  default     = "intela"
}

# --- Application artifacts -------------------------------------------------

variable "app_version" {
  description = "Version being deployed, normally the full commit SHA. Changing it is what triggers the migration run."
  type        = string
}

variable "api_zip_path" {
  description = "Deployment package built from cmd/lambda."
  type        = string
  default     = null
}

variable "migrate_zip_path" {
  description = "Deployment package built from cmd/lambda-migrate."
  type        = string
  default     = null
}

# --- Knobs -----------------------------------------------------------------

variable "database_deletion_protection" {
  description = "Refuse to delete the database through the API."
  type        = bool
  default     = true
}

variable "domain_name" {
  description = "Custom domain for the dashboard. Null serves it on the generated amplifyapp.com address."
  type        = string
  default     = null
}

variable "budget_notification_emails" {
  description = "Who gets told when spend crosses a threshold. Comes from the BUDGET_NOTIFICATION_EMAILS secret, never from a file in the repository."
  type        = list(string)

  # This repository is PUBLIC and terraform.yml posts the plan as a pull
  # request comment. Without this, the first plan would publish whoever's
  # address this is, in a comment, on the open internet, for scrapers to find.
  # `sensitive` makes Terraform render it as "(sensitive value)" everywhere it
  # would otherwise print it.
  sensitive = true
}

variable "monthly_budget_usd" {
  description = "Monthly ceiling in USD. The NAT Gateway (about USD 32 a month plus traffic) is why the example uses 45 instead of 20: a user-approved overage (ADR 0026). Without the NAT, 20 is enough."
  type        = number
  default     = 45
}

variable "anthropic_api_key" {
  description = "Key for the assistant's Anthropic provider (#66), the production default. Comes from the ANTHROPIC_API_KEY secret; only read when agente_proveedor = \"anthropic\"."
  type        = string
  default     = ""
  sensitive   = true
}

variable "agente_proveedor" {
  description = "Value for AGENTE_PROVEEDOR. anthropic is the production path (ADR 0026): it needs the key AND enable_nat. bedrock needs the account's Anthropic use-case form; falso is a deterministic stub."
  type        = string
  default     = "anthropic"

  validation {
    condition     = contains(["bedrock", "anthropic", "falso"], var.agente_proveedor)
    error_message = "agente_proveedor must be bedrock, anthropic or falso."
  }
}

variable "enable_nat" {
  description = "One NAT Gateway for outbound internet from the private subnets (about USD 32 a month plus traffic). Needed by agente_proveedor = \"anthropic\" and by Titan embeddings. Turn off after switching to bedrock with the use-case form submitted."
  type        = bool
  default     = true
}
