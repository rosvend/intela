# The network the database and the Lambdas live in.
#
# This module CREATES the VPC rather than reading the account's default one.
# Reading the default would be shorter, but it would silently couple Intela to
# whatever else lives in that account -- and this deployment shares an AWS
# Organization management account with unrelated company work. Creating our own
# is also what makes a second deployment, in another account or another region,
# a copy of a tfvars file instead of a surprise.
#
# PostgreSQL is inside this VPC and S3 is reached through a gateway endpoint,
# which is free. The assistant (#66, ADR 0026) calls the Anthropic API, which
# needs outbound internet: one NAT Gateway (about USD 32 a month), switchable
# with enable_nat. CloudWatch Logs works without an endpoint because the Lambda
# service writes the logs on the function's behalf, not through the function's
# own network interface.

variable "name_prefix" {
  description = "Prefix for every resource name, so two deployments never collide."
  type        = string
}

variable "cidr_block" {
  description = "Address space for the VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "subnet_count" {
  description = "How many private subnets to create. RDS requires a subnet group spanning at least two availability zones, so two is the floor."
  type        = number
  default     = 2

  validation {
    condition     = var.subnet_count >= 2
    error_message = "RDS needs a DB subnet group covering at least two availability zones."
  }
}

variable "enable_nat" {
  description = "Create one NAT Gateway (single AZ, about USD 32 a month plus traffic) so the private subnets can reach the internet. Off means no route out of the VPC."
  type        = bool
  default     = false
}
