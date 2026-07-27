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

resource "takoform_model_endpoint" "example" {
  name  = "model-endpoint"
  model = "portable-conformance/v1/embedding-small"
  task  = "embedding"
}

output "model_endpoint_outputs" {
  value = takoform_model_endpoint.example.outputs
}
