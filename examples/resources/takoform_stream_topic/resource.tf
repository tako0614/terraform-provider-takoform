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

resource "takoform_stream_topic" "example" {
  name            = "stream-topic"
  partitions      = 3
  retention_hours = 24
}

output "stream_topic_outputs" {
  value = takoform_stream_topic.example.outputs
}
