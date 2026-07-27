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

resource "takoform_metric_sink" "example" {
  name               = "metric-sink"
  retention_days     = 90
  resolution_seconds = 60
}

output "metric_sink_outputs" {
  value = takoform_metric_sink.example.outputs
}
