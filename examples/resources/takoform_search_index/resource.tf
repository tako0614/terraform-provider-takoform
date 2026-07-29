terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_search_index" "example" {
  name     = "search-index"
  fields   = ["body", "title"]
  language = "en"
}

output "search_index_outputs" {
  value = takoform_search_index.example.outputs
}
