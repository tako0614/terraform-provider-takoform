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
