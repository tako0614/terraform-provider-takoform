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

resource "takoform_key_value_store" "example" {
  name        = "key-value-store"
  consistency = "eventual"
}

output "key_value_store_outputs" {
  value = takoform_key_value_store.example.outputs
}
