terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_indexed_store" "example" {
  name               = "indexed-store"
  partition_key      = "tenantId"
  sort_key           = "createdAt"
  indexed_attributes = ["status"]
}

output "indexed_store_outputs" {
  value = takoform_indexed_store.example.outputs
}
