output "oidc_provider_arn" {
  description = "ARN of the GitHub OIDC provider in use."
  value       = local.oidc_arn
}

output "plan_role_arn" {
  description = "Set this as the AWS_PLAN_ROLE_ARN secret."
  value       = aws_iam_role.plan.arn
}

output "deploy_role_arn" {
  description = "Set this as the AWS_DEPLOY_ROLE_ARN secret."
  value       = aws_iam_role.deploy.arn
}
