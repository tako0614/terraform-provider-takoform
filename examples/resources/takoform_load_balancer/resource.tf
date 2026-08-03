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

resource "takoform_load_balancer" "example" {
  name                 = "load-balancer"
  protocol             = "https"
  listen_port          = 443
  health_check_path    = "/healthz"
  internal             = false
  idle_timeout_seconds = 60

  connections = [
    {
      name        = "upstream"
      resource    = "ContainerService/container-service"
      permissions = ["request"]
      projection  = "network.backend.v1"
    },
  ]
}

output "load_balancer_outputs" {
  value = takoform_load_balancer.example.outputs
}
