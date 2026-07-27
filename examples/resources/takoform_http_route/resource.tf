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

resource "takoform_http_route" "example" {
  name        = "http-route"
  hostname    = "api.portable-conformance.invalid"
  path_prefix = "/"

  connections = [
    {
      name        = "application"
      resource    = "HttpService/http-service"
      permissions = ["request"]
      projection  = "http.route.v1"
    },
  ]
}

output "http_route_outputs" {
  value = takoform_http_route.example.outputs
}
