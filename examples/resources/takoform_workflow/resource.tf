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

resource "takoform_workflow" "example" {
  name                    = "workflow"
  artifact_ref            = "portable-conformance/v1/workflow.mjs"
  artifact_sha256         = "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5"
  entrypoint              = "IngestWorkflow"
  max_attempts            = 3
  initial_backoff_seconds = 5
}

output "workflow_outputs" {
  value = takoform_workflow.example.outputs
}
