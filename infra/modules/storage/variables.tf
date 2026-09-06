# The vault for raw usage reports.
#
# ADR 0010 already decided this ("MinIO local con object-lock, S3 en
# produccion"), and ADR 0005/0006 say why: a figure has to be explainable back
# to the exact file it came from, so the evidence must be immutable.
#
# OBJECT LOCK IS ON, WITH NO DEFAULT RETENTION. Object Lock can only be enabled
# when a bucket is created, so deferring it would mean recreating the bucket
# later. Enabling it without a default retention rule means nothing is actually
# locked yet -- so terraform destroy still works during the MVP -- while the
# capability is there for the S3 adapter to set per-object retention when it
# lands. Today the only object store adapter is objetos/disco.go, which writes
# to a local filesystem and is not wired into any binary.

variable "name_prefix" {
  description = "Prefix for every resource name."
  type        = string
}

variable "bucket_suffix" {
  description = "Disambiguator appended to the bucket name. S3 bucket names are globally unique, so this is normally the account id."
  type        = string
}
