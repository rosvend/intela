output "app_id" {
  description = "Amplify app id. GitHub Actions needs it for `aws amplify start-deployment`."
  value       = aws_amplify_app.this.id
}

output "branch_name" {
  description = "Branch that serves production."
  value       = aws_amplify_branch.this.branch_name
}

output "url" {
  description = "Public address of the dashboard, and therefore of /api too."
  value       = var.domain_name == null ? "https://${aws_amplify_branch.this.branch_name}.${aws_amplify_app.this.default_domain}" : "https://${var.domain_name}"
}
