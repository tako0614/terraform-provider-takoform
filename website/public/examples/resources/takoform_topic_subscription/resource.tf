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
