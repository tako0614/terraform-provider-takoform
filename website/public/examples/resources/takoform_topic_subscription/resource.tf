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

resource "takoform_topic_subscription" "example" {
  name                = "topic-subscription"
  topic               = "topic"
  target              = "events"
  filter_policy       = { "eventType" = ["order.created", "order.updated"] }
  retry_delay_seconds = 60
  max_retries         = 3
  dead_letter         = "dead-letters"
}

output "topic_subscription_outputs" {
  value = takoform_topic_subscription.example.outputs_json
}
