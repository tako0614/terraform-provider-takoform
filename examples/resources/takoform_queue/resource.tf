terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.1.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_queue" "example" {
  name                      = "queue"
  message_retention_seconds = 345600
  ordering                  = "best_effort"
}

output "queue_outputs" {
  value = takoform_queue.example.outputs
}
