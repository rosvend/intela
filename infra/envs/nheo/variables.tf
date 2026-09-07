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
  description = "Monthly ceiling in USD."
  type        = number
  default     = 20
}
