terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_edge_worker" "example" {
  name                    = "edge-worker"
  artifact_media_type     = "application/vnd.takoform.edge-worker+tar"
  artifact_sha256         = "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37"
  artifact_url            = "https://artifacts.portable-conformance.invalid/edge-worker.tar"
  entrypoint              = "worker.mjs"
  runtime                 = "javascript"
  runtime_version         = "2026.1"
  request_timeout_seconds = 30
  concurrency             = 100
  configuration           = { "LOG_LEVEL" = "info" }

  connections = [
    {
      name        = "assets"
      resource    = "ObjectBucket/object-bucket"
      permissions = ["read"]
      projection  = "object.binding.v1"
    },
  ]
}

output "edge_worker_outputs" {
  value = takoform_edge_worker.example.outputs
}
