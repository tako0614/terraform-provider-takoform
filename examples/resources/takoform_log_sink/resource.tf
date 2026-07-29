terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_log_sink" "example" {
  name           = "log-sink"
  retention_days = 30
  format         = "json"
}

output "log_sink_outputs" {
  value = takoform_log_sink.example.outputs
}
