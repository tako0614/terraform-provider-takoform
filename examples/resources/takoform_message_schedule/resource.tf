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

resource "takoform_message_schedule" "example" {
  name   = "schedule"
  cron   = "*/5 * * * *"
  target = { "attributes" = { "source" = "schedule" }, "body" = { "data" = "scheduled.message", "encoding" = "utf8" }, "queue" = "scheduled-work", "type" = "queueMessage" }
  paused = false

  retry_policy = {
    max_attempts        = 3
    retry_delay_seconds = 60
  }
}

output "message_schedule_outputs" {
  value = takoform_message_schedule.example.outputs_json
}
