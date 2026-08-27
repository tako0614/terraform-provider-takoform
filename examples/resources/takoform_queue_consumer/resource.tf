terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative Provider metadata. The current
      # exact FormRef and digest do not contain this name.
      # Provider 2.1.1's 15 versioned identities remain retained history.
      version = "= 3.0.0"
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
