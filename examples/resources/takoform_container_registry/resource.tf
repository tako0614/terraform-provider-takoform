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

resource "takoform_container_registry" "example" {
  name           = "container-registry"
  visibility     = "private"
  immutable_tags = true
}

output "container_registry_outputs" {
  value = takoform_container_registry.example.outputs
}
