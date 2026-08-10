terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.1 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.1"
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

  name        = "counter"
  main_module = "index.js"
  content_dir = "${path.module}/dist"
  kv_bindings = { COUNTER = takoform_edge_kv_namespace.counter.name }
  endpoint    = true
}

output "worker_app_url" {
  value = module.worker_app.url
}
