# Run once per AWS account, by a human, with admin credentials:
#
#   cd infra/bootstrap
#   terraform init
#   AWS_PROFILE=nheo-roy terraform apply
#
# It creates the two things that must exist before CI can do anything at all:
# the Terraform state bucket, and the roles GitHub Actions assumes. Everything
# else lives in infra/envs/<name>/ and is applied by the pipeline.
#
# Moving to a different AWS account is: run this there with the other profile,
# then update the two GitHub secrets and envs/<name>/backend.hcl.

variable "region" {
  description = "Region for the state bucket. Usually the same one the workloads run in."
  type        = string
  default     = "us-east-1"
}

variable "name_prefix" {
  description = "Prefix for every resource name."
  type        = string
  default     = "intela"
}

variable "project_tag" {
  description = "Value of the Project tag."
  type        = string
  default     = "intela"
}

variable "tags" {
  description = "Extra tags. Project is forced on top and cannot be overridden."
  type        = map(string)
  default     = {}
}

variable "github_owner" {
  description = "GitHub org or user owning the repository."
  type        = string
}

variable "github_repo" {
  description = "Repository name."
  type        = string
}

# Needed because GitHub is moving the OIDC subject claim to a form that embeds
# these ids. See the note in modules/github-oidc/main.tf; get them with
#
#   gh api repos/<owner>/<repo> --jq '.owner.id, .id'
variable "github_owner_id" {
  description = "Numeric owner id, for GitHub's immutable subject claim."
  type        = number
  default     = null
}

variable "github_repo_id" {
  description = "Numeric repository id, for GitHub's immutable subject claim."
  type        = number
  default     = null
}

variable "deploy_environment" {
  description = "GitHub environment the deploy job declares. When set, the deploy role trusts `environment:<name>` instead of the branch ref -- which is what GitHub actually sends. The environment's deployment branch policy then becomes what restricts the branch."
  type        = string
  default     = null
}

variable "deploy_branch" {
  description = "Branch allowed to assume the deploy role."
  type        = string
  default     = "main"
}

variable "create_oidc_provider" {
  description = "False if this account already trusts token.actions.githubusercontent.com for another project. An account can hold only one provider per issuer."
  type        = bool
  default     = true
}

variable "existing_oidc_provider_arn" {
  description = "Required when create_oidc_provider is false."
  type        = string
  default     = null
}
