# GitHub Actions -> AWS, with no stored credentials.
#
# Two roles, not one, because the two jobs need different power and run on
# different triggers. A pull request from anyone with write access can run the
# plan; only a push to main can apply. Splitting them means a PR cannot deploy
# even if the workflow is edited in that same PR.
#
# This is also the portability story: moving to another AWS account means
# creating the same provider and roles there and updating two GitHub secrets.
# No key rotation, because there are no keys.

variable "name_prefix" {
  description = "Prefix for role names, and the scope of the IAM permissions granted to the deploy role."
  type        = string
}

variable "github_owner" {
  description = "GitHub org or user that owns the repository."
  type        = string
}

variable "github_repo" {
  description = "Repository name."
  type        = string
}

# The numeric ids below are what GitHub's immutable subject claim embeds. Both
# or neither: with only one there is no second subject to build, and the trust
# policy silently keeps trusting just the legacy spelling.
#
#   gh api repos/<owner>/<repo> --jq '.owner.id, .id'
#
# and confirm against the prefix the platform reports:
#
#   gh api repos/<owner>/<repo>/actions/oidc/customization/sub --jq .sub_claim_prefix
variable "github_owner_id" {
  description = "Numeric id of the owner, for GitHub's immutable subject claim. Null keeps only the legacy subject."
  type        = number
  default     = null
}

variable "github_repo_id" {
  description = "Numeric id of the repository, for GitHub's immutable subject claim. Null keeps only the legacy subject."
  type        = number
  default     = null
}

variable "deploy_environment" {
  description = <<-TEXT
    GitHub environment the deploy job declares, if any. When set, the trust
    policy matches `environment:<name>` instead of `ref:refs/heads/<branch>`,
    because that is what GitHub actually puts in the subject for a job that
    declares an environment.

    SETTING THIS MOVES THE BRANCH RESTRICTION OUT OF IAM. The environment's
    deployment branch policy becomes the only thing keeping other branches from
    assuming the deploy role, so it must list the deploy branch and nothing
    else. See the note in main.tf.
  TEXT
  type        = string
  default     = null
}

variable "deploy_branch" {
  description = "Branch allowed to assume the deploy role. Matched exactly, not by prefix."
  type        = string
  default     = "main"
}

variable "state_bucket_arn" {
  description = "ARN of the Terraform state bucket. Passed in rather than looked up, so this module never assumes a bucket naming convention."
  type        = string
}

variable "create_oidc_provider" {
  description = "Whether to create the IAM OIDC provider. Set false if the account already trusts token.actions.githubusercontent.com for another project -- an account can only have one."
  type        = bool
  default     = true
}

variable "existing_oidc_provider_arn" {
  description = "ARN of a pre-existing GitHub OIDC provider, required when create_oidc_provider is false."
  type        = string
  default     = null
}
