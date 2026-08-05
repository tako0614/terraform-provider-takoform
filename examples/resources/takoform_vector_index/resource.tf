terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_vector_index" "example" {
  name       = "vector-index"
  dimensions = 1536
  metric     = "cosine"
}

output "vector_index_outputs" {
  value = takoform_vector_index.example.outputs
}
