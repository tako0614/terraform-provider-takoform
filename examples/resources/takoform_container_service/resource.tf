terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_container_service" "example" {
  name              = "container-service"
  image             = "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317"
  ports             = [80]
  public_http       = true
  cpu_millicores    = 250
  memory_mib        = 512
  replicas          = 2
  health_check_path = "/healthz"
  configuration     = { "LOG_LEVEL" = "info" }
}

output "container_service_outputs" {
  value = takoform_container_service.example.outputs
}
