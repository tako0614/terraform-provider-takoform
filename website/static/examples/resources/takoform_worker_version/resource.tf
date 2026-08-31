terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative official-provider metadata. The
      # current versionless Form and its digest do not contain this name.
      # Provider 2.1.1 carries only retained versioned history; use a provider
      # release whose exact-Form registry includes this current identity. This
      # example is candidate/unpublished until Provider >=3.1.0 is released.
      version = ">= 3.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_worker_version" "example" {
  # Candidate/unpublished Provider >=3.1.0 only: released Provider 3.0.0 does
  # not expose `apply_idempotency_key`.
  revision_owner          = "module-worker"
  apply_idempotency_key   = "worker-version-example-v1"
  worker                  = "module-worker"
  bundle                  = "worker-bundle"
  handlers                = ["fetch"]
  vars_json               = jsonencode({ "LOG_LEVEL" = "info" })
  required_sensitive_vars = ["API_SIGNING_TOKEN_NAME"]

  assets = {
    bundle             = "static-asset-bundle"
    run_worker_first   = true
    not_found_handling = "single_page_application"
  }

  kv_bindings = [
    {
      name        = "CACHE"
      target_name = "edge-kv-namespace"
    },
  ]

  sqlite_bindings = [
    {
      name        = "DB"
      target_name = "sqlite-database"
    },
  ]

  queue_producer_bindings = [
    {
      name        = "EVENTS"
      target_name = "at-least-once-queue"
    },
  ]

  service_bindings = [
    {
      name        = "AUTH"
      target_name = "auth-worker"
    },
  ]

  workflow_bindings = [
    {
      name        = "ORDERS"
      target_name = "durable-workflow"
    },
  ]

  actor_bindings = [
    {
      name        = "ROOMS"
      target_name = "actor-namespace"
    },
  ]

  external_services = [
    {
      name     = "PRIMARY_DB"
      protocol = "org.postgresql.wire"
    },
    {
      name     = "MEDIA"
      protocol = "com.amazonaws.s3"
    },
  ]
}

output "worker_version_outputs" {
  value = takoform_worker_version.example.outputs_json
}
