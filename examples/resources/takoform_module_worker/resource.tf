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

resource "takoform_module_worker" "example" {
  name = "module-worker"
}

output "module_worker_outputs" {
  value = takoform_module_worker.example.outputs_json
}
