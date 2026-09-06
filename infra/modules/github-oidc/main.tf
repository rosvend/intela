data "aws_partition" "current" {}

locals {
  oidc_arn = var.create_oidc_provider ? aws_iam_openid_connect_provider.github[0].arn : var.existing_oidc_provider_arn

  # The account id is read, never written as a literal, so the same code
  # produces correct ARNs in whatever account it is applied to.
  iam_role_scope   = "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:role/${var.name_prefix}-*"
  iam_policy_scope = "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:policy/${var.name_prefix}-*"

  # THE SUBJECT CLAIM HAS TWO SHAPES, and a role that only trusts one of them
  # cannot be assumed when GitHub sends the other.
  #
  #   legacy     repo:owner/repo:pull_request
  #   immutable  repo:owner@<owner id>/repo@<repo id>:pull_request
  #
  # The immutable form is GitHub's rollout that pins the subject to numeric
  # ids, so renaming a user or repository can no longer silently hand someone
  # else's workflow a role that trusted the old name. Whether a repository
  # emits it is a property of the rollout, NOT of anything in this code: the
  # first real plan against this account failed with
  #
  #   Not authorized to perform sts:AssumeRoleWithWebIdentity
  #
  # and CloudTrail showed the presented subject was the immutable one while
  # this policy demanded the legacy one. `GET /repos/{o}/{r}/actions/oidc/
  # customization/sub` reports the prefix in use as `sub_claim_prefix`.
  #
  # Both are listed, and StringEquals over a list is an OR of EXACT matches --
  # this deliberately does not become StringLike. A wildcard here is what turns
  # "only this repository's pull requests" into "anything that happens to
  # start with these characters", and that property is the whole point of the
  # trust policy.
  subject_prefixes = compact([
    "repo:${var.github_owner}/${var.github_repo}",
    var.github_owner_id != null && var.github_repo_id != null
    ? "repo:${var.github_owner}@${var.github_owner_id}/${var.github_repo}@${var.github_repo_id}"
    : "",
  ])

  plan_subjects = [for p in local.subject_prefixes : "${p}:pull_request"]

  # A JOB THAT DECLARES `environment:` GETS A DIFFERENT SUBJECT. GitHub swaps
  # the `ref:` part for `environment:<name>` -- the branch disappears from the
  # claim entirely. deploy.yml's job declares `environment: production`, so it
  # presents
  #
  #   repo:.../...:environment:production
  #
  # and never the `ref:refs/heads/main` this policy used to demand. That is why
  # the first real deploy failed with an opaque AccessDenied; CloudTrail is
  # where the presented subject is legible.
  #
  # THE BRANCH RESTRICTION THEREFORE LIVES SOMEWHERE ELSE NOW, and this is the
  # part that must not be lost: "only main can deploy" is no longer enforced by
  # this trust policy, because the branch is not in the claim. It is enforced by
  # the environment's DEPLOYMENT BRANCH POLICY, which must list only the deploy
  # branch. Without that policy an environment accepts any branch, and this
  # role would be assumable from any branch that names the environment.
  #
  #   gh api repos/<o>/<r>/environments/<env>/deployment-branch-policies
  #
  # The ref-based subject is deliberately NOT kept as a fallback. Trusting it
  # too would let a job that does NOT declare the environment assume this role,
  # which is exactly how the approval gate would get bypassed.
  deploy_subjects = var.deploy_environment != null ? [
    for p in local.subject_prefixes : "${p}:environment:${var.deploy_environment}"
    ] : [
    for p in local.subject_prefixes : "${p}:ref:refs/heads/${var.deploy_branch}"
  ]
}

data "aws_caller_identity" "current" {}

# An AWS account can hold only one OIDC provider per issuer URL, which is why
# creating it is optional: a second project in the same account must reuse it.
resource "aws_iam_openid_connect_provider" "github" {
  count = var.create_oidc_provider ? 1 : 0

  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]

  # thumbprint_list is deliberately omitted. AWS validates GitHub's OIDC
  # endpoint against its own trusted CAs, so a pinned thumbprint is both
  # unnecessary and a scheduled outage waiting for GitHub to rotate a cert.
  lifecycle {
    ignore_changes = [thumbprint_list]
  }
}

# ---------------------------------------------------------------------------
# Trust
# ---------------------------------------------------------------------------

