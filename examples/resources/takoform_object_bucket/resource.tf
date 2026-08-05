terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_object_bucket" "example" {
  name       = "object-bucket"
  versioning = true
}

output "object_bucket_outputs" {
  value = takoform_object_bucket.example.outputs
}
