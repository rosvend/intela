# Region metadata, not an application resource: this asks "which AZs does this
# region have", which is exactly the kind of thing that must not be hardcoded
# if the same module is to run in another region.
data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "this" {
  cidr_block           = var.cidr_block
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = var.name_prefix }
}

resource "aws_subnet" "private" {
  count = var.subnet_count

  vpc_id            = aws_vpc.this.id
  cidr_block        = cidrsubnet(var.cidr_block, 8, count.index)
  availability_zone = data.aws_availability_zones.available.names[count.index]

  tags = { Name = "${var.name_prefix}-private-${count.index}" }

  # The upper bound on subnet_count, which the variable itself cannot express:
  # a `validation` block may not read a data source, and how many AZs exist is
  # only known after this region is queried. Without it, asking for more
  # subnets than the region has AZs fails on an index out of range -- an error
  # that points at this line and says nothing about the variable that caused
  # it.
  lifecycle {
    precondition {
      condition     = var.subnet_count <= length(data.aws_availability_zones.available.names)
      error_message = "subnet_count (${var.subnet_count}) exceeds the availability zones in this region (${length(data.aws_availability_zones.available.names)}). One subnet per AZ."
    }
  }
}

# An explicit route table. Its entries are the S3 gateway endpoint below and,
# only when var.enable_nat is true, a default route through the NAT Gateway.
# With the toggle off there is no route to the internet at all.
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.name_prefix}-private" }
}

resource "aws_route_table_association" "private" {
  count = var.subnet_count

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# Gateway endpoints are free. Interface endpoints are not -- they bill about
# $7.20 per month per AZ, which is why nothing in this design reads SSM or
# Secrets Manager at runtime from inside the VPC.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${data.aws_region.current.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]

  tags = { Name = "${var.name_prefix}-s3" }
}

data "aws_region" "current" {}

resource "aws_security_group" "lambda" {
  name        = "${var.name_prefix}-lambda"
  description = "Intela Lambda functions. No ingress: invocation arrives through the Lambda service, not the network."
  vpc_id      = aws_vpc.this.id

  tags = { Name = "${var.name_prefix}-lambda" }
}

# Egress is narrowed to what the functions use: PostgreSQL inside the VPC and
# HTTPS (S3 gateway endpoint, and the assistant's model API through the NAT).
resource "aws_vpc_security_group_egress_rule" "lambda_https" {
  security_group_id = aws_security_group.lambda.id
  description       = "HTTPS to the S3 gateway endpoint and, with the NAT, the assistant's model API"
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "lambda_postgres" {
  security_group_id            = aws_security_group.lambda.id
  description                  = "PostgreSQL to the database security group"
  from_port                    = 5432
  to_port                      = 5432
  ip_protocol                  = "tcp"
  referenced_security_group_id = aws_security_group.database.id
}

resource "aws_security_group" "database" {
  name        = "${var.name_prefix}-database"
  description = "Intela PostgreSQL. Reachable only from the Lambda security group."
  vpc_id      = aws_vpc.this.id

  tags = { Name = "${var.name_prefix}-database" }
}

# Referenced by security group, not by CIDR. A CIDR rule would keep working if
# something unrelated were placed in these subnets; this one would not.
resource "aws_vpc_security_group_ingress_rule" "database_from_lambda" {
  security_group_id            = aws_security_group.database.id
  description                  = "PostgreSQL from the Intela Lambdas"
  from_port                    = 5432
  to_port                      = 5432
  ip_protocol                  = "tcp"
  referenced_security_group_id = aws_security_group.lambda.id
}

# Outbound internet for the private subnets (#66, ADR 0026). The assistant calls
# the Anthropic API, and Titan embeddings (#67) go to the public Bedrock endpoint.
# One NAT Gateway in one AZ, on purpose: it bills about USD 32 a month plus USD
# 0.045 per GB, per gateway. If its AZ is lost the assistant goes down; the
# database and the rest of the API do not use it. Turn it off with
# enable_nat = false.
resource "aws_subnet" "public" {
  count = var.enable_nat ? 1 : 0

  vpc_id            = aws_vpc.this.id
  cidr_block        = cidrsubnet(var.cidr_block, 8, 200)
  availability_zone = data.aws_availability_zones.available.names[0]

  tags = { Name = "${var.name_prefix}-public" }
}

resource "aws_internet_gateway" "this" {
  count = var.enable_nat ? 1 : 0

  vpc_id = aws_vpc.this.id

  tags = { Name = var.name_prefix }
}

resource "aws_route_table" "public" {
  count = var.enable_nat ? 1 : 0

  vpc_id = aws_vpc.this.id

  tags = { Name = "${var.name_prefix}-public" }
}

resource "aws_route" "public_internet" {
  count = var.enable_nat ? 1 : 0

  route_table_id         = aws_route_table.public[0].id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this[0].id
}

resource "aws_route_table_association" "public" {
  count = var.enable_nat ? 1 : 0

  subnet_id      = aws_subnet.public[0].id
  route_table_id = aws_route_table.public[0].id
}

resource "aws_eip" "nat" {
  count = var.enable_nat ? 1 : 0

  domain = "vpc"

  tags       = { Name = "${var.name_prefix}-nat" }
  depends_on = [aws_internet_gateway.this]
}

resource "aws_nat_gateway" "this" {
  count = var.enable_nat ? 1 : 0

  allocation_id = aws_eip.nat[0].id
  subnet_id     = aws_subnet.public[0].id

  tags       = { Name = var.name_prefix }
  depends_on = [aws_internet_gateway.this]
}

resource "aws_route" "private_internet" {
  count = var.enable_nat ? 1 : 0

  route_table_id         = aws_route_table.private.id
  destination_cidr_block = "0.0.0.0/0"
  nat_gateway_id         = aws_nat_gateway.this[0].id
}
