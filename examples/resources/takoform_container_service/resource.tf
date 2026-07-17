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

resource "takoform_container_service" "agent" {
  name        = "agent"
  image       = "ghcr.io/example/agent@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
  ports       = [8080]
  public_http = true

}

output "container_resource_version" {
  value = takoform_container_service.agent.resource_version
}

output "container_outputs" {
  value = takoform_container_service.agent.outputs
}
