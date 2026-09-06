# The edge: serves the React SPA and forwards the API path to the backend.
#
# WHAT AMPLIFY IS AND IS NOT. Amplify Hosting serves static and Node-rendered
# frontends -- React, Vue, Next.js, Nuxt. It cannot run the Go backend, and
# Amplify Gen 2's "fullstack" story generates an AppSync + DynamoDB + Cognito
# application, which is a different system, not a host for this one. So the API
# runs on Lambda regardless; Amplify's role here is the SPA plus a reverse proxy.
#
# THE COUPLING THAT ISN'T. api_origin_url is an opaque string. This module does
# not know a Lambda is behind it, which is what makes replacing Amplify with
# S3 + CloudFront a change to this module and the root wiring, and nothing else.
#
# There is no repository connection on purpose: connecting one would mean
# storing a GitHub token in AWS and would move builds out of the CI pipeline
# that already gates everything. GitHub Actions pushes builds instead, with
# `aws amplify start-deployment`, authenticated by OIDC.

variable "name_prefix" {
  description = "Prefix for every resource name."
  type        = string
}

variable "branch_name" {
  description = "Amplify branch that serves production. Named after the git branch it tracks, though nothing is connected to git."
  type        = string
  default     = "main"
}

variable "api_origin_url" {
  description = "Where /api/* is forwarded. Opaque to this module."
  type        = string
}

variable "api_path_prefix" {
  description = "Path prefix the SPA calls and the edge strips. The Go router registers routes at the root, so /api/ready must reach the backend as /ready -- the same prefix strip deploy/nginx.conf gets from the trailing slash on proxy_pass."
  type        = string
  default     = "/api"
}

variable "domain_name" {
  description = "Custom domain. Left null, the app is served on its generated amplifyapp.com address with AWS-managed TLS. Wired now so adding a domain later is a tfvars change."
  type        = string
  default     = null
}
