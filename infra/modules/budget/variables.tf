# The cost tripwire.
#
# Everything here is tagged Project=intela, which is what makes this deployment
# separable from unrelated work in the same AWS Organization management account.
# Tagging alone is an intention; a budget filtered on that tag is what turns it
# into something that tells you when it stops being true.
#
# ONE MANUAL STEP: `Project` has to be activated as a cost allocation tag in the
# Billing console, from the management account. Until that is done the tag filter
# reports nothing. See infra/README.md.
#
# The first two budgets in an account are free.

variable "name_prefix" {
  description = "Prefix for the budget's own name."
  type        = string
}

# Deliberately not derived from name_prefix. The two can diverge -- a second
# deployment might use name_prefix = "intela-dev" while still belonging to the
# same Project for cost purposes.
variable "tag_key" {
  description = "Cost allocation tag key to filter on."
  type        = string
  default     = "Project"
}

variable "tag_value" {
  description = "Cost allocation tag value to filter on."
  type        = string
  default     = "intela"
}

variable "monthly_limit_usd" {
  description = "Monthly ceiling in USD."
  type        = number
  default     = 20
}

variable "notification_emails" {
  description = "Who hears about it. Empty creates the budget without alerts, which defeats the point."
  type        = list(string)
}

variable "thresholds_percent" {
  description = "Percentages of the limit that trigger a notification. 80 is the warning, 100 is the ceiling."
  type        = list(number)
  default     = [80, 100]
}
