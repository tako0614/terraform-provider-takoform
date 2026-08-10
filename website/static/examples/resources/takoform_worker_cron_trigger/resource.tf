terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # stable v2.1.1 release target; descriptor remains candidate-only until owner publication.
      version = "= 2.1.1"
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
  cron   = "*/5 * * * *"
}

output "worker_cron_trigger_outputs" {
  value = takoform_worker_cron_trigger.example.outputs_json
}
