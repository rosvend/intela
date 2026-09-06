terraform {
  # 1.10 is the floor: use_lockfile below is what replaces the DynamoDB lock
  # table, and it does not exist before then.
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # Partial configuration. bucket, key and region arrive from backend.hcl, which
  # is gitignored because it names an account-specific bucket. Moving accounts
  # is `terraform init -backend-config=<other file>` and nothing else.
  backend "s3" {
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.region

  # Every resource in this state gets these, without any module knowing about
  # them. Tagging is an account policy, so it is decided here, once, at the
  # composition point -- a module that tagged itself would be deciding it.
  default_tags {
    tags = local.tags
  }
}
