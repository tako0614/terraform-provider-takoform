terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # Provider v2.1.1 is Registry-published; release/version.json remains
      # candidate-only descriptor metadata after owner publication. v2.1.1
      # serves this resource type under the retained v1beta1 identities; the
      # v1beta2 identity this page documents ships with the next release
      # (decision 0046).
      version = "= 2.1.1"
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
