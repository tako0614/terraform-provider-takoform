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

resource "takoform_webhook_endpoint" "example" {
  name            = "webhook-endpoint"
  path            = "/hooks"
  allowed_methods = ["POST"]

  connections = [
    {
      name        = "destination"
      resource    = "Queue/queue"
      permissions = ["send"]
      projection  = "queue.producer.v1"
    },
  ]
}

output "webhook_endpoint_outputs" {
  value = takoform_webhook_endpoint.example.outputs
}
