terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_worker_cron_trigger" "example" {
  name   = "worker-cron-trigger"
  worker = "module-worker"
  cron   = "0 3 * * *"
}

output "worker_cron_trigger_outputs" {
  value = takoform_worker_cron_trigger.example.outputs_json
}