data "aws_iam_policy_document" "assume_plan" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.oidc_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # Only a pull request in this exact repository. Not StringLike with a
    # wildcard: "repo:owner/*" would trust every repository the org owns.
    # The list holds the legacy and immutable spellings of that one subject --
    # see the note in locals; both entries are still exact.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = local.plan_subjects
    }
  }
}

data "aws_iam_policy_document" "assume_deploy" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.oidc_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    # A push to the deploy branch, and nothing else. A pull request carries a
    # different sub and therefore cannot assume this role, no matter what the
    # workflow file in that pull request says. Two spellings of that one
    # subject, both exact -- see the note in locals.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = local.deploy_subjects
    }
  }
}

# ---------------------------------------------------------------------------
# The plan role: read everything, write only the state lock
# ---------------------------------------------------------------------------

resource "aws_iam_role" "plan" {
  name               = "${var.name_prefix}-gha-plan"
  description        = "Terraform plan from a pull request. Read-only, plus the state lock."
  assume_role_policy = data.aws_iam_policy_document.assume_plan.json
}

resource "aws_iam_role_policy_attachment" "plan_readonly" {
  role       = aws_iam_role.plan.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/ReadOnlyAccess"
}

# The two roles need DIFFERENT access to the state, and giving them the same
# document was the bug: a plan does not write state, only the lock.
#
# Terraform 1.10+ keeps the lock as an object in the bucket (use_lockfile), so
# there is no DynamoDB table -- but it does mean "take the lock" is an S3 write,
# and the naive way to allow it is PutObject over the whole bucket. That is what
# this used to do, for both roles. Since the plan role is assumable by ANY pull
# request, it also meant any pull request could overwrite or delete
# terraform.tfstate: the only record of what exists in the account.
locals {
  state_objects = "${var.state_bucket_arn}/*"
  state_lock    = "${var.state_bucket_arn}/*.tflock"
}

data "aws_iam_policy_document" "state_read" {
  statement {
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketVersioning"]
    resources = [var.state_bucket_arn]
  }

  statement {
    effect    = "Allow"
    actions   = ["s3:GetObject"]
    resources = [local.state_objects]
  }

  # The lock, and nothing else. A plan may take it and release it; it may not
  # touch the state file.
  statement {
    effect    = "Allow"
    actions   = ["s3:PutObject", "s3:DeleteObject"]
    resources = [local.state_lock]
  }
}

data "aws_iam_policy_document" "state_write" {
  statement {
    effect    = "Allow"
    actions   = ["s3:ListBucket", "s3:GetBucketVersioning"]
    resources = [var.state_bucket_arn]
  }

  # An apply does write the state. This role is only assumable by a push to the
  # deploy branch, which is behind branch protection and the environment gate.
  statement {
    effect    = "Allow"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = [local.state_objects]
  }
}

resource "aws_iam_role_policy" "plan_state" {
  name   = "${var.name_prefix}-state"
  role   = aws_iam_role.plan.id
  policy = data.aws_iam_policy_document.state_read.json
}

# ---------------------------------------------------------------------------
# The deploy role
# ---------------------------------------------------------------------------

resource "aws_iam_role" "deploy" {
  name               = "${var.name_prefix}-gha-deploy"
  description        = "Terraform apply from a push to the deploy branch."
  assume_role_policy = data.aws_iam_policy_document.assume_deploy.json
}

resource "aws_iam_role_policy" "deploy_state" {
  name   = "${var.name_prefix}-state"
  role   = aws_iam_role.deploy.id
  policy = data.aws_iam_policy_document.state_write.json
}

