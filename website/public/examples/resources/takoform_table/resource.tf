terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative official-provider metadata. The
      # retained versionless Form and its digest do not contain this name.
      # Provider 2.1.1 carries only retained versioned history; use a provider
      # release whose exact-Form registry includes this retained identity.
      version = ">= 3.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_table" "example" {
  name          = "table"
  ttl_attribute = "expiresAt"

  partition_key = {
    name = "tenantId"
    type = "string"
  }

  sort_key = {
    name = "createdAt"
    type = "number"
  }

  secondary_indexes = [
    {
      name          = "by-email"
      partition_key = "email"
      sort_key      = "createdAt"
    },
  ]
}

output "table_outputs" {
  value = takoform_table.example.outputs_json
}
