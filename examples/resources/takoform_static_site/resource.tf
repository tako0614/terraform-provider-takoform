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

resource "takoform_static_site" "example" {
  name                  = "static-site"
  artifact_ref          = "portable-conformance/v1/static-site.tar"
  artifact_sha256       = "3b1d4c2f9a8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817065f4e"
  index_document        = "index.html"
  cache_control_seconds = 300
}

output "static_site_outputs" {
  value = takoform_static_site.example.outputs
}
