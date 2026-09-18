variable "partition" {
  description = "AWS partition (aws, aws-us-gov, aws-cn). Passed in, never looked up here: a data source in this module would be deferred by the caller's depends_on and churn the role on every deploy. See the note in main.tf."
  type        = string
}

# Reusable building block: one Go binary running as a Lambda function.
#
# This module knows nothing about Intela. It takes a zip, a handful of runtime
# knobs, and produces a function with its OWN execution role and its OWN log
# group. Anything environment-specific arrives as a variable; nothing is looked
# up by name. That is what lets the same module serve the API, the migration
# runner, and later the worker and the scheduler, without being modified.

variable "name" {
  description = "Fully qualified function name. Callers derive it from name_prefix."
  type        = string
}

variable "description" {
  description = "What this function is for. Shows up in the console."
  type        = string
  default     = ""
}

variable "zip_path" {
  description = "Path to the deployment package on the machine running Terraform."
  type        = string
}

variable "memory_mb" {
  description = "Memory size. On Lambda, CPU scales with memory, so this is also the CPU dial."
  type        = number
  default     = 512
}

variable "timeout_s" {
  description = "Maximum execution time in seconds."
  type        = number
  default     = 30
}

variable "environment" {
  description = "Environment variables handed to the process."
  type        = map(string)
  default     = {}
  sensitive   = true
}

# Empty lists mean "not attached to a VPC". Passing subnets flips the execution
# role from the basic managed policy to the VPC one, because a function with an
# ENI needs ec2:CreateNetworkInterface and friends to start at all.
variable "subnet_ids" {
  description = "Private subnets to attach the function to. Empty means no VPC."
  type        = list(string)
  default     = []
}

variable "security_group_ids" {
  description = "Security groups for the function's ENI. Required when subnet_ids is set."
  type        = list(string)
  default     = []
}

variable "reserved_concurrency" {
  description = "Cap on simultaneous executions. -1 leaves the function unreserved. Set it when the function talks to a database with a small connection budget."
  type        = number
  default     = -1
}

variable "create_function_url" {
  description = "Whether to expose a Lambda Function URL."
  type        = bool
  default     = false
}

variable "function_url_auth_type" {
  description = "NONE for a publicly reachable URL, AWS_IAM to require SigV4 (what CloudFront OAC signs with)."
  type        = string
  default     = "NONE"

  validation {
    condition     = contains(["NONE", "AWS_IAM"], var.function_url_auth_type)
    error_message = "function_url_auth_type must be NONE or AWS_IAM."
  }
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention. Left to Lambda, log groups are created with no expiry and the bill grows forever."
  type        = number
  default     = 14
}

variable "architecture" {
  description = "Lambda instruction set. arm64 is cheaper per GB-second than x86_64."
  type        = string
  default     = "arm64"
}

# Graph token, not configuration. A module-level `depends_on` defers every data
# source inside the module until apply, which makes policy_arn unknown and
# ForceNew-replaces aws_iam_role_policy_attachment on every plan
# (hashicorp/terraform-provider-aws#32529). Referencing this value from the
# function itself sequences goose before the new code without that deferral.
# Empty means no extra edge.
variable "wait_for" {
  description = "Opaque token from an upstream resource. The function is not updated until this value is known."
  type        = string
  default     = ""
}
