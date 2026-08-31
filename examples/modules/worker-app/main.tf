terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This example uses apply_idempotency_key, which requires Provider 3.1.0+.
      version = ">= 3.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_edge_kv_namespace" "counter" {
  name = "counter-state"
}

# The official module. It assembles the Module Worker identity, the immutable
# Worker Bundle and Worker Version revisions, the Worker Deployment that selects
# traffic, and the Worker Endpoint that makes the worker reachable.
#
# `source` is the registry address of the published module; this example uses
# the in-repository path so the provider's own gates validate the exact bytes.
module "worker_app" {
  source = "../../../modules/worker-app"

  name                  = "counter"
  main_module           = "index.js"
  content_dir           = "${path.module}/dist"
  apply_idempotency_key = "counter-release-v1"
  kv_bindings           = { COUNTER = takoform_edge_kv_namespace.counter.name }
  endpoint              = true
}

output "worker_app_url" {
  value = module.worker_app.url
}
