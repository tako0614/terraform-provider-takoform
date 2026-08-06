terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
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
