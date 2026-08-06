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

resource "takoform_queue_consumer" "example" {
  name                      = "queue-consumer"
  queue                     = "at-least-once-queue"
  worker                    = "module-worker"
  max_batch_size            = 10
  max_batch_timeout_seconds = 5
  max_retries               = 3
  retry_delay_seconds       = 60
  dead_letter_queue         = "dead-letters"
  max_concurrency           = 4
}

output "queue_consumer_outputs" {
  value = takoform_queue_consumer.example.outputs_json
}
