terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # Provider v2.1.1 is Registry-published; release/version.json remains
      # candidate-only descriptor metadata after owner publication.
      version = "= 2.1.1"
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
