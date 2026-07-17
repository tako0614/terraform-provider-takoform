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

resource "takoform_queue" "delivery" {
  name           = "delivery"
  max_retries    = 5
  max_batch_size = 25
}

output "queue_resource_version" {
  value = takoform_queue.delivery.resource_version
}

output "queue_outputs" {
  value = takoform_queue.delivery.outputs
}
