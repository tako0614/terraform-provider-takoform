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

resource "takoform_stateful_entity" "example" {
  name                = "stateful-entity"
  artifact_media_type = "application/vnd.takoform.stateful-entity+tar"
  artifact_sha256     = "5d877f919bf8db6e6fd819e32f74dff6fc94b06f8914fa1abf5bcd2fb32ae958"
  artifact_url        = "https://artifacts.portable-conformance.invalid/stateful-entity.tar"
  entity_class        = "RoomEntity"
  runtime             = "javascript"
  runtime_version     = "2026.1"
  persistence         = "transactional"
  migration_tag       = "v1"
  configuration       = { "LOG_LEVEL" = "info" }
}

output "stateful_entity_outputs" {
  value = takoform_stateful_entity.example.outputs
}
