terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_edge_worker" "api" {
  name               = "api"
  artifact_url       = "https://example.com/releases/api-worker.js"
  artifact_sha256    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
  compatibility_date = "2026-06-29"
  profiles           = ["workers_bindings"]
}

output "api_resource_version" {
  value = takoform_edge_worker.api.resource_version
}

output "api_outputs" {
  value = takoform_edge_worker.api.outputs
}
