terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative Provider metadata. The current
      # exact FormRef and digest do not contain this name.
      # Provider 3's broader aggregate remains retained history. The next
      # major registers only the tako0614 Edge Form set.
      version = "~> 4.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_worker_version" "example" {
  revision_owner = "module-worker"
  worker         = "module-worker"
  bundle         = "worker-bundle"
  handlers       = ["fetch"]
  vars_json      = jsonencode({ "LOG_LEVEL" = "info" })

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
      name        = "OBJECTS"
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
  ]
}

output "worker_version_outputs" {
  value = takoform_worker_version.example.outputs_json
}
