terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 0.3"
    }
  }
}

resource "takoform_edge_worker" "api" {
  name            = "example-api"
  artifact_url    = "https://example.invalid/example-api.tar.gz"
  artifact_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

resource "takoform_interface" "primary" {
  name          = "example.runtime"
  version       = "1"
  resource_kind = "EdgeWorker"
  resource_name = takoform_edge_worker.api.name

  document_json = jsonencode({
    title = "Example runtime surface"
  })
  inputs_json = jsonencode([
    {
      name    = "endpoint"
      source  = "output"
      pointer = "/url"
    }
  ])
}
