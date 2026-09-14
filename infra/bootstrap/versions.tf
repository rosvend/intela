terraform {
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # No backend block on purpose. This root creates the bucket every OTHER root
  # stores its state in, so it cannot store its own state there -- that is the
  # chicken and egg this directory exists to break. Its state stays local and
  # gitignored; it is run by a human, rarely, and holds nothing that a rebuild
  # from `terraform import` could not recover.
}

provider "aws" {
  region = var.region

  default_tags {
    tags = local.tags
  }
}
