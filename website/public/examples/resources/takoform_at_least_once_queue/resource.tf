terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative official-provider metadata. The
      # current versionless Form and its digest do not contain this name.
      # Provider 2.1.1 carries only retained versioned history; use a provider
      # release whose exact-Form registry includes this current identity.
      version = ">= 3.0.0"
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
