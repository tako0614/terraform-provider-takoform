terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_workflow" "example" {
  name                    = "workflow"
  artifact_media_type     = "text/javascript"
  artifact_sha256         = "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5"
  artifact_url            = "https://artifacts.portable-conformance.invalid/workflow.mjs"
  entrypoint              = "IngestWorkflow"
  max_attempts            = 3
  initial_backoff_seconds = 5
  configuration           = { "LOG_LEVEL" = "info" }
}

output "workflow_outputs" {
  value = takoform_workflow.example.outputs
}
