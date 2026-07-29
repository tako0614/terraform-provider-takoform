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

resource "takoform_key_value_store" "example" {
  name                = "key-value-store"
  consistency         = "eventual"
  default_ttl_seconds = 3600
}

output "key_value_store_outputs" {
  value = takoform_key_value_store.example.outputs
}
