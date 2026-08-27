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

resource "takoform_pull_queue" "example" {
  name                               = "pull-queue"
  message_retention_seconds          = 345600
  default_visibility_timeout_seconds = 30
  receive_wait_bound_seconds         = 20

  dead_letter = {
    queue             = "dead-letters"
    max_receive_count = 5
  }
}

output "pull_queue_outputs" {
  value = takoform_pull_queue.example.outputs_json
}
