locals {
  # AWS Budgets wants "user:<Key>$<Value>". Built with format() rather than
  # string interpolation because a literal '$' immediately before '${' is an
  # escaping trap in HCL.
  tag_filter = format("user:%s$%s", var.tag_key, var.tag_value)
}

resource "aws_budgets_budget" "project" {
  name         = "${var.name_prefix}-monthly"
  budget_type  = "COST"
  limit_amount = tostring(var.monthly_limit_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # Scoped to this project's tag, not to the whole account -- the account also
  # carries unrelated company spend, which must neither trigger these alerts nor
  # be hidden by them.
  cost_filter {
    name   = "TagKeyValue"
    values = [local.tag_filter]
  }

  dynamic "notification" {
    for_each = var.thresholds_percent

    content {
      comparison_operator = "GREATER_THAN"
      threshold           = notification.value
      threshold_type      = "PERCENTAGE"

      # ACTUAL, not FORECASTED: a forecast on a brand-new deployment with no
      # history produces noise for weeks.
      notification_type = "ACTUAL"

      subscriber_email_addresses = var.notification_emails
    }
  }
}
