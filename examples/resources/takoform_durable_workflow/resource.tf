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

resource "takoform_durable_workflow" "ingest" {
  name            = "ingest"
  artifact_url    = "https://example.com/releases/ingest-workflow.js"
  artifact_sha256 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
  entrypoint      = "IngestWorkflow"

  max_attempts            = 5
  initial_backoff_seconds = 10
}

output "durable_workflow_outputs" {
  value = takoform_durable_workflow.ingest.outputs
}
