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

resource "takoform_container_service" "example" {
  name        = "container-service"
  image       = "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317"
  ports       = [80]
  public_http = true
}

output "container_service_outputs" {
  value = takoform_container_service.example.outputs
}
