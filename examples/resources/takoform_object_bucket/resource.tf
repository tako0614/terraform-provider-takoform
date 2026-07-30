terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.2"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_object_bucket" "example" {
  name             = "object-bucket"
  storage_class    = "standard"
  versioning       = true
  access_protocols = ["s3_api"]
}

output "object_bucket_outputs" {
  value = takoform_object_bucket.example.outputs
}
