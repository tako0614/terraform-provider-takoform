terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # Provider v2.1.1 is Registry-published; release/version.json remains
      # candidate-only descriptor metadata after owner publication. v2.1.1
      # serves this resource type under the retained v1beta1 identities; the
      # v1beta2 identity this page documents ships with the next release
      # (decision 0046).
      version = "= 2.1.1"
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
