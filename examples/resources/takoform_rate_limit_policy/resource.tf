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

resource "takoform_rate_limit_policy" "example" {
  name                = "rate-limit-policy"
  requests_per_minute = 600
  burst               = 100
  scope               = "client"

  connections = [
    {
      name        = "subject"
      resource    = "HttpRoute/http-route"
      permissions = ["administer"]
      projection  = "http.route.v1"
    },
  ]
}

output "rate_limit_policy_outputs" {
  value = takoform_rate_limit_policy.example.outputs
}
