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

resource "takoform_container_traffic" "example" {
  name    = "container-traffic"
  service = "container-service"

  revisions = [
    {
      container_revision = "container-revision"
      weight             = 10000
    },
  ]
}

output "container_traffic_outputs" {
  value = takoform_container_traffic.example.outputs_json
}
