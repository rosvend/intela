output "vpc_id" {
  description = "Id of the created VPC."
  value       = aws_vpc.this.id
}

output "private_subnet_ids" {
  description = "Private subnets, for the DB subnet group and the Lambda ENIs."
  value       = aws_subnet.private[*].id
}

output "lambda_security_group_id" {
  description = "Security group to attach to every Lambda that needs the database."
  value       = aws_security_group.lambda.id
}

output "database_security_group_id" {
  description = "Security group for the database instance."
  value       = aws_security_group.database.id
}
