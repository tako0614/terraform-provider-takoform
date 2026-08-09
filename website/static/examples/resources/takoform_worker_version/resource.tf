terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_worker_version" "example" {
  revision_owner          = "module-worker"
  worker                  = "module-worker"
  bundle                  = "worker-bundle"
  handlers                = ["fetch"]
  vars_json               = jsonencode({ "LOG_LEVEL" = "info" })
  required_sensitive_vars = ["API_SIGNING_TOKEN_NAME"]

  kv_bindings = [
    {
      name        = "CACHE"
      target_name = "edge-kv-namespace"
    },
  ]

  bucket_bindings = [
    {
      name        = "MEDIA"
      target_name = "object-bucket"
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
}

output "worker_version_outputs" {
  value = takoform_worker_version.example.outputs_json
}
