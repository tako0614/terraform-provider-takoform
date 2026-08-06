terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
      # provider v2.1.0 is an unpublished source candidate; build the provider from source.
      version = "= 2.1.0"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

# The four form_* values come from the Form's own publisher. They are an exact
# identity, not a lookup key: the provider validates their grammar and the host
# validates the spec against that exact Form.
resource "takoform_resource" "example" {
  form_api_version        = "forms.example.com/v1alpha1"
  form_kind               = "ExampleWidget"
  form_definition_version = "1.0.0"
  form_schema_digest      = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
  name                    = "example-widget"

  spec_json = jsonencode({ size = "small" })
}

output "resource_outputs" {
  value = takoform_resource.example.outputs_json
}
