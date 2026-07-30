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

resource "takoform_queue" "example" {
  name                       = "queue"
  max_retries                = 5
  max_batch_size             = 10
  visibility_timeout_seconds = 30
  message_retention_seconds  = 345600
  max_message_bytes          = 262144
  delivery_delay_seconds     = 0
  ordering                   = "best_effort"
}

output "queue_outputs" {
  value = takoform_queue.example.outputs
}