data "aws_iam_policy_document" "deploy" {
  # IAM is the one that matters, and it is scoped hard. Terraform may manage
  # execution roles for this project's own Lambdas and nothing else: no other
  # role, no user, no group, no account-level setting.
  statement {
    sid    = "ProjectScopedIAM"
    effect = "Allow"
    actions = [
      "iam:CreateRole",
      "iam:DeleteRole",
      "iam:GetRole",
      "iam:UpdateRole",
      "iam:UpdateRoleDescription",
      "iam:UpdateAssumeRolePolicy",
      "iam:TagRole",
      "iam:UntagRole",
      "iam:ListRoleTags",
      "iam:PassRole",
      "iam:AttachRolePolicy",
      "iam:DetachRolePolicy",
      "iam:ListAttachedRolePolicies",
      "iam:PutRolePolicy",
      "iam:DeleteRolePolicy",
      "iam:GetRolePolicy",
      "iam:ListRolePolicies",
      "iam:CreatePolicy",
      "iam:DeletePolicy",
      "iam:GetPolicy",
      "iam:GetPolicyVersion",
      "iam:ListPolicyVersions",
      "iam:CreatePolicyVersion",
      "iam:DeletePolicyVersion",
      "iam:TagPolicy",
      "iam:UntagPolicy",
    ]
    resources = [local.iam_role_scope, local.iam_policy_scope]
  }

  # The scope above reads as tight, and it is not: both OIDC roles are named
  # ${name_prefix}-gha-*, so `role/${name_prefix}-*` MATCHES THIS ROLE ITSELF.
  # With iam:AttachRolePolicy in the list and no constraint on which policy is
  # attached, whoever could merge to the deploy branch could attach
  # AdministratorAccess to the very role the pipeline runs as. That is a direct
  # path from "merge a PR" to "own the account", in an account that holds other
  # people's work.
  #
  # An explicit Deny is the fix that does not depend on getting the Allow above
  # exactly right: Deny always wins, in this policy and in any future one.
  # Terraform manages the Lambda execution roles, not these two -- these are
  # created once by infra/bootstrap with admin credentials.
  statement {
    sid    = "NoTouchingTheDeploymentRoles"
    effect = "Deny"
    actions = [
      "iam:AttachRolePolicy",
      "iam:DetachRolePolicy",
      "iam:PutRolePolicy",
      "iam:DeleteRolePolicy",
      "iam:UpdateAssumeRolePolicy",
      "iam:UpdateRole",
      "iam:DeleteRole",
    ]
    resources = [aws_iam_role.plan.arn, aws_iam_role.deploy.arn]
  }

  # Second lock on the same door: even for the roles it MAY manage, this one can
  # only attach the two AWS-managed policies a Go Lambda in a VPC actually
  # needs. Without the condition, "manage the execution role" is enough to mint
  # an administrator and pass it to a function.
  statement {
    sid       = "OnlyTheLambdaExecutionPolicies"
    effect    = "Deny"
    actions   = ["iam:AttachRolePolicy"]
    resources = [local.iam_role_scope]

    condition {
      test     = "ArnNotEquals"
      variable = "iam:PolicyARN"
      values = [
        "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
        "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole",
      ]
    }
  }

  # Reading the managed policies it attaches, and the service-linked roles AWS
  # creates on its own for RDS and Lambda-in-VPC.
  statement {
    sid    = "IAMReadAndServiceLinked"
    effect = "Allow"
    actions = [
      "iam:GetPolicy",
      "iam:GetPolicyVersion",
      "iam:ListPolicies",
      "iam:CreateServiceLinkedRole",
    ]
    resources = ["*"]
  }

  # Named resources: everything this project creates carries the prefix.
  statement {
    sid    = "ProjectScopedResources"
    effect = "Allow"
    actions = [
      "lambda:*",
      "ssm:*",
    ]
    resources = [
      "arn:${data.aws_partition.current.partition}:lambda:*:${data.aws_caller_identity.current.account_id}:function:${var.name_prefix}-*",
      "arn:${data.aws_partition.current.partition}:ssm:*:${data.aws_caller_identity.current.account_id}:parameter/${var.name_prefix}/*",
    ]
  }

  statement {
    sid     = "ProjectScopedBuckets"
    effect  = "Allow"
    actions = ["s3:*"]
    resources = [
      "arn:${data.aws_partition.current.partition}:s3:::${var.name_prefix}-*",
      "arn:${data.aws_partition.current.partition}:s3:::${var.name_prefix}-*/*",
    ]
  }

  # These services either do not support resource-level permissions for the
  # create calls Terraform has to make (EC2 for VPC, subnets and security
  # groups), or are account-scoped by nature (Budgets). Narrowing them would
  # mean an ABAC scheme whose failure mode is a half-applied stack -- more risk
  # than it removes at this size. The account boundary, the Project tag and the
  # pipeline's destroy guard are what hold here.
  statement {
    sid    = "ServicesWithoutUsefulResourceScoping"
    effect = "Allow"
    actions = [
      "ec2:*",
      "rds:*",
      "amplify:*",
      "logs:*",
      "budgets:*",
      "cloudwatch:GetMetricData",
      "cloudwatch:ListMetrics",
      "sts:GetCallerIdentity",
      "tag:GetResources",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "deploy" {
  name   = "${var.name_prefix}-deploy"
  role   = aws_iam_role.deploy.id
  policy = data.aws_iam_policy_document.deploy.json
}
