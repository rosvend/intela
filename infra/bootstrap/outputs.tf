output "state_bucket" {
  description = "Put this in envs/<name>/backend.hcl as `bucket`."
  value       = aws_s3_bucket.state.id
}

output "plan_role_arn" {
  description = "Store as the AWS_PLAN_ROLE_ARN secret on the production environment."
  value       = module.github_oidc.plan_role_arn
}

output "deploy_role_arn" {
  description = "Store as the AWS_DEPLOY_ROLE_ARN secret on the production environment."
  value       = module.github_oidc.deploy_role_arn
}

output "siguiente_paso" {
  description = "What to do with the values above."
  value       = <<-TEXT
    1. envs/nheo/backend.hcl  ->  bucket = "${aws_s3_bucket.state.id}"
    2. GitHub environment `production` secrets:
         AWS_PLAN_ROLE_ARN   = ${module.github_oidc.plan_role_arn}
         AWS_DEPLOY_ROLE_ARN = ${module.github_oidc.deploy_role_arn}
    3. Billing console -> Cost allocation tags -> activate `Project`.
  TEXT
}
