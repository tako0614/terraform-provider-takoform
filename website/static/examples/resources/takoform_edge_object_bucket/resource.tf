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

resource "takoform_edge_object_bucket" "example" {
  name = "object-bucket"
}

output "edge_object_bucket_outputs" {
  value = takoform_edge_object_bucket.example.outputs_json
}
