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

resource "takoform_kv_store" "cache" {
  name        = "cache"
  consistency = "eventual"
}

output "kv_selected_implementation" {
  value = takoform_kv_store.cache.selected_implementation
}

output "kv_outputs" {
  value = takoform_kv_store.cache.outputs
}
