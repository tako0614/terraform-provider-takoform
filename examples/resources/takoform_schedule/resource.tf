terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_schedule" "nightly_ingest" {
  name     = "nightly-ingest"
  cron     = "0 0 * * *"
  timezone = "UTC"

  connections = [{
    name        = "workflow"
    resource    = "DurableWorkflow/ingest"
    permissions = ["invoke"]
    projection  = "schedule_trigger"
  }]
}

output "schedule_outputs" {
  value = takoform_schedule.nightly_ingest.outputs
}
