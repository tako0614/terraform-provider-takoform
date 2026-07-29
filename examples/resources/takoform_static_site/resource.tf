terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_static_site" "example" {
  name                  = "static-site"
  artifact_media_type   = "application/vnd.takoform.static-site+tar"
  artifact_sha256       = "3b1d4c2f9a8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817065f4e"
  artifact_url          = "https://artifacts.portable-conformance.invalid/static-site.tar"
  index_document        = "index.html"
  error_document        = "404.html"
  single_page_app       = false
  cache_control_seconds = 300
}

output "static_site_outputs" {
  value = takoform_static_site.example.outputs
}
