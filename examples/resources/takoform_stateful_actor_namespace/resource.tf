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

# The Resource owns the namespace lifecycle. Actor instances (for example a
# room id) are addressed at runtime and are not separate Resource objects.
resource "takoform_stateful_actor_namespace" "rooms" {
  name            = "rooms"
  class_name      = "RoomActor"
  storage_profile = "durable_sqlite"
  migration_tag   = "v1"
}

output "actor_namespace_outputs" {
  value = takoform_stateful_actor_namespace.rooms.outputs
}
