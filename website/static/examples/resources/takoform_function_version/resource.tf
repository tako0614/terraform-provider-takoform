terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative Provider metadata. The current
      # exact FormRef and digest do not contain this name.
      # Provider 2.1.1's 15 versioned identities remain retained history.
      version = "= 3.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_function_version" "example" {
  revision_owner          = "function"
  function                = "function"
  artifact                = "sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43"
  handler                 = "handle"
  vars_json               = jsonencode({ "LOG_LEVEL" = "info" })
  required_sensitive_vars = ["API_SIGNING_TOKEN"]
  memory_mib              = 512
  timeout_seconds         = 30
  max_concurrency         = 10

  external_services = [
    {
      name     = "PRIMARY_DB"
      protocol = "org.postgresql.wire"
    },
  ]
}

output "function_version_outputs" {
  value = takoform_function_version.example.outputs_json
}
