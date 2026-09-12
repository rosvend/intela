# Alphanumeric on purpose. The password goes into a URL-shaped DSN, and RDS
# separately rejects '/', '@', '"' and spaces in master passwords; 32
# alphanumeric characters sidesteps both problems without percent-encoding.
resource "random_password" "master" {
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "this" {
  name       = var.name_prefix
  subnet_ids = var.subnet_ids

  tags = { Name = var.name_prefix }
}

resource "aws_db_parameter_group" "this" {
  name   = "${var.name_prefix}-pg${var.engine_version}"
  family = "postgres${var.engine_version}"

  parameter {
    name  = "rds.force_ssl"
    value = "1"
    # Static parameter: it only takes effect after a reboot, and RDS rejects the
    # change outright if we claim it can be applied immediately.
    apply_method = "pending-reboot"
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_db_instance" "this" {
  identifier = var.name_prefix

  engine         = "postgres"
  engine_version = var.engine_version
  instance_class = var.instance_class

  allocated_storage = var.allocated_storage_gb
  storage_type      = "gp3"
  storage_encrypted = true

  db_name  = var.database_name
  username = var.master_username
  password = random_password.master.result

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = var.security_group_ids
  parameter_group_name   = aws_db_parameter_group.this.name

  # No public endpoint. The only route to 5432 is from the Lambda security group
  # inside this VPC.
  publicly_accessible = false
  multi_az            = false

  backup_retention_period    = var.backup_retention_days
  auto_minor_version_upgrade = true

  # Performance Insights is off deliberately: it costs money on some classes and
  # buys nothing at this scale.
  performance_insights_enabled = false

  deletion_protection       = var.deletion_protection
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name_prefix}-final"

  lifecycle {
    # The last line of defence. The CI destroy guard catches a bad plan before
    # it is applied; this catches one that is applied anyway, from a laptop.
    # Removing it is a deliberate two-step edit, which is the point.
    prevent_destroy = true

    # The provider cannot read the password back, so it would otherwise propose
    # a change on every plan.
    ignore_changes = [password]
  }
}

locals {
  # aws_db_instance.endpoint is already "host:port", which is a valid authority
  # in a libpq URL.
  database_url = "postgres://${var.master_username}:${random_password.master.result}@${aws_db_instance.this.endpoint}/${var.database_name}?sslmode=require&pool_max_conns=${var.pool_max_conns}"
}

# Nothing reads this at runtime -- the Lambdas receive the DSN as an environment
# variable, because reading SSM from inside a VPC would need an interface
# endpoint at roughly $7.20/month. It exists so a human can retrieve the
# connection string without reading Terraform state. Standard-tier parameters
# are free.
resource "aws_ssm_parameter" "database_url" {
  name        = "/${var.name_prefix}/database_url"
  description = "PostgreSQL DSN for Intela, including the pgxpool sizing parameters."
  type        = "SecureString"
  # Explicit: AWS stores `text` and data_type is ForceNew. Leaving it unset
  # lets a refresh see `text` against a null in config and replace the
  # parameter on every plan, which the destroy guard then refuses.
  data_type = "text"
  value     = local.database_url
}
