locals {
  # The Function URL comes back with a trailing slash; the Amplify target needs
  # exactly one separator before the captured suffix.
  api_target = "${trimsuffix(var.api_origin_url, "/")}/<*>"

  # Amplify's documented SPA rule: anything without a file extension, plus
  # anything whose extension is not a real asset type, is served index.html so
  # the client-side router can handle it. Without it, reloading on /estado
  # returns 404 -- the same failure web/nginx.conf avoids with try_files.
  spa_source = "</^[^.]+$|\\.(?!(css|gif|ico|jpg|js|png|txt|svg|woff|woff2|ttf|map|json|webp)$)([^.]+$)/>"
}

resource "aws_amplify_app" "this" {
  name        = var.name_prefix
  description = "Intela dashboard (web/) and reverse proxy to the API"
  platform    = "WEB"

  # Nothing is built here. The build happens in GitHub Actions, which uploads
  # the artifact; leaving auto-build off keeps that unambiguous.
  enable_branch_auto_build    = false
  enable_auto_branch_creation = false

  # Order matters: Amplify applies rules top down, so the API proxy has to be
  # matched before the catch-all that rewrites everything to index.html.
  custom_rule {
    source = "${var.api_path_prefix}/<*>"
    target = local.api_target
    status = "200"
  }

  custom_rule {
    source = local.spa_source
    target = "/index.html"
    status = "200"
  }

  # Ported from web/nginx.conf. Asset filenames carry a content hash, so they
  # are immutable and can be cached forever; index.html must not be, or a
  # browser keeps asking for the previous deploy's assets after a release.
  custom_headers = <<-YAML
    customHeaders:
      - pattern: '/assets/*'
        headers:
          - key: 'Cache-Control'
            value: 'public, max-age=31536000, immutable'
      - pattern: '/index.html'
        headers:
          - key: 'Cache-Control'
            value: 'no-cache'
  YAML
}

resource "aws_amplify_branch" "this" {
  app_id      = aws_amplify_app.this.id
  branch_name = var.branch_name
  framework   = "React"
  stage       = "PRODUCTION"
}

resource "aws_amplify_domain_association" "this" {
  count = var.domain_name == null ? 0 : 1

  app_id      = aws_amplify_app.this.id
  domain_name = var.domain_name

  sub_domain {
    branch_name = aws_amplify_branch.this.branch_name
    prefix      = ""
  }
}
