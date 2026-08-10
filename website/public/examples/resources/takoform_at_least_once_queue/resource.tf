terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # stable v2.1.1 release target; descriptor remains candidate-only until owner publication.
      version = "= 2.1.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_at_least_once_queue" "example" {
  name                      = "at-least-once-queue"
  message_retention_seconds = 345600
  delivery_delay_seconds    = 0
}

output "at_least_once_queue_outputs" {
  value = takoform_at_least_once_queue.example.outputs_json
}
