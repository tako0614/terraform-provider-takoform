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

resource "takoform_model_endpoint" "example" {
  name                = "model-endpoint"
  artifact_media_type = "application/vnd.safetensors"
  artifact_sha256     = "fd52f6d3dfaa989615128f2049893584cc6f71a4ae5536b86681ae33ae2c072b"
  artifact_url        = "https://artifacts.portable-conformance.invalid/embedding-small.safetensors"
  task                = "embedding"
  max_concurrency     = 8
}

output "model_endpoint_outputs" {
  value = takoform_model_endpoint.example.outputs
}
