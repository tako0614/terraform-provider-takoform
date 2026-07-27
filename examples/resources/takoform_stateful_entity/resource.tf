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

resource "takoform_stateful_entity" "example" {
  name          = "stateful-entity"
  entity_class  = "RoomEntity"
  migration_tag = "v1"
}

output "stateful_entity_outputs" {
  value = takoform_stateful_entity.example.outputs
}
