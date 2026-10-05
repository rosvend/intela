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

# An explicit route table with no default route. The only entry it ever gets is
# the S3 gateway endpoint below, which is what "private, but can still reach the
# object store" means without paying for a NAT Gateway.
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
# Secrets Manager at runtime from inside the VPC. The one exception is Bedrock,
# below, which the assistant cannot do without.
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

resource "aws_vpc_security_group_egress_rule" "lambda_all" {
  security_group_id = aws_security_group.lambda.id
  description       = "Outbound to PostgreSQL, the S3 gateway endpoint and the Bedrock interface endpoint. There is no route to the internet from these subnets."
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
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

# The assistant's model (#66, ADR 0025). Bedrock is reached through an interface
# endpoint so the API keeps having no route to the internet. The alternative, a
# NAT Gateway, is about USD 32 a month plus traffic and opens egress for the
# whole API.
#
# One subnet, on purpose: an interface endpoint bills about USD 7.20 a month
# PER AZ plus USD 0.01 per GB. A Lambda in the other AZ still reaches it, with
# cross-AZ traffic that is negligible for chat-sized payloads. The price is that
# losing this AZ takes the assistant down; nothing else depends on it.
#
# Private DNS makes bedrock-runtime.<region>.amazonaws.com resolve to the
# endpoint, so the SDK needs no custom endpoint URL.
resource "aws_vpc_endpoint" "bedrock_runtime" {
  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.${data.aws_region.current.region}.bedrock-runtime"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [aws_subnet.private[0].id]
  security_group_ids  = [aws_security_group.bedrock_endpoint.id]
  private_dns_enabled = true

  tags = { Name = "${var.name_prefix}-bedrock-runtime" }
}

# The Lambda side needs no new rule: lambda_all already allows all egress, and
# the subnets still have no route out of the VPC.
resource "aws_security_group" "bedrock_endpoint" {
  name        = "${var.name_prefix}-bedrock-endpoint"
  description = "Bedrock runtime interface endpoint. HTTPS from the Intela Lambdas only."
  vpc_id      = aws_vpc.this.id

  tags = { Name = "${var.name_prefix}-bedrock-endpoint" }
}

resource "aws_vpc_security_group_ingress_rule" "bedrock_from_lambda" {
  security_group_id            = aws_security_group.bedrock_endpoint.id
  description                  = "HTTPS from the Intela Lambdas"
  from_port                    = 443
  to_port                      = 443
  ip_protocol                  = "tcp"
  referenced_security_group_id = aws_security_group.lambda.id
}
