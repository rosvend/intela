output "state_bucket" {
  description = "Put this in envs/<name>/backend.hcl as `bucket`."
  value       = aws_s3_bucket.state.id
}

output "plan_role_arn" {
  description = "Store as the AWS_PLAN_ROLE_ARN secret at REPOSITORY level -- not on an environment. See infra/README.md."
  value       = module.github_oidc.plan_role_arn
}

output "deploy_role_arn" {
  description = "Store as the AWS_DEPLOY_ROLE_ARN secret on the `production` ENVIRONMENT, which is where the approval gate lives."
  value       = module.github_oidc.deploy_role_arn
}

output "siguiente_paso" {
  description = "What to do with the values above."
  value       = <<-TEXT
    1. envs/nheo/backend.hcl  ->  bucket = "${aws_s3_bucket.state.id}"

    2. REPOSITORY variable:
         TF_STATE_BUCKET = ${aws_s3_bucket.state.id}

    3. REPOSITORY secrets (a job that calls a reusable workflow cannot read an
       environment secret, and the plan runs on pull requests):
         AWS_PLAN_ROLE_ARN          = ${module.github_oidc.plan_role_arn}
         BUDGET_NOTIFICATION_EMAILS = you@example.com   (comma-separated)

    4. `production` ENVIRONMENT secret, which is where the approval gate lives:
         AWS_DEPLOY_ROLE_ARN = ${module.github_oidc.deploy_role_arn}

    5. Billing console -> Cost allocation tags -> activate `Project`.
  TEXT
}
