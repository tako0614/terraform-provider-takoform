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

resource "takoform_function_deployment" "example" {
  name     = "function-deployment"
  function = "function"

  versions = [
    {
      function_version = "function-version"
      weight           = 10000
    },
  ]
}

output "function_deployment_outputs" {
  value = takoform_function_deployment.example.outputs_json
}
