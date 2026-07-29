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

# Version is explicit because interface identity is the pair (name, version).
# Omitting it is allowed only when this name has one visible version.
data "takoform_interface" "runtime" {
  name          = "example.runtime"
  version       = "1"
  resource_kind = "EdgeWorker"
  resource_name = "api"
}

output "runtime_interface_document" {
  value = data.takoform_interface.runtime.document_json
}
