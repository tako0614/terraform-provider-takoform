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

resource "takoform_worker_endpoint" "example" {
  name   = "worker-endpoint"
  worker = "module-worker"
}

output "worker_endpoint_hostname" {
  value = takoform_worker_endpoint.example.hostname
}

output "worker_endpoint_url" {
  value = takoform_worker_endpoint.example.url
}
