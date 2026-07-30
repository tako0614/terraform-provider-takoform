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

resource "takoform_http_route" "example" {
  name              = "http-route"
  hostname          = "api.portable-conformance.invalid"
  path_prefix       = "/"
  strip_path_prefix = false

  connections = [
    {
      name        = "application"
      resource    = "EdgeWorker/edge-worker"
      permissions = ["request"]
      projection  = "http.route.v1"
    },
  ]
}

output "http_route_outputs" {
  value = takoform_http_route.example.outputs
}
