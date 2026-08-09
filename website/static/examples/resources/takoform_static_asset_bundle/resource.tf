terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_static_asset_bundle" "example" {
  revision_owner  = "module-worker"
  manifest_digest = "sha256:50ae1f6f1c6b121e8d64c4c5a83a3780c92d3a888765640a07bc20b20d71f4ef"
}

output "static_asset_bundle_outputs" {
  value = takoform_static_asset_bundle.example.outputs_json
}
