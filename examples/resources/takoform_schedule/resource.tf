terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_schedule" "example" {
  name     = "schedule"
  cron     = "0 0 * * *"
  timezone = "UTC"

  connections = [
    {
      name        = "invocation"
      resource    = "Workflow/workflow"
      permissions = ["invoke"]
      projection  = "schedule.trigger.v1"
    },
  ]
}

output "schedule_outputs" {
  value = takoform_schedule.example.outputs
}
