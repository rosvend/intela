# PostgreSQL for Intela.
#
# WHY A PROVISIONED INSTANCE AND NOT AURORA SERVERLESS V2
#
# Aurora Serverless v2 can scale to zero ACUs, but only "when the specified
# delay period passes with no connections to the instance". A warm Lambda holds
# its pgx pool open, and pgxpool's idle reaper is a background goroutine that
# does not run while Lambda has the execution environment frozen -- so the
# connection survives and the cluster never pauses. Closing the pool on every
# invocation does work, but one stray connection anywhere (an uptime monitor
# pointed at GET /ready, which queries the database) silently costs
# 0.5 ACU * 730h * $0.12 = $43.80/month. RDS Proxy cannot rescue it either: the
# proxy holds a connection open by design and blocks the pause.
#
# db.t4g.micro is $0.016/hour, about $14/month with 20 GB of gp3. Predictable
# beats cheaper-on-paper when the account is an employer's and the failure mode
# is a quiet 3x overrun. See ADR 0014.
#
# The engine matters and is not swappable: migrations/00001_init.sql needs
# pg_trgm, btree_gist (for an EXCLUDE constraint over a daterange), pgcrypto,
# two plpgsql functions and five triggers.

variable "name_prefix" {
  description = "Prefix for every resource name."
  type        = string
}

variable "subnet_ids" {
  description = "Private subnets for the DB subnet group. Passed in; never discovered."
  type        = list(string)
}

variable "security_group_ids" {
  description = "Security groups controlling who may reach port 5432."
  type        = list(string)
}

variable "engine_version" {
  description = "Major PostgreSQL version. Pinned to the major only, so minor patches land through the maintenance window instead of showing up as Terraform drift."
  type        = string
  default     = "16"
}

variable "instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.micro"
}

variable "allocated_storage_gb" {
  description = "Storage in GB. gp3 bills about $0.115 per GB-month."
  type        = number
  default     = 20
}

variable "database_name" {
  description = "Name of the initial database."
  type        = string
  default     = "intela"
}

variable "master_username" {
  description = "Master user. Not 'postgres', which is what every scanner tries first."
  type        = string
  default     = "intela"
}

variable "backup_retention_days" {
  description = "Automated backup retention. Backups up to the size of the instance storage are free."
  type        = number
  default     = 7
}

variable "deletion_protection" {
  description = "Refuse to delete the instance through the API. Independent of the prevent_destroy lifecycle rule, which stops Terraform itself."
  type        = bool
  default     = true
}

# pgxpool reads this straight out of the DSN (pgxpool.ParseConfig), so capping
# the pool needs no Go code at all. With the API function reserved at 10
# concurrent executions this tops out at 20 connections, against roughly 112
# available on a db.t4g.micro.
variable "pool_max_conns" {
  description = "Value for the pool_max_conns DSN parameter that pgxpool honours."
  type        = number
  default     = 2
}
