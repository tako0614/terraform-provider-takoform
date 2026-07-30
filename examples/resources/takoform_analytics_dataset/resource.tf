terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.2"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_analytics_dataset" "example" {
  name            = "analytics-dataset"
  partition_field = "eventDate"
  retention_days  = 365
}

output "analytics_dataset_outputs" {
  value = takoform_analytics_dataset.example.outputs
}
