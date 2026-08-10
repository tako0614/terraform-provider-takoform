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

resource "takoform_edge_kv_namespace" "example" {
  name = "edge-kv-namespace"
}

output "edge_kv_namespace_outputs" {
  value = takoform_edge_kv_namespace.example.outputs_json
}
