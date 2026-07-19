terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

# Version is explicit because interface identity is the pair (name, version).
# Omitting it is allowed only when this name has one visible version.
data "takoform_interface" "mcp" {
  name          = "mcp.server"
  version       = "2025-11-25"
  resource_kind = "EdgeWorker"
  resource_name = "api"
}

output "mcp_interface_document" {
  value = data.takoform_interface.mcp.document_json
}
