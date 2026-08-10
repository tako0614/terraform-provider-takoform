terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # stable v2.1.0 release target; descriptor remains candidate-only until owner publication.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_worker_custom_domain" "example" {
  name     = "worker-custom-domain"
  worker   = "module-worker"
  hostname = "app.portable-conformance.invalid"
}

output "worker_custom_domain_outputs" {
  value = takoform_worker_custom_domain.example.outputs_json
}
