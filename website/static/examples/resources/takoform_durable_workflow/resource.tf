terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # This resource type is non-normative official-provider metadata. The
      # current versionless Form and its digest do not contain this name.
      # Provider 2.1.1 carries only retained versioned history; use a provider
      # release whose exact-Form registry includes this current identity.
      version = ">= 3.0.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_durable_workflow" "example" {
  name       = "durable-workflow"
  worker     = "module-worker"
  class_name = "OrderFulfilment"
}

output "durable_workflow_outputs" {
  value = takoform_durable_workflow.example.outputs_json
}
