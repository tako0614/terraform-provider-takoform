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

resource "takoform_object_bucket" "assets" {
  name          = "assets"
  storage_class = "standard"
  interfaces    = ["s3_api", "signed_url"]
}

output "bucket_selected_implementation" {
  value = takoform_object_bucket.assets.selected_implementation
}

output "bucket_outputs" {
  value = takoform_object_bucket.assets.outputs
}
