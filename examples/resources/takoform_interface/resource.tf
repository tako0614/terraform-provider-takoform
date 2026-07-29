terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 0.2"
    }
  }
}

resource "takoform_http_service" "api" {
  name            = "example-api"
  artifact_url    = "https://example.invalid/example-api.tar.gz"
  artifact_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

resource "takoform_interface" "primary" {
  name          = "example.runtime"
  version       = "1"
  resource_kind = "HttpService"
  resource_name = takoform_http_service.api.name

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
