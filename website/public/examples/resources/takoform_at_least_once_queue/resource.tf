terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative Provider metadata. The current
      # exact FormRef and digest do not contain this name.
      # Provider 3's broader aggregate remains retained history. The next
      # major registers only the tako0614 Edge Form set.
      version = "~> 4.0"
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
