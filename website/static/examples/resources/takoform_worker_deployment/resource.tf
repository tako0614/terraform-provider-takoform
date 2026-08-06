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

resource "takoform_worker_deployment" "example" {
  name   = "worker-deployment"
  worker = "module-worker"

  versions = [
    {
      worker_version = "worker-version"
      weight         = 10000
    },
  ]
}

output "worker_deployment_outputs" {
  value = takoform_worker_deployment.example.outputs_json
}
